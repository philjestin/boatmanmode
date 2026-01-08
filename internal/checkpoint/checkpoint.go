// Package checkpoint provides progress saving and resume capabilities.
// It allows workflows to be resumed after failures or interruptions.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Step represents a step in the workflow.
type Step string

const (
	StepFetchTicket   Step = "fetch_ticket"
	StepCreateWorktree Step = "create_worktree"
	StepPlanning      Step = "planning"
	StepValidation    Step = "validation"
	StepExecution     Step = "execution"
	StepTesting       Step = "testing"
	StepReview        Step = "review"
	StepRefactor      Step = "refactor"
	StepVerify        Step = "verify"
	StepCommit        Step = "commit"
	StepPush          Step = "push"
	StepCreatePR      Step = "create_pr"
	StepComplete      Step = "complete"
)

// Status represents the status of a step.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusComplete   Status = "complete"
	StatusFailed     Status = "failed"
	StatusSkipped    Status = "skipped"
)

// Checkpoint represents saved state at a point in the workflow.
type Checkpoint struct {
	// ID is a unique identifier for this checkpoint
	ID string `json:"id"`
	// TicketID is the Linear ticket being worked on
	TicketID string `json:"ticket_id"`
	// WorktreePath is the path to the worktree
	WorktreePath string `json:"worktree_path"`
	// BranchName is the git branch
	BranchName string `json:"branch_name"`
	// CurrentStep is the current/next step to execute
	CurrentStep Step `json:"current_step"`
	// StepHistory tracks completed steps
	StepHistory []StepRecord `json:"step_history"`
	// Iteration is the current review iteration
	Iteration int `json:"iteration"`
	// MaxIterations is the maximum allowed iterations
	MaxIterations int `json:"max_iterations"`
	// State holds serialized state for the current step
	State json.RawMessage `json:"state,omitempty"`
	// CreatedAt is when the checkpoint was created
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the checkpoint was last updated
	UpdatedAt time.Time `json:"updated_at"`
	// Error holds any error message
	Error string `json:"error,omitempty"`
}

