package filesummary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSummarizer(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.MaxFullFileLines != 200 {
		t.Errorf("Expected MaxFullFileLines 200, got %d", s.MaxFullFileLines)
	}
}

func TestSummarizeGoFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	goCode := `package main

import (
	"fmt"
	"os"
)

// Config holds configuration.
type Config struct {
	Name    string
	Timeout int
}

// Helper interface defines helper methods.
type Helper interface {
	Help() error
}

// doSomething does something important.
func doSomething(x int) error {
	if x < 0 {
		return fmt.Errorf("invalid x")
	}
	fmt.Println(x)
	return nil
}

func (c *Config) String() string {
	return c.Name
}

func main() {
	cfg := &Config{Name: "test"}
	doSomething(42)
	fmt.Println(cfg)
	os.Exit(0)
}
`

	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	// Summary should exist
	if summary == nil {
		t.Fatal("Summary should not be nil")
	}

	// Should have language detected
	if summary.Language != "go" {
		t.Errorf("Expected language go, got %s", summary.Language)
	}

	// Should have total lines
	if summary.TotalLines == 0 {
		t.Error("Should have total lines")
	}
}

func TestSummarizePythonFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pythonCode := `"""
Module docstring.
"""

import os
from typing import List, Optional

class Config:
    """Configuration class."""
    
    def __init__(self, name: str):
        self.name = name
    
    def process(self) -> None:
        """Process the config."""
        pass

def helper(x: int) -> str:
    """Helper function."""
    return str(x)

if __name__ == "__main__":
    cfg = Config("test")
    cfg.process()
`

	filePath := filepath.Join(tmpDir, "main.py")
	if err := os.WriteFile(filePath, []byte(pythonCode), 0644); err != nil {
		t.Fatal(err)
	}

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	if summary.Language != "python" {
		t.Errorf("Expected language python, got %s", summary.Language)
	}
}

func TestSummarizeRubyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rubyCode := `# frozen_string_literal: true

require 'json'
require_relative 'helper'

module MyModule
  class Config
    attr_reader :name
    
    def initialize(name)
      @name = name
    end
    
    def process
      puts @name
    end
  end
  
  def self.helper_method
    true
  end
end
`

	filePath := filepath.Join(tmpDir, "config.rb")
	os.WriteFile(filePath, []byte(rubyCode), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	if summary.Language != "ruby" {
		t.Errorf("Expected language ruby, got %s", summary.Language)
	}
}

func TestSummarizeJavaScriptFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jsCode := `import React from 'react';
import { useState, useEffect } from 'react';

export const Button = ({ onClick, children }) => {
  return <button onClick={onClick}>{children}</button>;
};

export default function App() {
  const [count, setCount] = useState(0);
  
  useEffect(() => {
    console.log('mounted');
  }, []);
  
  return (
    <div>
      <Button onClick={() => setCount(c => c + 1)}>
        Count: {count}
      </Button>
    </div>
  );
}
`

	filePath := filepath.Join(tmpDir, "App.jsx")
	os.WriteFile(filePath, []byte(jsCode), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	// JSX should be detected as javascript
	if summary.Language != "javascript" && summary.Language != "jsx" {
		t.Errorf("Expected language javascript or jsx, got %s", summary.Language)
	}
}

func TestSummarizeTypeScriptFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tsCode := `import { Request, Response } from 'express';

interface User {
  id: string;
  name: string;
  email: string;
}

type Handler = (req: Request, res: Response) => Promise<void>;

export class UserService {
  private users: Map<string, User>;
  
  constructor() {
    this.users = new Map();
  }
  
  async getUser(id: string): Promise<User | null> {
    return this.users.get(id) || null;
  }
}

export const createHandler: Handler = async (req, res) => {
  res.json({ status: 'ok' });
};
`

	filePath := filepath.Join(tmpDir, "user.service.ts")
	os.WriteFile(filePath, []byte(tsCode), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	if summary.Language != "typescript" {
		t.Errorf("Expected language typescript, got %s", summary.Language)
	}
}

func TestSummaryToString(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.go")
	os.WriteFile(filePath, []byte("package main\nfunc main() {}"), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	formatted := summary.ToString()
	// Just verify it doesn't panic and returns something
	_ = formatted
}

func TestSummaryToTokenBudget(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.go")
	os.WriteFile(filePath, []byte("package main\nfunc main() {}"), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	output := summary.ToTokenBudget(1000)
	// Just verify it doesn't panic
	_ = output
}

func TestSummarizeWithMaxTokens(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a large file
	var builder strings.Builder
	builder.WriteString("package main\n\n")
	for i := 0; i < 500; i++ {
		builder.WriteString("func dummy" + string(rune('A'+i%26)) + "() {}\n")
	}

	filePath := filepath.Join(tmpDir, "large.go")
	os.WriteFile(filePath, []byte(builder.String()), 0644)

	s := New()
	s.MaxFullFileLines = 50 // Force summarization

	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	// Large file should be summarized
	if summary.TotalLines < 400 {
		t.Errorf("Expected large file, got %d lines", summary.TotalLines)
	}
}

func TestSummarizeMultiple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main\nfunc A() {}"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main\nfunc B() {}"), 0644)

	s := New()
	summaries, err := s.SummarizeMultiple([]string{
		filepath.Join(tmpDir, "file1.go"),
		filepath.Join(tmpDir, "file2.go"),
	})
	if err != nil {
		t.Fatalf("SummarizeMultiple failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Errorf("Expected 2 summaries, got %d", len(summaries))
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		ext  string
		lang string
	}{
		{".go", "go"},
		{".py", "python"},
		{".rb", "ruby"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".java", "java"},
		{".rs", "rust"},
	}

	for _, test := range tests {
		lang := detectLanguage(test.ext)
		if lang != test.lang {
			t.Errorf("For %s, expected %s, got %s", test.ext, test.lang, lang)
		}
	}
}

func TestEmptyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "empty.go")
	os.WriteFile(filePath, []byte(""), 0644)

	s := New()
	summary, err := s.SummarizeFile(filePath)
	if err != nil {
		t.Fatalf("SummarizeFile failed: %v", err)
	}

	if summary == nil {
		t.Error("Should return summary even for empty file")
	}
}

func TestNonExistentFile(t *testing.T) {
	s := New()
	_, err := s.SummarizeFile("/nonexistent/file.go")
	if err == nil {
		t.Error("Should error for non-existent file")
	}
}

func TestSummarizeDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesummary-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main"), 0644)

	s := New()
	summaries, err := s.SummarizeDirectory(tmpDir)
	if err != nil {
		t.Fatalf("SummarizeDirectory failed: %v", err)
	}

	if len(summaries) < 2 {
		t.Errorf("Expected at least 2 summaries, got %d", len(summaries))
	}
}

func TestClassSummary(t *testing.T) {
	cs := ClassSummary{
		Name:     "Config",
		Type:     "struct",
		Line:     10,
		Methods:  []string{"String", "Load"},
		Inherits: "",
	}

	if cs.Name != "Config" {
		t.Error("Name should be Config")
	}
	if len(cs.Methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(cs.Methods))
	}
}

func TestFunctionSummary(t *testing.T) {
	fs := FunctionSummary{
		Name:       "DoSomething",
		Signature:  "func DoSomething(x int) error",
		Line:       20,
		DocComment: "DoSomething does something",
		IsPublic:   true,
	}

	if fs.Name != "DoSomething" {
		t.Error("Name should be DoSomething")
	}
	if !fs.IsPublic {
		t.Error("Should be public")
	}
}
