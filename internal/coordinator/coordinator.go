// Package coordinator provides agent communication and coordination.
// It acts as a message bus allowing agents to communicate, claim work,
// and avoid duplicate effort when running in parallel.
package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Coordinator manages agent communication and work distribution.
type Coordinator struct {
	registry      *Registry
	messages      chan Message
	subscribers   map[string]chan Message
	subscribersMu sync.RWMutex

	// Work tracking to prevent duplicates
	claimedWork   map[string]string // workID -> agentID
	claimedWorkMu sync.RWMutex

	// Shared context between agents
	sharedContext   map[string]interface{}
	sharedContextMu sync.RWMutex

	// File locks for context pinning
	fileLocks   map[string]string // file -> agentID
	fileLocksMu sync.RWMutex

	// Wait conditions
	waiters   map[string][]chan struct{}
	waitersMu sync.Mutex

	// Running state (atomic for thread-safe access)
	running atomic.Bool
	done    chan struct{}

	// Metrics for observability
	droppedMessages atomic.Int64

	// Configuration
	subscriberBufferSize int
}

// Options configures coordinator behavior.
type Options struct {
	// MessageBufferSize is the size of the main message channel.
	MessageBufferSize int

	// SubscriberBufferSize is the size of per-subscriber channels.
	SubscriberBufferSize int
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		MessageBufferSize:    1000,
		SubscriberBufferSize: 100,
	}
}

// New creates a new Coordinator with default options.
func New() *Coordinator {
	return NewWithOptions(DefaultOptions())
}

// NewWithOptions creates a new Coordinator with custom options.
func NewWithOptions(opts Options) *Coordinator {
	if opts.MessageBufferSize <= 0 {
		opts.MessageBufferSize = 1000
	}
	if opts.SubscriberBufferSize <= 0 {
		opts.SubscriberBufferSize = 100
	}

	return &Coordinator{
		registry:             NewRegistry(),
		messages:             make(chan Message, opts.MessageBufferSize),
		subscribers:          make(map[string]chan Message),
		claimedWork:          make(map[string]string),
		sharedContext:        make(map[string]interface{}),
		fileLocks:            make(map[string]string),
		waiters:              make(map[string][]chan struct{}),
		done:                 make(chan struct{}),
		subscriberBufferSize: opts.SubscriberBufferSize,
	}
}

// Start begins processing messages.
func (c *Coordinator) Start(ctx context.Context) {
	c.running.Store(true)
	go c.processMessages(ctx)
}

// Stop halts message processing and cleans up resources.
func (c *Coordinator) Stop() {
	c.running.Store(false)
	close(c.done)

	// Clean up maps to prevent memory leaks
	c.claimedWorkMu.Lock()
	clear(c.claimedWork)
	c.claimedWorkMu.Unlock()

	c.sharedContextMu.Lock()
	clear(c.sharedContext)
	c.sharedContextMu.Unlock()

	c.fileLocksMu.Lock()
	clear(c.fileLocks)
	c.fileLocksMu.Unlock()

	c.waitersMu.Lock()
	// Close any remaining waiter channels
	for _, waiters := range c.waiters {
		for _, ch := range waiters {
			select {
			case <-ch:
				// Already closed
			default:
				close(ch)
			}
		}
	}
	clear(c.waiters)
	c.waitersMu.Unlock()

	// Log dropped message stats
	if dropped := c.droppedMessages.Load(); dropped > 0 {
		slog.Warn("coordinator stopped with dropped messages",
			"dropped_count", dropped)
	}
}

// Register adds an agent to the coordinator.
func (c *Coordinator) Register(agent Agent) {
	c.registry.Register(agent)
	agent.SetCoordinator(c)

	// Create subscriber channel for this agent
	bufSize := c.subscriberBufferSize
	if bufSize <= 0 {
		bufSize = 100
	}
	c.subscribersMu.Lock()
	c.subscribers[agent.ID()] = make(chan Message, bufSize)
	c.subscribersMu.Unlock()
}

// Subscribe returns a channel for receiving messages.
func (c *Coordinator) Subscribe(agentID string) <-chan Message {
	c.subscribersMu.RLock()
	defer c.subscribersMu.RUnlock()
	return c.subscribers[agentID]
}