// StepRecord records completion of a step.
type StepRecord struct {
	Step       Step          `json:"step"`
	Status     Status        `json:"status"`
	StartedAt  time.Time     `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	Error      string        `json:"error,omitempty"`
	Output     interface{}   `json:"output,omitempty"`
}

// Manager handles checkpoint operations.
type Manager struct {
	// BaseDir is where checkpoints are stored
	BaseDir string
	// Current is the active checkpoint
	Current *Checkpoint
}

// NewManager creates a new checkpoint manager.
func NewManager(baseDir string) (*Manager, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(homeDir, ".boatman", "checkpoints")
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	return &Manager{BaseDir: baseDir}, nil
}

// Start begins a new checkpoint for a ticket.
func (m *Manager) Start(ticketID string, maxIterations int) *Checkpoint {
	now := time.Now()
	m.Current = &Checkpoint{
		ID:            fmt.Sprintf("%s-%d", ticketID, now.Unix()),
		TicketID:      ticketID,
		CurrentStep:   StepFetchTicket,
		StepHistory:   []StepRecord{},
		Iteration:     0,
		MaxIterations: maxIterations,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return m.Current
}

// Resume loads and resumes from a checkpoint.
func (m *Manager) Resume(checkpointID string) (*Checkpoint, error) {
	path := m.checkpointPath(checkpointID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	m.Current = &cp
	return &cp, nil
}

// ResumeLatest resumes the most recent checkpoint for a ticket.
func (m *Manager) ResumeLatest(ticketID string) (*Checkpoint, error) {
	checkpoints, err := m.ListForTicket(ticketID)
	if err != nil {
		return nil, err
	}

	if len(checkpoints) == 0 {
		return nil, fmt.Errorf("no checkpoints found for ticket %s", ticketID)
	}

	// Sort by updated time, most recent first
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].UpdatedAt.After(checkpoints[j].UpdatedAt)
	})

	return m.Resume(checkpoints[0].ID)
}

// BeginStep marks the start of a step.
func (m *Manager) BeginStep(step Step) {
	if m.Current == nil {
		return
	}

	record := StepRecord{
		Step:      step,
		Status:    StatusInProgress,
		StartedAt: time.Now(),
	}

	m.Current.CurrentStep = step
	m.Current.StepHistory = append(m.Current.StepHistory, record)
	m.Current.UpdatedAt = time.Now()

	m.Save()
}

// CompleteStep marks a step as complete.
func (m *Manager) CompleteStep(step Step, output interface{}) {
	if m.Current == nil {
		return
	}

	now := time.Now()

	// Find and update the step record
	for i := len(m.Current.StepHistory) - 1; i >= 0; i-- {
		if m.Current.StepHistory[i].Step == step && m.Current.StepHistory[i].Status == StatusInProgress {
			m.Current.StepHistory[i].Status = StatusComplete
			m.Current.StepHistory[i].CompletedAt = now
			m.Current.StepHistory[i].Duration = now.Sub(m.Current.StepHistory[i].StartedAt)
			m.Current.StepHistory[i].Output = output
			break
		}
	}

	m.Current.UpdatedAt = now
	m.Save()
}

// FailStep marks a step as failed.
func (m *Manager) FailStep(step Step, err error) {
	if m.Current == nil {
		return
	}

	now := time.Now()

	// Find and update the step record
	for i := len(m.Current.StepHistory) - 1; i >= 0; i-- {
		if m.Current.StepHistory[i].Step == step && m.Current.StepHistory[i].Status == StatusInProgress {
			m.Current.StepHistory[i].Status = StatusFailed
			m.Current.StepHistory[i].CompletedAt = now
			m.Current.StepHistory[i].Duration = now.Sub(m.Current.StepHistory[i].StartedAt)
			m.Current.StepHistory[i].Error = err.Error()
			break
		}
	}

	m.Current.Error = err.Error()
	m.Current.UpdatedAt = now
	m.Save()
}

// SetWorktree records the worktree path.
func (m *Manager) SetWorktree(path, branchName string) {
	if m.Current == nil {
		return
	}
	m.Current.WorktreePath = path
	m.Current.BranchName = branchName
	m.Save()
}

// SetIteration updates the current iteration.
func (m *Manager) SetIteration(iteration int) {
	if m.Current == nil {
		return
	}
	m.Current.Iteration = iteration
	m.Save()
}

// SaveState saves arbitrary state for the current step.
func (m *Manager) SaveState(state interface{}) error {
	if m.Current == nil {
		return nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	m.Current.State = data
	return m.Save()
}

// LoadState loads the saved state into the given struct.
func (m *Manager) LoadState(into interface{}) error {
	if m.Current == nil || m.Current.State == nil {
		return nil
	}

	return json.Unmarshal(m.Current.State, into)
}

// Save persists the current checkpoint to disk.
func (m *Manager) Save() error {
	if m.Current == nil {
		return nil
	}

	m.Current.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(m.Current, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	path := m.checkpointPath(m.Current.ID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	return nil
}

// Delete removes a checkpoint.
func (m *Manager) Delete(checkpointID string) error {
	path := m.checkpointPath(checkpointID)
	return os.Remove(path)
}

// Cleanup removes old checkpoints.
func (m *Manager) Cleanup(maxAge time.Duration) error {
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(m.BaseDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}

		if cp.UpdatedAt.Before(cutoff) {
			os.Remove(path)
		}
	}

	return nil
}

// List returns all checkpoints.
func (m *Manager) List() ([]Checkpoint, error) {
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		return nil, err
	}

	var checkpoints []Checkpoint

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(m.BaseDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}

		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, nil
}

// ListForTicket returns checkpoints for a specific ticket.
func (m *Manager) ListForTicket(ticketID string) ([]Checkpoint, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}

	var filtered []Checkpoint
	for _, cp := range all {
		if cp.TicketID == ticketID {
			filtered = append(filtered, cp)
		}
	}

	return filtered, nil
}

// HasIncompleteCheckpoint checks if there's an incomplete checkpoint for a ticket.
func (m *Manager) HasIncompleteCheckpoint(ticketID string) bool {
	checkpoints, err := m.ListForTicket(ticketID)
	if err != nil {
		return false
	}

	for _, cp := range checkpoints {
		if cp.CurrentStep != StepComplete && cp.Error == "" {
			return true
		}
	}

	return false
}

// GetProgress returns a summary of progress.
func (m *Manager) GetProgress() Progress {
	if m.Current == nil {
		return Progress{}
	}

	p := Progress{
		TicketID:     m.Current.TicketID,
		CurrentStep:  m.Current.CurrentStep,
		Iteration:    m.Current.Iteration,
		MaxIterations: m.Current.MaxIterations,
		StepsComplete: 0,
		TotalDuration: time.Duration(0),
	}

	for _, record := range m.Current.StepHistory {
		if record.Status == StatusComplete {
			p.StepsComplete++
			p.TotalDuration += record.Duration
		}
	}

	return p
}

// Progress represents current progress.
type Progress struct {
	TicketID      string
	CurrentStep   Step
	Iteration     int
	MaxIterations int
	StepsComplete int
	TotalDuration time.Duration
}

// FormatProgress returns a formatted progress string.
func (p Progress) FormatProgress() string {
	steps := []Step{
		StepFetchTicket, StepCreateWorktree, StepPlanning, StepValidation,
		StepExecution, StepTesting, StepReview, StepCommit, StepCreatePR,
	}

	currentIdx := 0
	for i, s := range steps {
		if s == p.CurrentStep {
			currentIdx = i
			break
		}
	}

	return fmt.Sprintf(
		"[%s] Step %d/%d: %s (iteration %d/%d, elapsed: %s)",
		p.TicketID,
		currentIdx+1,
		len(steps),
		p.CurrentStep,
		p.Iteration,
		p.MaxIterations,
		p.TotalDuration.Round(time.Second),
	)
}

// checkpointPath returns the file path for a checkpoint.
func (m *Manager) checkpointPath(id string) string {
	return filepath.Join(m.BaseDir, id+".json")
}

// CanResume checks if a checkpoint can be resumed.
func (cp *Checkpoint) CanResume() bool {
	// Can resume if not complete and not failed
	if cp.CurrentStep == StepComplete {
		return false
	}

	// Check for fatal steps that can't be resumed
	nonResumableSteps := map[Step]bool{
		StepCommit:   true, // Commit is atomic
		StepPush:     true, // Push is atomic
		StepCreatePR: true, // PR creation is atomic
	}

	// If last step failed and it's non-resumable, can't resume
	if len(cp.StepHistory) > 0 {
		lastRecord := cp.StepHistory[len(cp.StepHistory)-1]
		if lastRecord.Status == StatusFailed && nonResumableSteps[lastRecord.Step] {
			return false
		}
	}

	return true
}

// GetResumePoint returns the step to resume from.
func (cp *Checkpoint) GetResumePoint() Step {
	if len(cp.StepHistory) == 0 {
		return StepFetchTicket
	}

	// Find the last successful step and return the next one
	for i := len(cp.StepHistory) - 1; i >= 0; i-- {
		record := cp.StepHistory[i]
		if record.Status == StatusComplete {
			return getNextStep(record.Step)
		}
	}

	// If no complete steps, start from beginning
	return StepFetchTicket
}

// getNextStep returns the next step in the workflow.
func getNextStep(current Step) Step {
	order := []Step{
		StepFetchTicket, StepCreateWorktree, StepPlanning, StepValidation,
		StepExecution, StepTesting, StepReview, StepRefactor, StepVerify,
		StepCommit, StepPush, StepCreatePR, StepComplete,
	}

	for i, s := range order {
		if s == current && i < len(order)-1 {
			return order[i+1]
		}
	}

	return StepComplete
}

// FormatCheckpoint returns a formatted summary.
func (cp *Checkpoint) FormatCheckpoint() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Checkpoint: %s\n", cp.ID))
	sb.WriteString(fmt.Sprintf("  Ticket: %s\n", cp.TicketID))
	sb.WriteString(fmt.Sprintf("  Branch: %s\n", cp.BranchName))
	sb.WriteString(fmt.Sprintf("  Current Step: %s\n", cp.CurrentStep))
	sb.WriteString(fmt.Sprintf("  Iteration: %d/%d\n", cp.Iteration, cp.MaxIterations))
	sb.WriteString(fmt.Sprintf("  Created: %s\n", cp.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("  Updated: %s\n", cp.UpdatedAt.Format(time.RFC3339)))

	if cp.Error != "" {
		sb.WriteString(fmt.Sprintf("  Error: %s\n", cp.Error))
	}

	sb.WriteString("  Step History:\n")
	for _, record := range cp.StepHistory {
		status := "⏳"
		switch record.Status {
		case StatusComplete:
			status = "✅"
		case StatusFailed:
			status = "❌"
		case StatusSkipped:
			status = "⏭️"
		}
		sb.WriteString(fmt.Sprintf("    %s %s", status, record.Step))
		if record.Duration > 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", record.Duration.Round(time.Second)))
		}
		if record.Error != "" {
			sb.WriteString(fmt.Sprintf(" - %s", record.Error))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
