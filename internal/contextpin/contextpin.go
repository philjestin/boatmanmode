// Package contextpin provides file dependency tracking for multi-file changes.
// It ensures that when modifying multiple related files, all dependencies
// are fresh and locked to prevent race conditions.
package contextpin

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/philjestin/boatmanmode/internal/coordinator"
)

// Pin represents a context pin for coordinated file access.
type Pin struct {
	// Files that are pinned together
	Files []string
	// Checksums at pin time
	Checksums map[string]string
	// Contents at pin time (for small files)
	Contents map[string]string
	// AgentID that holds the pin
	AgentID string
	// Locked indicates if files are locked for exclusive access
	Locked bool
}

// DependencyGraph tracks relationships between files.
type DependencyGraph struct {
	mu           sync.RWMutex
	dependencies map[string][]string // file -> files it depends on
	dependents   map[string][]string // file -> files that depend on it
	checksums    map[string]string   // file -> content checksum
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		dependencies: make(map[string][]string),
		dependents:   make(map[string][]string),
		checksums:    make(map[string]string),
	}
}

// ContextPinner manages file context and dependencies.
type ContextPinner struct {
	worktreePath string
	graph        *DependencyGraph
	coord        *coordinator.Coordinator
	pins         map[string]*Pin // agentID -> pin
	pinsMu       sync.RWMutex
}

// New creates a new ContextPinner.
func New(worktreePath string) *ContextPinner {
	return &ContextPinner{
		worktreePath: worktreePath,
		graph:        NewDependencyGraph(),
		pins:         make(map[string]*Pin),
	}
}

// SetCoordinator sets the coordinator for file locking.
func (cp *ContextPinner) SetCoordinator(c *coordinator.Coordinator) {
	cp.coord = c
}

// AnalyzeFile extracts dependencies from a file.
func (cp *ContextPinner) AnalyzeFile(relPath string) ([]string, error) {
	fullPath := filepath.Join(cp.worktreePath, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(relPath)
	deps := extractDependencies(string(content), ext)

	// Resolve relative imports to full paths
	dir := filepath.Dir(relPath)
	resolved := make([]string, 0, len(deps))
	for _, dep := range deps {
		resolvedPath := resolveDependency(dep, dir, ext, cp.worktreePath)
		if resolvedPath != "" {
			resolved = append(resolved, resolvedPath)
		}
	}

	// Update graph
	cp.graph.mu.Lock()
	cp.graph.dependencies[relPath] = resolved
	for _, dep := range resolved {
		cp.graph.dependents[dep] = appendUnique(cp.graph.dependents[dep], relPath)
	}
	cp.graph.mu.Unlock()

	return resolved, nil
}

// AnalyzeFiles analyzes multiple files and builds the dependency graph.
func (cp *ContextPinner) AnalyzeFiles(files []string) error {
	for _, file := range files {
		if _, err := cp.AnalyzeFile(file); err != nil {
			// Log but continue - file might not exist yet
			continue
		}
	}
	return nil
}

// GetRelatedFiles returns all files related to the given file.
// This includes both dependencies and dependents.
func (cp *ContextPinner) GetRelatedFiles(file string) []string {
	cp.graph.mu.RLock()
	defer cp.graph.mu.RUnlock()

	seen := make(map[string]bool)
	seen[file] = true

	// BFS to find all related files
	queue := []string{file}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Add dependencies
		for _, dep := range cp.graph.dependencies[current] {
			if !seen[dep] {
				seen[dep] = true
				queue = append(queue, dep)
			}
		}

		// Add dependents
		for _, dep := range cp.graph.dependents[current] {
			if !seen[dep] {
				seen[dep] = true
				queue = append(queue, dep)
			}
		}
	}

	result := make([]string, 0, len(seen))
	for f := range seen {
		result = append(result, f)
	}
	return result
}

// GetDependencies returns files that the given file depends on.
func (cp *ContextPinner) GetDependencies(file string) []string {
	cp.graph.mu.RLock()
	defer cp.graph.mu.RUnlock()
	return cp.graph.dependencies[file]
}

// GetDependents returns files that depend on the given file.
func (cp *ContextPinner) GetDependents(file string) []string {
	cp.graph.mu.RLock()
	defer cp.graph.mu.RUnlock()
	return cp.graph.dependents[file]
}

