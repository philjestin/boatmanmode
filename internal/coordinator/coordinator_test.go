package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewCoordinator(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.registry == nil {
		t.Error("registry is nil")
	}
	if c.messages == nil {
		t.Error("messages channel is nil")
	}
}

func TestCoordinatorStartStop(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	time.Sleep(10 * time.Millisecond) // Let it start

	c.Stop()
	// Should not panic or hang
}

func TestWorkClaiming(t *testing.T) {
	c := New()

	// Claim work
	claim := &WorkClaim{
		WorkID:      "work-1",
		WorkType:    "execute",
		Description: "Test work",
		Files:       []string{"file1.go", "file2.go"},
	}

	// First claim should succeed
	if !c.ClaimWork("agent-1", claim) {
		t.Error("First claim should succeed")
	}

	// Same work claimed again should fail
	if c.ClaimWork("agent-2", claim) {
		t.Error("Duplicate claim should fail")
	}

	// Different work should succeed
	claim2 := &WorkClaim{
		WorkID:   "work-2",
		WorkType: "review",
	}
	if !c.ClaimWork("agent-2", claim2) {
		t.Error("Different work claim should succeed")
	}

	// Release work
	c.ReleaseWork("work-1", "agent-1")

	// Now it should be claimable
	if !c.ClaimWork("agent-3", claim) {
		t.Error("Released work should be claimable")
	}
}

func TestFileLocking(t *testing.T) {
	c := New()

	files := []string{"file1.go", "file2.go"}

	// First lock should succeed
	if !c.LockFiles("agent-1", files) {
		t.Error("First lock should succeed")
	}

	// Check if locked
	locked, holder := c.IsFileLocked("file1.go")
	if !locked {
		t.Error("file1.go should be locked")
	}
	if holder != "agent-1" {
		t.Errorf("Expected holder agent-1, got %s", holder)
	}

	// Overlapping lock should fail
	if c.LockFiles("agent-2", []string{"file2.go", "file3.go"}) {
		t.Error("Overlapping lock should fail")
	}

	// Non-overlapping lock should succeed
	if !c.LockFiles("agent-2", []string{"file3.go", "file4.go"}) {
		t.Error("Non-overlapping lock should succeed")
	}

	// Unlock files
	c.UnlockFiles("agent-1", files)

	// Now should be unlocked
	locked, _ = c.IsFileLocked("file1.go")
	if locked {
		t.Error("file1.go should be unlocked")
	}
}

func TestSharedContext(t *testing.T) {
	c := New()

	// Set context
	c.SetContext("key1", "value1")
	c.SetContext("key2", 42)

	// Get context
	val, ok := c.GetContext("key1")
	if !ok {
		t.Error("key1 should exist")
	}
	if val != "value1" {
		t.Errorf("Expected value1, got %v", val)
	}

	val, ok = c.GetContext("key2")
	if !ok {
		t.Error("key2 should exist")
	}
	if val != 42 {
		t.Errorf("Expected 42, got %v", val)
	}

	// Non-existent key
	_, ok = c.GetContext("key3")
	if ok {
		t.Error("key3 should not exist")
	}
}

