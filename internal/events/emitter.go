// Package events provides JSON event emission for boatmanapp integration.
// Events are emitted to stdout as newline-delimited JSON for the desktop app to parse.
package events

import (
	"encoding/json"
	"fmt"
	"os"
)

// Event represents a structured event emitted during workflow execution.
type Event struct {
	Type        string         `json:"type"`
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Message     string         `json:"message,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

// Emit writes a JSON event to stdout.
func Emit(event Event) {
	json, _ := json.Marshal(event)
	fmt.Fprintln(os.Stdout, string(json))
}

// AgentStarted emits an event when an agent begins execution.
func AgentStarted(id, name, description string) {
	Emit(Event{
		Type:        "agent_started",
		ID:          id,
		Name:        name,
		Description: description,
	})
}

// AgentCompleted emits an event when an agent finishes execution.
func AgentCompleted(id, name, status string) {
	Emit(Event{
		Type:   "agent_completed",
		ID:     id,
		Name:   name,
		Status: status,
	})
}

// AgentCompletedWithData emits an event when an agent finishes execution with additional metadata.
func AgentCompletedWithData(id, name, status string, data map[string]any) {
	Emit(Event{
		Type:   "agent_completed",
		ID:     id,
		Name:   name,
		Status: status,
		Data:   data,
	})
}

// TaskCreated emits an event when a task is created.
func TaskCreated(id, name, description string) {
	Emit(Event{
		Type:        "task_created",
		ID:          id,
		Name:        name,
		Description: description,
	})
}

// TaskUpdated emits an event when a task's status changes.
func TaskUpdated(id, status string) {
	Emit(Event{
		Type:   "task_updated",
		ID:     id,
		Status: status,
	})
}

// Progress emits a general progress message.
func Progress(message string) {
	Emit(Event{
		Type:    "progress",
		Message: message,
	})
}