// Send sends a message to the coordinator.
func (c *Coordinator) Send(msg Message) {
	msg.Timestamp = time.Now()
	if c.running.Load() {
		select {
		case c.messages <- msg:
		default:
			c.droppedMessages.Add(1)
			slog.Warn("coordinator message channel full, message dropped",
				"msg_type", msg.Type,
				"from", msg.From,
				"to", msg.To)
		}
	}
}

// processMessages handles incoming messages.
func (c *Coordinator) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case msg := <-c.messages:
			c.handleMessage(msg)
		}
	}
}

// handleMessage processes a single message.
func (c *Coordinator) handleMessage(msg Message) {
	switch msg.Type {
	case MsgClaimWork:
		c.handleClaimWork(msg)
	case MsgWorkComplete:
		c.handleWorkComplete(msg)
	case MsgWorkFailed:
		c.handleWorkFailed(msg)
	case MsgStatusUpdate:
		c.handleStatusUpdate(msg)
	case MsgContextUpdate:
		c.handleContextUpdate(msg)
	case MsgQuery:
		c.handleQuery(msg)
	default:
		// Broadcast to target or all
		c.broadcast(msg)
	}
}

// handleClaimWork processes work claim requests.
func (c *Coordinator) handleClaimWork(msg Message) {
	claim, ok := msg.Payload.(*WorkClaim)
	if !ok {
		return
	}

	c.claimedWorkMu.Lock()
	defer c.claimedWorkMu.Unlock()

	// Check if work is already claimed
	if existingAgent, claimed := c.claimedWork[claim.WorkID]; claimed {
		// Send rejection
		c.sendTo(msg.From, Message{
			Type: MsgWorkClaimed,
			From: "coordinator",
			Payload: &WorkClaim{
				WorkID:      claim.WorkID,
				Description: fmt.Sprintf("Already claimed by %s", existingAgent),
			},
		})
		return
	}

	// Check for file conflicts
	for _, file := range claim.Files {
		if existingAgent, locked := c.fileLocks[file]; locked && existingAgent != msg.From {
			c.sendTo(msg.From, Message{
				Type: MsgWorkClaimed,
				From: "coordinator",
				Payload: &WorkClaim{
					WorkID:      claim.WorkID,
					Description: fmt.Sprintf("File %s locked by %s", file, existingAgent),
				},
			})
			return
		}
	}

	// Claim the work
	c.claimedWork[claim.WorkID] = msg.From

	// Lock the files
	c.fileLocksMu.Lock()
	for _, file := range claim.Files {
		c.fileLocks[file] = msg.From
	}
	c.fileLocksMu.Unlock()

	// Confirm claim
	c.sendTo(msg.From, Message{
		Type: MsgWorkClaimed,
		From: "coordinator",
		Payload: &WorkClaim{
			WorkID:      claim.WorkID,
			Description: "Claimed successfully",
		},
	})

	// Broadcast to others
	c.broadcast(Message{
		Type: MsgWorkClaimed,
		From: msg.From,
		Payload: claim,
	})
}

// handleWorkComplete processes work completion.
func (c *Coordinator) handleWorkComplete(msg Message) {
	result, ok := msg.Payload.(*WorkResult)
	if !ok {
		return
	}

	// Release the claim
	c.claimedWorkMu.Lock()
	delete(c.claimedWork, result.WorkID)
	c.claimedWorkMu.Unlock()

	// Notify waiters
	c.notifyWaiters(fmt.Sprintf("work:%s", result.WorkID))
	c.notifyWaiters(fmt.Sprintf("agent:%s", msg.From))

	// Update agent state
	c.registry.UpdateState(msg.From, StateComplete)

	// Broadcast completion
	c.broadcast(msg)
}

// handleWorkFailed processes work failure.
func (c *Coordinator) handleWorkFailed(msg Message) {
	result, ok := msg.Payload.(*WorkResult)
	if !ok {
		return
	}

	// Release the claim
	c.claimedWorkMu.Lock()
	delete(c.claimedWork, result.WorkID)
	c.claimedWorkMu.Unlock()

	// Release file locks
	c.releaseFileLocks(msg.From)

	// Update agent state
	c.registry.UpdateState(msg.From, StateFailed)

	// Notify waiters
	c.notifyWaiters(fmt.Sprintf("work:%s", result.WorkID))
	c.notifyWaiters(fmt.Sprintf("agent:%s", msg.From))

	// Broadcast failure
	c.broadcast(msg)
}

