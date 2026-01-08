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
