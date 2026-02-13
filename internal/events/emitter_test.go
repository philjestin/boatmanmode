package events

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestAgentStarted(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	AgentStarted("test-123", "Test Agent", "Testing agent events")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("Failed to parse event JSON: %v", err)
	}

	if event.Type != "agent_started" {
		t.Errorf("Expected type 'agent_started', got '%s'", event.Type)
	}
	if event.ID != "test-123" {
		t.Errorf("Expected ID 'test-123', got '%s'", event.ID)
	}
	if event.Name != "Test Agent" {
		t.Errorf("Expected name 'Test Agent', got '%s'", event.Name)
	}
	if event.Description != "Testing agent events" {
		t.Errorf("Expected description 'Testing agent events', got '%s'", event.Description)
	}
}

func TestAgentCompleted(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	AgentCompleted("test-123", "Test Agent", "success")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("Failed to parse event JSON: %v", err)
	}

	if event.Type != "agent_completed" {
		t.Errorf("Expected type 'agent_completed', got '%s'", event.Type)
	}
	if event.ID != "test-123" {
		t.Errorf("Expected ID 'test-123', got '%s'", event.ID)
	}
	if event.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", event.Status)
	}
}

func TestTaskCreated(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	TaskCreated("task-1", "Implement feature", "Add new API endpoint")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("Failed to parse event JSON: %v", err)
	}

	if event.Type != "task_created" {
		t.Errorf("Expected type 'task_created', got '%s'", event.Type)
	}
	if event.ID != "task-1" {
		t.Errorf("Expected ID 'task-1', got '%s'", event.ID)
	}
}

func TestTaskUpdated(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	TaskUpdated("task-1", "in_progress")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("Failed to parse event JSON: %v", err)
	}

	if event.Type != "task_updated" {
		t.Errorf("Expected type 'task_updated', got '%s'", event.Type)
	}
	if event.Status != "in_progress" {
		t.Errorf("Expected status 'in_progress', got '%s'", event.Status)
	}
}

func TestProgress(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Progress("Running tests...")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("Failed to parse event JSON: %v", err)
	}

	if event.Type != "progress" {
		t.Errorf("Expected type 'progress', got '%s'", event.Type)
	}
	if event.Message != "Running tests..." {
		t.Errorf("Expected message 'Running tests...', got '%s'", event.Message)
	}
}