// handleStatusUpdate processes status updates.
func (c *Coordinator) handleStatusUpdate(msg Message) {
	status, ok := msg.Payload.(*StatusUpdate)
	if !ok {
		return
	}

	c.registry.UpdateState(msg.From, status.State)
	c.broadcast(msg)
}

// handleContextUpdate processes shared context updates.
func (c *Coordinator) handleContextUpdate(msg Message) {
	update, ok := msg.Payload.(*ContextUpdate)
	if !ok {
		return
	}

	c.sharedContextMu.Lock()
	c.sharedContext[update.Key] = update.Value
	c.sharedContextMu.Unlock()

	// Notify waiters for this context key
	c.notifyWaiters(fmt.Sprintf("context:%s", update.Key))

	// Broadcast to relevant agents
	c.broadcast(msg)
}

// handleQuery processes queries from agents.
func (c *Coordinator) handleQuery(msg Message) {
	query, ok := msg.Payload.(*Query)
	if !ok {
		return
	}

	var response interface{}
	var err string

	switch query.QueryType {
	case "claimed_work":
		c.claimedWorkMu.RLock()
		response = c.claimedWork
		c.claimedWorkMu.RUnlock()
	case "context":
		key, _ := query.Data.(string)
		c.sharedContextMu.RLock()
		response = c.sharedContext[key]
		c.sharedContextMu.RUnlock()
	case "file_locks":
		c.fileLocksMu.RLock()
		response = c.fileLocks
		c.fileLocksMu.RUnlock()
	case "agents":
		response = c.registry.List()
	default:
		err = fmt.Sprintf("unknown query type: %s", query.QueryType)
	}

	c.sendTo(msg.From, Message{
		Type: MsgQueryResponse,
		From: "coordinator",
		Payload: &QueryResponse{
			QueryID: query.QueryID,
			Data:    response,
			Error:   err,
		},
	})
}

// broadcast sends a message to all subscribers or a specific target.
func (c *Coordinator) broadcast(msg Message) {
	c.subscribersMu.RLock()
	defer c.subscribersMu.RUnlock()

	if msg.To != "" {
		// Targeted message
		if ch, ok := c.subscribers[msg.To]; ok {
			select {
			case ch <- msg:
			default:
				c.droppedMessages.Add(1)
				slog.Warn("subscriber channel full, message dropped",
					"target", msg.To,
					"msg_type", msg.Type,
					"from", msg.From)
			}
		}
		return
	}

	// Broadcast to all except sender
	for agentID, ch := range c.subscribers {
		if agentID == msg.From {
			continue
		}
		select {
		case ch <- msg:
		default:
			c.droppedMessages.Add(1)
			slog.Warn("subscriber channel full, broadcast message dropped",
				"target", agentID,
				"msg_type", msg.Type,
				"from", msg.From)
		}
	}
}

// sendTo sends a message to a specific agent.
func (c *Coordinator) sendTo(agentID string, msg Message) {
	c.subscribersMu.RLock()
	ch, ok := c.subscribers[agentID]
	c.subscribersMu.RUnlock()

	if ok {
		select {
		case ch <- msg:
		default:
			c.droppedMessages.Add(1)
			slog.Warn("agent channel full, message dropped",
				"target", agentID,
				"msg_type", msg.Type,
				"from", msg.From)
		}
	}
}

// releaseFileLocks releases all file locks held by an agent.
func (c *Coordinator) releaseFileLocks(agentID string) {
	c.fileLocksMu.Lock()
	defer c.fileLocksMu.Unlock()

	for file, holder := range c.fileLocks {
		if holder == agentID {
			delete(c.fileLocks, file)
		}
	}
}

// notifyWaiters notifies all waiters for a condition.
func (c *Coordinator) notifyWaiters(key string) {
	c.waitersMu.Lock()
	defer c.waitersMu.Unlock()

	if waiters, ok := c.waiters[key]; ok {
		for _, ch := range waiters {
			close(ch)
		}
		delete(c.waiters, key)
	}
}