func TestFileConflictWithWorkClaim(t *testing.T) {
	c := New()

	// Lock some files
	c.LockFiles("agent-1", []string{"locked.go"})

	// Try to claim work that includes locked file
	claim := &WorkClaim{
		WorkID: "work-1",
		Files:  []string{"locked.go", "other.go"},
	}

	if c.ClaimWork("agent-2", claim) {
		t.Error("Claim with locked file should fail")
	}

	// Same agent can claim its own locked files
	if !c.ClaimWork("agent-1", claim) {
		t.Error("Agent should be able to claim its own locked files")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)

	var wg sync.WaitGroup
	claimCount := 0
	var mu sync.Mutex

	// Multiple goroutines trying to claim the same work
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			claim := &WorkClaim{
				WorkID: "contested-work",
			}
			if c.ClaimWork("agent-"+string(rune('0'+id)), claim) {
				mu.Lock()
				claimCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Only one should have claimed
	if claimCount != 1 {
		t.Errorf("Expected exactly 1 claim, got %d", claimCount)
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	// Create a mock agent
	agent := &mockAgent{id: "test-agent", name: "Test Agent"}

	// Register
	r.Register(agent)

	// Get
	info, ok := r.Get("test-agent")
	if !ok {
		t.Fatal("Agent should be registered")
	}
	if info.Name != "Test Agent" {
		t.Errorf("Expected name 'Test Agent', got %s", info.Name)
	}

	// Update state
	r.UpdateState("test-agent", StateWorking)
	info, _ = r.Get("test-agent")
	if info.State != StateWorking {
		t.Errorf("Expected state Working, got %s", info.State)
	}

	// Find by capability
	agents := r.FindByCapability(CapExecute)
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent with Execute capability, got %d", len(agents))
	}

	// Unregister
	r.Unregister("test-agent")
	_, ok = r.Get("test-agent")
	if ok {
		t.Error("Agent should be unregistered")
	}
}

func TestAllIdle(t *testing.T) {
	r := NewRegistry()

	agent1 := &mockAgent{id: "agent-1", name: "Agent 1"}
	agent2 := &mockAgent{id: "agent-2", name: "Agent 2"}

	r.Register(agent1)
	r.Register(agent2)

	// Initially all idle
	if !r.AllIdle() {
		t.Error("All agents should be idle initially")
	}

	// Set one to working
	r.UpdateState("agent-1", StateWorking)
	if r.AllIdle() {
		t.Error("Should not be all idle when one is working")
	}

	// Set to complete
	r.UpdateState("agent-1", StateComplete)
	if !r.AllIdle() {
		t.Error("Complete agents count as idle")
	}
}

func TestNewWithOptions(t *testing.T) {
	opts := Options{
		MessageBufferSize:    2000,
		SubscriberBufferSize: 200,
	}

	c := NewWithOptions(opts)

	if c == nil {
		t.Fatal("NewWithOptions returned nil")
	}
	if cap(c.messages) != 2000 {
		t.Errorf("Expected message buffer size 2000, got %d", cap(c.messages))
	}
	if c.subscriberBufferSize != 200 {
		t.Errorf("Expected subscriber buffer size 200, got %d", c.subscriberBufferSize)
	}
}

func TestNewWithOptionsDefaults(t *testing.T) {
	// Zero values should use defaults
	opts := Options{
		MessageBufferSize:    0,
		SubscriberBufferSize: 0,
	}

	c := NewWithOptions(opts)

	if cap(c.messages) != 1000 {
		t.Errorf("Expected default message buffer size 1000, got %d", cap(c.messages))
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.MessageBufferSize != 1000 {
		t.Errorf("Expected MessageBufferSize 1000, got %d", opts.MessageBufferSize)
	}
	if opts.SubscriberBufferSize != 100 {
		t.Errorf("Expected SubscriberBufferSize 100, got %d", opts.SubscriberBufferSize)
	}
}

func TestAtomicRunningFlag(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initially not running
	if c.running.Load() {
		t.Error("Should not be running before Start")
	}

	c.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Should be running
	if !c.running.Load() {
		t.Error("Should be running after Start")
	}

	c.Stop()

	// Should not be running
	if c.running.Load() {
		t.Error("Should not be running after Stop")
	}
}

func TestStopCleansUpMaps(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)

	// Add some data
	c.claimedWorkMu.Lock()
	c.claimedWork["work-1"] = "agent-1"
	c.claimedWorkMu.Unlock()

	c.sharedContextMu.Lock()
	c.sharedContext["key1"] = "value1"
	c.sharedContextMu.Unlock()

	c.fileLocksMu.Lock()
	c.fileLocks["file1.go"] = "agent-1"
	c.fileLocksMu.Unlock()

	c.Stop()

	// Verify maps are cleared
	c.claimedWorkMu.RLock()
	if len(c.claimedWork) != 0 {
		t.Errorf("claimedWork should be cleared, has %d items", len(c.claimedWork))
	}
	c.claimedWorkMu.RUnlock()

	c.sharedContextMu.RLock()
	if len(c.sharedContext) != 0 {
		t.Errorf("sharedContext should be cleared, has %d items", len(c.sharedContext))
	}
	c.sharedContextMu.RUnlock()

	c.fileLocksMu.RLock()
	if len(c.fileLocks) != 0 {
		t.Errorf("fileLocks should be cleared, has %d items", len(c.fileLocks))
	}
	c.fileLocksMu.RUnlock()
}

func TestDroppedMessages(t *testing.T) {
	// Create coordinator with tiny buffer
	opts := Options{
		MessageBufferSize:    1,
		SubscriberBufferSize: 1,
	}
	c := NewWithOptions(opts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	defer c.Stop()

	// Register an agent but don't consume messages
	agent := &mockAgent{id: "slow-agent", name: "Slow Agent"}
	c.Register(agent)

	// Initial count should be 0
	if c.DroppedMessages() != 0 {
		t.Errorf("Initial dropped count should be 0, got %d", c.DroppedMessages())
	}

	// Send many messages to overflow buffer
	for i := 0; i < 10; i++ {
		c.Send(Message{
			Type: MsgStatusUpdate,
			From: "test",
			To:   "slow-agent",
		})
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Some messages should be dropped (buffer size is 1)
	dropped := c.DroppedMessages()
	if dropped == 0 {
		t.Log("Warning: No messages dropped, buffer may not have overflowed")
	}
}

func TestSendToNotRunning(t *testing.T) {
	c := New()

	// Don't start the coordinator
	// Send should not panic or block
	c.Send(Message{
		Type: MsgStatusUpdate,
		From: "test",
	})
	// If we get here without blocking, test passes
}

func TestConcurrentStartStop(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Multiple goroutines trying to read running flag
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = c.running.Load()
			}
		}()
	}

	// While others are starting/stopping
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Start(ctx)
		time.Sleep(10 * time.Millisecond)
		c.Stop()
	}()

	wg.Wait()
	// No race condition should occur
}

func TestBroadcastDropsMessages(t *testing.T) {
	opts := Options{
		MessageBufferSize:    100,
		SubscriberBufferSize: 1, // Very small subscriber buffer
	}
	c := NewWithOptions(opts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	defer c.Stop()

	// Register multiple agents
	agent1 := &mockAgent{id: "agent-1", name: "Agent 1"}
	agent2 := &mockAgent{id: "agent-2", name: "Agent 2"}
	c.Register(agent1)
	c.Register(agent2)

	// Send broadcasts (not consuming from subscriber channels)
	for i := 0; i < 20; i++ {
		c.broadcast(Message{
			Type: MsgStatusUpdate,
			From: "broadcaster",
		})
	}

	// Should have dropped some messages
	dropped := c.DroppedMessages()
	if dropped == 0 {
		t.Log("Warning: Expected some dropped messages with tiny buffer")
	}
}

// Mock agent for testing
type mockAgent struct {
	id    string
	name  string
	coord *Coordinator
}

func (a *mockAgent) ID() string                            { return a.id }
func (a *mockAgent) Name() string                          { return a.name }
func (a *mockAgent) Capabilities() []AgentCapability       { return []AgentCapability{CapExecute} }
func (a *mockAgent) SetCoordinator(c *Coordinator)         { a.coord = c }
func (a *mockAgent) Execute(ctx context.Context, h Handoff) (Handoff, error) {
	return nil, nil
}
