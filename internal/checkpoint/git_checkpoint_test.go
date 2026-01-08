package checkpoint

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGitCheckpointManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-checkpoint-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})
	if err != nil {
		t.Fatalf("NewGitCheckpointManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("Manager should not be nil")
	}
	if !mgr.useGit {
		t.Error("useGit should be true")
	}
	if mgr.commitPrefix != "[checkpoint]" {
		t.Errorf("Expected default prefix '[checkpoint]', got %s", mgr.commitPrefix)
	}
}

func TestGitCheckpointManagerWithCustomPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git-checkpoint-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		CommitPrefix: "[boatman]",
		BaseDir:      tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if mgr.commitPrefix != "[boatman]" {
		t.Errorf("Expected prefix '[boatman]', got %s", mgr.commitPrefix)
	}
}

func setupGitRepo(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "git-checkpoint-test")
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	initialFile := filepath.Join(tmpDir, "README.md")
	os.WriteFile(initialFile, []byte("# Test Repo"), 0644)
	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	cmd.Run()

	return tmpDir
}

func TestCommitCheckpoint(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	// Start a checkpoint
	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	// Create some changes
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)

	// Commit checkpoint
	err := mgr.CommitCheckpoint("test checkpoint")
	if err != nil {
		t.Fatalf("CommitCheckpoint failed: %v", err)
	}

	// Verify commit was created
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	if !strings.Contains(string(output), "[checkpoint]") {
		t.Errorf("Commit should contain checkpoint prefix, got: %s", output)
	}
}

func TestBeginStepWithCommit(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	err := mgr.BeginStepWithCommit(StepExecution)
	if err != nil {
		t.Fatalf("BeginStepWithCommit failed: %v", err)
	}

	if mgr.Current.CurrentStep != StepExecution {
		t.Errorf("Expected step execution, got %s", mgr.Current.CurrentStep)
	}

	// Verify commit message
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	if !strings.Contains(string(output), "begin execution") {
		t.Errorf("Commit should contain 'begin execution', got: %s", output)
	}
}

func TestCompleteStepWithCommit(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	mgr.BeginStep(StepExecution)

	// Create some changes
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)

	err := mgr.CompleteStepWithCommit(StepExecution, map[string]string{"result": "success"})
	if err != nil {
		t.Fatalf("CompleteStepWithCommit failed: %v", err)
	}

	// Verify commit
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()

	if !strings.Contains(string(output), "complete execution") {
		t.Errorf("Commit should contain 'complete execution', got: %s", output)
	}
}

func TestRollback(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	// Create first checkpoint
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("v1"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	// Create second checkpoint
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("v2"), 0644)
	mgr.CompleteStepWithCommit(StepExecution, nil)

	// Create third checkpoint
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("v3"), 0644)
	mgr.BeginStepWithCommit(StepReview)

	// Verify current content
	content, _ := os.ReadFile(filepath.Join(tmpDir, "file1.go"))
	if string(content) != "v3" {
		t.Errorf("Expected v3, got %s", content)
	}

	// Rollback 1 step
	err := mgr.Rollback(1)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify content is rolled back
	content, _ = os.ReadFile(filepath.Join(tmpDir, "file1.go"))
	if string(content) != "v2" {
		t.Errorf("Expected v2 after rollback, got %s", content)
	}
}

func TestGetGitHistory(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	// Create several checkpoints
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("step1"), 0644)
	mgr.BeginStepWithCommit(StepPlanning)
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("step2"), 0644)
	mgr.CompleteStepWithCommit(StepPlanning, nil)
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("step3"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	history, err := mgr.GetGitHistory()
	if err != nil {
		t.Fatalf("GetGitHistory failed: %v", err)
	}

	if len(history) < 3 {
		t.Errorf("Expected at least 3 history entries, got %d", len(history))
	}
}