// WaitFor blocks until a condition is met.
func (c *Coordinator) WaitFor(ctx context.Context, cond WaitCondition) error {
	var key string
	switch cond.Type {
	case WaitForAgent:
		key = fmt.Sprintf("agent:%s", cond.Target)
	case WaitForWork:
		key = fmt.Sprintf("work:%s", cond.Target)
	case WaitForContext:
		key = fmt.Sprintf("context:%s", cond.Target)
	case WaitForAllAgents:
		// Special case: wait for all to be idle
		for !c.registry.AllIdle() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		return nil
	}

	// Create wait channel
	ch := make(chan struct{})
	c.waitersMu.Lock()
	c.waiters[key] = append(c.waiters[key], ch)
	c.waitersMu.Unlock()

	// Wait with timeout
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(cond.Timeout):
		return fmt.Errorf("timeout waiting for %s", key)
	case <-ch:
		return nil
	}
}

// ClaimWork attempts to claim work for an agent.
func (c *Coordinator) ClaimWork(agentID string, claim *WorkClaim) bool {
	c.claimedWorkMu.Lock()
	defer c.claimedWorkMu.Unlock()

	// Check if already claimed
	if _, claimed := c.claimedWork[claim.WorkID]; claimed {
		return false
	}

	// Check file conflicts
	c.fileLocksMu.RLock()
	for _, file := range claim.Files {
		if holder, locked := c.fileLocks[file]; locked && holder != agentID {
			c.fileLocksMu.RUnlock()
			return false
		}
	}
	c.fileLocksMu.RUnlock()

	// Claim it
	c.claimedWork[claim.WorkID] = agentID

	// Lock files
	c.fileLocksMu.Lock()
	for _, file := range claim.Files {
		c.fileLocks[file] = agentID
	}
	c.fileLocksMu.Unlock()

	return true
}

// ReleaseWork releases a work claim.
func (c *Coordinator) ReleaseWork(workID string, agentID string) {
	c.claimedWorkMu.Lock()
	if holder, ok := c.claimedWork[workID]; ok && holder == agentID {
		delete(c.claimedWork, workID)
	}
	c.claimedWorkMu.Unlock()

	c.releaseFileLocks(agentID)
}

// GetContext retrieves a value from shared context.
func (c *Coordinator) GetContext(key string) (interface{}, bool) {
	c.sharedContextMu.RLock()
	defer c.sharedContextMu.RUnlock()
	val, ok := c.sharedContext[key]
	return val, ok
}

// SetContext sets a value in shared context.
func (c *Coordinator) SetContext(key string, value interface{}) {
	c.sharedContextMu.Lock()
	c.sharedContext[key] = value
	c.sharedContextMu.Unlock()

	c.notifyWaiters(fmt.Sprintf("context:%s", key))
}

// LockFiles locks files for an agent.
func (c *Coordinator) LockFiles(agentID string, files []string) bool {
	c.fileLocksMu.Lock()
	defer c.fileLocksMu.Unlock()

	// Check all files first
	for _, file := range files {
		if holder, locked := c.fileLocks[file]; locked && holder != agentID {
			return false
		}
	}

	// Lock all files
	for _, file := range files {
		c.fileLocks[file] = agentID
	}
	return true
}

// UnlockFiles unlocks files for an agent.
func (c *Coordinator) UnlockFiles(agentID string, files []string) {
	c.fileLocksMu.Lock()
	defer c.fileLocksMu.Unlock()

	for _, file := range files {
		if holder, ok := c.fileLocks[file]; ok && holder == agentID {
			delete(c.fileLocks, file)
		}
	}
}

// IsFileLocked checks if a file is locked.
func (c *Coordinator) IsFileLocked(file string) (bool, string) {
	c.fileLocksMu.RLock()
	defer c.fileLocksMu.RUnlock()
	holder, locked := c.fileLocks[file]
	return locked, holder
}

// Registry returns the agent registry.
func (c *Coordinator) Registry() *Registry {
	return c.registry
}

// DroppedMessages returns the count of messages dropped due to full channels.
func (c *Coordinator) DroppedMessages() int64 {
	return c.droppedMessages.Load()
}
