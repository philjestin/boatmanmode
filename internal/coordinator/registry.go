// Package coordinator provides agent communication and coordination.
package coordinator

import (
	"context"
	"sync"
)

// AgentCapability describes what an agent can do.
type AgentCapability string

const (
	CapPlan       AgentCapability = "plan"
	CapExecute    AgentCapability = "execute"
	CapReview     AgentCapability = "review"
	CapRefactor   AgentCapability = "refactor"
	CapTest       AgentCapability = "test"
	CapValidate   AgentCapability = "validate"
	CapVerifyDiff AgentCapability = "verify_diff"
)

// AgentInfo describes a registered agent.
type AgentInfo struct {
	ID           string
	Name         string
	Capabilities []AgentCapability
	State        AgentState
	Priority     int // Higher priority agents get work first
}

// Agent is the interface that all coordinated agents must implement.
type Agent interface {
	// ID returns the unique identifier for this agent
	ID() string
	// Name returns a human-readable name
	Name() string
	// Capabilities returns what this agent can do
	Capabilities() []AgentCapability
	// Execute runs the agent with the given handoff
	Execute(ctx context.Context, handoff Handoff) (Handoff, error)
	// SetCoordinator gives the agent access to coordination
	SetCoordinator(c *Coordinator)
}

// Handoff is the interface for passing context between agents.
type Handoff interface {
	// Full returns the complete context
	Full() string
	// Concise returns a summary suitable for quick handoffs
	Concise() string
	// ForTokenBudget returns context sized to fit within token budget
	ForTokenBudget(maxTokens int) string
	// Type returns the handoff type for routing
	Type() string
}

// Registry manages agent registration and lookup.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

// NewRegistry creates a new agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*AgentInfo),
	}
}

// Register adds an agent to the registry.
func (r *Registry) Register(agent Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[agent.ID()] = &AgentInfo{
		ID:           agent.ID(),
		Name:         agent.Name(),
		Capabilities: agent.Capabilities(),
		State:        StateIdle,
	}
}

// Unregister removes an agent from the registry.
func (r *Registry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
}

// Get returns info about a specific agent.
func (r *Registry) Get(agentID string) (*AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.agents[agentID]
	return info, ok
}

// FindByCapability returns agents with a specific capability.
func (r *Registry) FindByCapability(cap AgentCapability) []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*AgentInfo
	for _, info := range r.agents {
		for _, c := range info.Capabilities {
			if c == cap {
				result = append(result, info)
				break
			}
		}
	}
	return result
}

// UpdateState updates an agent's state.
func (r *Registry) UpdateState(agentID string, state AgentState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info, ok := r.agents[agentID]; ok {
		info.State = state
	}
}

// AllIdle returns true if all agents are idle or complete.
func (r *Registry) AllIdle() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, info := range r.agents {
		if info.State == StateWorking || info.State == StateWaiting {
			return false
		}
	}
	return true
}

// List returns all registered agents.
func (r *Registry) List() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		result = append(result, info)
	}
	return result
}