func TestGetCheckpointAtCommit(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)
	mgr.SetIteration(1)

	// Create a checkpoint
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("content"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	// Get the commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()
	commitHash := strings.TrimSpace(string(output))

	// Change state
	mgr.SetIteration(2)
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("new content"), 0644)
	mgr.CompleteStepWithCommit(StepExecution, nil)

	// Get checkpoint at old commit
	cp, err := mgr.GetCheckpointAtCommit(commitHash)
	if err != nil {
		t.Fatalf("GetCheckpointAtCommit failed: %v", err)
	}

	if cp.Iteration != 1 {
		t.Errorf("Expected iteration 1 at old commit, got %d", cp.Iteration)
	}
}

func TestCreateSnapshotBranch(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	// Create some work
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("snapshot"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	// Create snapshot branch
	err := mgr.CreateSnapshotBranch("before-review")
	if err != nil {
		t.Fatalf("CreateSnapshotBranch failed: %v", err)
	}

	// Verify branch exists
	cmd := exec.Command("git", "branch", "--list", "checkpoint/ENG-123/before-review")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()

	if !strings.Contains(string(output), "checkpoint/ENG-123/before-review") {
		t.Error("Snapshot branch should exist")
	}
}

func TestListSnapshotBranches(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	// Create some work
	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("v1"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	// Create branches
	mgr.CreateSnapshotBranch("snapshot-1")
	mgr.CreateSnapshotBranch("snapshot-2")

	branches, err := mgr.ListSnapshotBranches()
	if err != nil {
		t.Fatalf("ListSnapshotBranches failed: %v", err)
	}

	if len(branches) < 2 {
		t.Errorf("Expected at least 2 branches, got %d", len(branches))
	}
}

func TestFormatGitHistory(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("test"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	formatted, err := mgr.FormatGitHistory()
	if err != nil {
		t.Fatalf("FormatGitHistory failed: %v", err)
	}

	if formatted == "" {
		t.Error("Formatted history should not be empty")
	}
	if !strings.Contains(formatted, "Checkpoint History") {
		t.Error("Should contain header")
	}
}

func TestParseCommitMessage(t *testing.T) {
	mgr := &GitCheckpointManager{commitPrefix: "[checkpoint]"}

	tests := []struct {
		msg       string
		wantStep  Step
		wantIter  int
	}{
		{
			"[checkpoint] ENG-123: begin execution (step: execution, iter: 1)",
			StepExecution,
			1,
		},
		{
			"[checkpoint] ENG-123: complete review (step: review, iter: 3)",
			StepReview,
			3,
		},
		{
			"regular commit message",
			"",
			0,
		},
	}

	for _, tt := range tests {
		step, iter := mgr.parseCommitMessage(tt.msg)
		if step != tt.wantStep {
			t.Errorf("parseCommitMessage(%q) step = %s, want %s", tt.msg, step, tt.wantStep)
		}
		if iter != tt.wantIter {
			t.Errorf("parseCommitMessage(%q) iter = %d, want %d", tt.msg, iter, tt.wantIter)
		}
	}
}

func TestExportHistory(t *testing.T) {
	tmpDir := setupGitRepo(t)
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       true,
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)
	mgr.SetWorktreePath(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("test"), 0644)
	mgr.BeginStepWithCommit(StepExecution)

	exportPath := filepath.Join(tmpDir, "history.json")
	err := mgr.ExportHistory(exportPath)
	if err != nil {
		t.Fatalf("ExportHistory failed: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	var history []GitHistory
	if err := json.Unmarshal(data, &history); err != nil {
		t.Fatalf("Exported file is not valid JSON: %v", err)
	}
}

func TestGitNotEnabled(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "git-checkpoint-test")
	defer os.RemoveAll(tmpDir)

	mgr, _ := NewGitCheckpointManager(GitCheckpointOptions{
		WorktreePath: tmpDir,
		UseGit:       false, // Disabled
		BaseDir:      tmpDir,
	})

	mgr.Start("ENG-123", 3)

	// These should not error, just no-op
	err := mgr.CommitCheckpoint("test")
	if err != nil {
		t.Error("CommitCheckpoint should not error when git disabled")
	}

	// Rollback should error when git not enabled
	err = mgr.Rollback(1)
	if err == nil {
		t.Error("Rollback should error when git disabled")
	}
}