// Pin creates a context pin for a set of files.
// If lock is true, files are locked for exclusive access.
func (cp *ContextPinner) Pin(agentID string, files []string, lock bool) (*Pin, error) {
	// Expand to include all related files
	allFiles := make(map[string]bool)
	for _, f := range files {
		allFiles[f] = true
		for _, related := range cp.GetRelatedFiles(f) {
			allFiles[related] = true
		}
	}

	fileList := make([]string, 0, len(allFiles))
	for f := range allFiles {
		fileList = append(fileList, f)
	}

	// Try to lock files if coordinated
	if lock && cp.coord != nil {
		if !cp.coord.LockFiles(agentID, fileList) {
			return nil, &FileLockError{Files: fileList}
		}
	}

	// Create pin with checksums and content
	pin := &Pin{
		Files:     fileList,
		Checksums: make(map[string]string),
		Contents:  make(map[string]string),
		AgentID:   agentID,
		Locked:    lock,
	}

	for _, file := range fileList {
		fullPath := filepath.Join(cp.worktreePath, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue // File might not exist yet
		}
		pin.Checksums[file] = checksum(content)
		// Only store content for small files
		if len(content) < 10000 {
			pin.Contents[file] = string(content)
		}
	}

	cp.pinsMu.Lock()
	cp.pins[agentID] = pin
	cp.pinsMu.Unlock()

	return pin, nil
}

// Unpin releases a context pin.
func (cp *ContextPinner) Unpin(agentID string) {
	cp.pinsMu.Lock()
	pin, ok := cp.pins[agentID]
	if ok {
		delete(cp.pins, agentID)
	}
	cp.pinsMu.Unlock()

	if ok && pin.Locked && cp.coord != nil {
		cp.coord.UnlockFiles(agentID, pin.Files)
	}
}

// VerifyPin checks if pinned files have changed.
func (cp *ContextPinner) VerifyPin(agentID string) (bool, []string) {
	cp.pinsMu.RLock()
	pin, ok := cp.pins[agentID]
	cp.pinsMu.RUnlock()

	if !ok {
		return false, nil
	}

	var changed []string
	for _, file := range pin.Files {
		fullPath := filepath.Join(cp.worktreePath, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			changed = append(changed, file)
			continue
		}
		if checksum(content) != pin.Checksums[file] {
			changed = append(changed, file)
		}
	}

	return len(changed) == 0, changed
}

