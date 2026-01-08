// Package coordinator provides agent communication and coordination.
// It ensures agents can work in parallel without duplicating work.
package coordinator

import (
	"time"
)

// MessageType identifies the kind of message being sent.
type MessageType string

const (
	// MsgClaimWork indicates an agent wants to claim a piece of work
	MsgClaimWork MessageType = "claim_work"
	// MsgWorkClaimed indicates work has been claimed by an agent
	MsgWorkClaimed MessageType = "work_claimed"
	// MsgWorkComplete indicates an agent finished its work
	MsgWorkComplete MessageType = "work_complete"
	// MsgWorkFailed indicates an agent's work failed
	MsgWorkFailed MessageType = "work_failed"
	// MsgStatusUpdate is a general status broadcast
	MsgStatusUpdate MessageType = "status_update"
	// MsgContextUpdate shares context with other agents
	MsgContextUpdate MessageType = "context_update"
	// MsgQuery asks for information from other agents
	MsgQuery MessageType = "query"
	// MsgQueryResponse responds to a query
	MsgQueryResponse MessageType = "query_response"
	// MsgWaitFor blocks until a condition is met
	MsgWaitFor MessageType = "wait_for"
)

// Message is the unit of communication between agents.
type Message struct {
	ID        string
	Type      MessageType
	From      string // Agent ID
	To        string // Target agent ID (empty for broadcast)
	Payload   interface{}
	Timestamp time.Time
}

// WorkClaim represents a request to claim specific work.
type WorkClaim struct {
	WorkID      string   // Unique identifier for the work
	WorkType    string   // Type of work (e.g., "modify_file", "run_tests")
	Description string   // Human-readable description
	Files       []string // Files involved (for conflict detection)
}

// WorkResult represents the outcome of completed work.
type WorkResult struct {
	WorkID    string
	Success   bool
	Output    interface{} // Type-specific result
	Error     string
	Duration  time.Duration
}

// StatusUpdate provides current agent status.
type StatusUpdate struct {
	State       AgentState
	CurrentWork string
	Progress    float64 // 0.0 to 1.0
	Message     string
}

// AgentState represents the current state of an agent.
type AgentState string

const (
	StateIdle       AgentState = "idle"
	StateWorking    AgentState = "working"
	StateWaiting    AgentState = "waiting"
	StateComplete   AgentState = "complete"
	StateFailed     AgentState = "failed"
)

// ContextUpdate shares context between agents.
type ContextUpdate struct {
	Key   string
	Value interface{}
	// Scope limits who receives this update
	Scope ContextScope
}

// ContextScope determines who receives context updates.
type ContextScope string

const (
	ScopeAll      ContextScope = "all"
	ScopeExecutor ContextScope = "executor"
	ScopeReviewer ContextScope = "reviewer"
)

// Query requests information from other agents.
type Query struct {
	QueryID   string
	QueryType string
	Data      interface{}
}

// QueryResponse answers a query.
type QueryResponse struct {
	QueryID string
	Data    interface{}
	Error   string
}

// WaitCondition specifies what to wait for.
type WaitCondition struct {
	// Type of wait condition
	Type WaitType
	// Target is what we're waiting for (agent ID, work ID, etc.)
	Target string
	// Timeout is how long to wait
	Timeout time.Duration
}

// WaitType specifies what kind of condition to wait for.
type WaitType string

const (
	WaitForAgent    WaitType = "agent"    // Wait for specific agent to complete
	WaitForWork     WaitType = "work"     // Wait for specific work to complete
	WaitForContext  WaitType = "context"  // Wait for context key to be set
	WaitForAllAgents WaitType = "all"     // Wait for all registered agents
)