// RefreshPin updates the pin with current file contents.
func (cp *ContextPinner) RefreshPin(agentID string) error {
	cp.pinsMu.Lock()
	pin, ok := cp.pins[agentID]
	if !ok {
		cp.pinsMu.Unlock()
		return nil
	}

	for _, file := range pin.Files {
		fullPath := filepath.Join(cp.worktreePath, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		pin.Checksums[file] = checksum(content)
		if len(content) < 10000 {
			pin.Contents[file] = string(content)
		}
	}
	cp.pinsMu.Unlock()

	return nil
}

// GetPinnedContent returns the pinned content for a file.
func (cp *ContextPinner) GetPinnedContent(agentID, file string) (string, bool) {
	cp.pinsMu.RLock()
	defer cp.pinsMu.RUnlock()

	pin, ok := cp.pins[agentID]
	if !ok {
		return "", false
	}

	content, ok := pin.Contents[file]
	return content, ok
}

// FileLockError is returned when files cannot be locked.
type FileLockError struct {
	Files []string
}

func (e *FileLockError) Error() string {
	return "could not lock files: " + strings.Join(e.Files, ", ")
}

// extractDependencies extracts import/require statements from code.
func extractDependencies(content, ext string) []string {
	var deps []string

	switch ext {
	case ".go":
		// import "path" or import ("path1" "path2")
		importRe := regexp.MustCompile(`import\s+(?:\(\s*([^)]+)\s*\)|"([^"]+)")`)
		stringRe := regexp.MustCompile(`"([^"]+)"`)
		
		for _, match := range importRe.FindAllStringSubmatch(content, -1) {
			if match[1] != "" {
				// Multi-line import
				for _, m := range stringRe.FindAllStringSubmatch(match[1], -1) {
					deps = append(deps, m[1])
				}
			} else if match[2] != "" {
				deps = append(deps, match[2])
			}
		}

	case ".rb":
		// require 'path' or require_relative 'path'
		requireRe := regexp.MustCompile(`require(?:_relative)?\s+['"]([^'"]+)['"]`)
		for _, match := range requireRe.FindAllStringSubmatch(content, -1) {
			deps = append(deps, match[1])
		}

	case ".ts", ".tsx", ".js", ".jsx":
		// import x from 'path' or require('path')
		importRe := regexp.MustCompile(`(?:import.*from\s+|require\s*\(\s*)['"]([^'"]+)['"]`)
		for _, match := range importRe.FindAllStringSubmatch(content, -1) {
			deps = append(deps, match[1])
		}

	case ".py":
		// from x import y or import x
		importRe := regexp.MustCompile(`(?:from|import)\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
		for _, match := range importRe.FindAllStringSubmatch(content, -1) {
			deps = append(deps, match[1])
		}
	}

	return deps
}

// resolveDependency converts an import path to a relative file path.
func resolveDependency(dep, currentDir, ext, worktreePath string) string {
	// Handle relative imports
	if strings.HasPrefix(dep, ".") {
		// Resolve relative path
		resolved := filepath.Join(currentDir, dep)
		
		// Try with same extension
		if ext != "" {
			candidate := resolved + ext
			if _, err := os.Stat(filepath.Join(worktreePath, candidate)); err == nil {
				return candidate
			}
		}
		
		// Try common extensions
		extensions := []string{".go", ".rb", ".ts", ".tsx", ".js", ".jsx", ".py"}
		for _, e := range extensions {
			candidate := resolved + e
			if _, err := os.Stat(filepath.Join(worktreePath, candidate)); err == nil {
				return candidate
			}
		}
		
		// Try as directory with index
		for _, index := range []string{"index.ts", "index.tsx", "index.js", "index.jsx"} {
			candidate := filepath.Join(resolved, index)
			if _, err := os.Stat(filepath.Join(worktreePath, candidate)); err == nil {
				return candidate
			}
		}
	}

	// For non-relative imports, we'd need more sophisticated resolution
	// (node_modules, GOPATH, gem paths, etc.)
	// For now, skip absolute/package imports
	return ""
}

// appendUnique appends to a slice if not already present.
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// checksum computes a simple checksum of content.
func checksum(content []byte) string {
	// Simple FNV-1a hash
	var hash uint64 = 14695981039346656037
	for _, b := range content {
		hash ^= uint64(b)
		hash *= 1099511628211
	}
	return fmt.Sprintf("%016x", hash)
}

// PinHandoff wraps pinned context for handoff.
type PinHandoff struct {
	Pin *Pin
}

func (h *PinHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString("# Pinned Context\n\n")
	sb.WriteString("## Files\n")
	for _, f := range h.Pin.Files {
		sb.WriteString("- " + f + "\n")
	}
	sb.WriteString("\n## Contents\n")
	for file, content := range h.Pin.Contents {
		sb.WriteString("### " + file + "\n```\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
	}
	return sb.String()
}

func (h *PinHandoff) Concise() string {
	return strings.Join(h.Pin.Files, ", ")
}

func (h *PinHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 {
		return full
	}
	
	// Return just file list and first 500 chars of each
	var sb strings.Builder
	sb.WriteString("# Pinned Context (truncated)\n\n")
	sb.WriteString("## Files\n")
	for _, f := range h.Pin.Files {
		sb.WriteString("- " + f + "\n")
	}
	sb.WriteString("\n## Contents (previews)\n")
	for file, content := range h.Pin.Contents {
		sb.WriteString("### " + file + "\n```\n")
		if len(content) > 500 {
			sb.WriteString(content[:500] + "\n... (truncated)")
		} else {
			sb.WriteString(content)
		}
		sb.WriteString("\n```\n\n")
	}
	return sb.String()
}

func (h *PinHandoff) Type() string {
	return "pinned_context"
}

// ScanDependencies scans a file and extracts its dependencies more thoroughly.
func ScanDependencies(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ext := filepath.Ext(filePath)
	var deps []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lineDeps := extractDependencies(line, ext)
		deps = append(deps, lineDeps...)
	}

	return deps, scanner.Err()
}
