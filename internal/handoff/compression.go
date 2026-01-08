// Package handoff provides structured context passing between agents.
// This file adds dynamic compression for token-efficient handoffs.
package handoff

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CompressionLevel indicates how aggressively to compress.
type CompressionLevel int

const (
	CompressionNone     CompressionLevel = 0
	CompressionLight    CompressionLevel = 1
	CompressionMedium   CompressionLevel = 2
	CompressionHeavy    CompressionLevel = 3
	CompressionExtreme  CompressionLevel = 4
)

// DynamicCompressor provides adaptive compression for handoffs.
type DynamicCompressor struct {
	// TargetTokens is the maximum tokens to aim for
	TargetTokens int
	// MinTokens is the minimum we can compress to
	MinTokens int
	// Priorities for different content types (higher = keep more)
	Priorities map[string]int
}

// NewDynamicCompressor creates a compressor with sensible defaults.
func NewDynamicCompressor(targetTokens int) *DynamicCompressor {
	return &DynamicCompressor{
		TargetTokens: targetTokens,
		MinTokens:    500,
		Priorities: map[string]int{
			"issues":       100, // Most important - always keep
			"requirements": 90,
			"approach":     80,
			"guidance":     70,
			"files":        60,
			"patterns":     50,
			"diff":         40,
			"code":         30,
			"context":      20,
		},
	}
}

// ContentBlock represents a piece of content with priority.
type ContentBlock struct {
	Type     string
	Content  string
	Priority int
	Tokens   int
	Required bool // Cannot be removed
}

// Compress compresses content to fit within token budget.
func (dc *DynamicCompressor) Compress(blocks []ContentBlock) string {
	// Calculate total tokens
	totalTokens := 0
	for i := range blocks {
		blocks[i].Tokens = EstimateTokens(blocks[i].Content)
		if blocks[i].Priority == 0 {
			blocks[i].Priority = dc.Priorities[blocks[i].Type]
		}
		totalTokens += blocks[i].Tokens
	}

	// If we're under budget, return full content
	if totalTokens <= dc.TargetTokens {
		return dc.joinBlocks(blocks)
	}

	// Sort by priority (highest first)
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].Required != blocks[j].Required {
			return blocks[i].Required
		}
		return blocks[i].Priority > blocks[j].Priority
	})

	// Determine compression level needed
	ratio := float64(dc.TargetTokens) / float64(totalTokens)
	level := dc.determineLevel(ratio)

	// Apply progressive compression
	result := dc.applyCompression(blocks, level)

	return result
}

// determineLevel calculates the compression level needed.
func (dc *DynamicCompressor) determineLevel(ratio float64) CompressionLevel {
	switch {
	case ratio >= 0.9:
		return CompressionLight
	case ratio >= 0.6:
		return CompressionMedium
	case ratio >= 0.3:
		return CompressionHeavy
	default:
		return CompressionExtreme
	}
}

// applyCompression applies the appropriate compression level.
func (dc *DynamicCompressor) applyCompression(blocks []ContentBlock, level CompressionLevel) string {
	var result strings.Builder
	usedTokens := 0
	remainingBudget := dc.TargetTokens

	for _, block := range blocks {
		if block.Required {
			// Required blocks get compressed but not removed
			compressed := dc.compressBlock(block, level, remainingBudget)
			result.WriteString(compressed)
			result.WriteString("\n\n")
			tokens := EstimateTokens(compressed)
			usedTokens += tokens
			remainingBudget -= tokens
			continue
		}

		// Skip if we're out of budget
		if remainingBudget < dc.MinTokens/len(blocks) {
			continue
		}

		// Calculate budget for this block based on priority
		blockBudget := dc.calculateBlockBudget(block, remainingBudget, level)

		if blockBudget < 50 {
			// Too little budget, skip or add summary only
			if block.Priority > 50 {
				summary := dc.summarizeBlock(block)
				result.WriteString(summary)
				result.WriteString("\n")
				tokens := EstimateTokens(summary)
				usedTokens += tokens
				remainingBudget -= tokens
			}
			continue
		}

		compressed := dc.compressBlock(block, level, blockBudget)
		result.WriteString(compressed)
		result.WriteString("\n\n")
		tokens := EstimateTokens(compressed)
		usedTokens += tokens
		remainingBudget -= tokens
	}

	return strings.TrimSpace(result.String())
}

// calculateBlockBudget determines how many tokens a block can use.
func (dc *DynamicCompressor) calculateBlockBudget(block ContentBlock, remaining int, level CompressionLevel) int {
	// Higher priority blocks get more of the remaining budget
	priorityFactor := float64(block.Priority) / 100.0

	switch level {
	case CompressionLight:
		return int(float64(remaining) * priorityFactor * 0.8)
	case CompressionMedium:
		return int(float64(remaining) * priorityFactor * 0.5)
	case CompressionHeavy:
		return int(float64(remaining) * priorityFactor * 0.3)
	case CompressionExtreme:
		return int(float64(remaining) * priorityFactor * 0.15)
	default:
		return remaining
	}
}

// compressBlock compresses a single block.
func (dc *DynamicCompressor) compressBlock(block ContentBlock, level CompressionLevel, budget int) string {
	content := block.Content
	tokens := EstimateTokens(content)

	if tokens <= budget {
		return content
	}

	switch level {
	case CompressionLight:
		// Remove extra whitespace, normalize
		return dc.lightCompress(content, budget)
	case CompressionMedium:
		// Remove comments, examples, trim long sections
		return dc.mediumCompress(content, budget)
	case CompressionHeavy:
		// Extract only key information
		return dc.heavyCompress(content, budget, block.Type)
	case CompressionExtreme:
		// Single line summary
		return dc.summarizeBlock(block)
	default:
		return TruncateToTokens(content, budget)
	}
}

// lightCompress performs minimal compression.
func (dc *DynamicCompressor) lightCompress(content string, budget int) string {
	// Normalize whitespace
	content = regexp.MustCompile(`\n{3,}`).ReplaceAllString(content, "\n\n")
	content = regexp.MustCompile(`[ \t]+`).ReplaceAllString(content, " ")

	if EstimateTokens(content) <= budget {
		return content
	}

	return TruncateToTokens(content, budget)
}

// mediumCompress removes less important content.
func (dc *DynamicCompressor) mediumCompress(content string, budget int) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Skip comments (but keep important ones)
		if isComment(trimmed) && !isImportantComment(trimmed) {
			continue
		}

		// Skip example/demo content
		if isExampleContent(trimmed) {
			continue
		}

		result = append(result, line)
	}

	compressed := strings.Join(result, "\n")

	if EstimateTokens(compressed) <= budget {
		return compressed
	}

	return TruncateToTokens(compressed, budget)
}

// heavyCompress extracts only key information.
func (dc *DynamicCompressor) heavyCompress(content string, budget int, contentType string) string {
	switch contentType {
	case "code", "diff":
		return dc.extractCodeSignature(content, budget)
	case "issues", "requirements":
		return dc.extractBulletPoints(content, budget)
	default:
		return dc.extractFirstParagraph(content, budget)
	}
}

// extractCodeSignature extracts function/class signatures from code.
func (dc *DynamicCompressor) extractCodeSignature(content string, budget int) string {
	lines := strings.Split(content, "\n")
	var signatures []string

	// Patterns for signatures
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^(func|def|function|class|type|interface)\s+\w+`),
		regexp.MustCompile(`^(public|private|protected)\s+(static\s+)?\w+\s+\w+`),
		regexp.MustCompile(`^@@.*@@`), // Diff headers
		regexp.MustCompile(`^\+\+\+|^---`), // File markers in diff
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, pattern := range patterns {
			if pattern.MatchString(trimmed) {
				signatures = append(signatures, line)
				break
			}
		}
	}

	result := strings.Join(signatures, "\n")
	if EstimateTokens(result) <= budget {
		return result
	}

	return TruncateToTokens(result, budget)
}

// extractBulletPoints extracts bullet points and numbered items.
func (dc *DynamicCompressor) extractBulletPoints(content string, budget int) string {
	lines := strings.Split(content, "\n")
	var bullets []string

	bulletPattern := regexp.MustCompile(`^[\s]*[-*•]\s+|^[\s]*\d+\.\s+`)

	for _, line := range lines {
		if bulletPattern.MatchString(line) {
			bullets = append(bullets, strings.TrimSpace(line))
		}
	}

	result := strings.Join(bullets, "\n")
	if EstimateTokens(result) <= budget {
		return result
	}

	return TruncateToTokens(result, budget)
}

// extractFirstParagraph extracts the first meaningful paragraph.
func (dc *DynamicCompressor) extractFirstParagraph(content string, budget int) string {
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(content, -1)

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if len(p) > 50 { // Skip short paragraphs
			if EstimateTokens(p) <= budget {
				return p
			}
			return TruncateToTokens(p, budget)
		}
	}

	return TruncateToTokens(content, budget)
}

// summarizeBlock creates a one-line summary.
func (dc *DynamicCompressor) summarizeBlock(block ContentBlock) string {
	// Extract first sentence or line
	content := strings.TrimSpace(block.Content)
	
	// Try to find first sentence
	sentenceEnd := regexp.MustCompile(`[.!?]\s`).FindStringIndex(content)
	if sentenceEnd != nil && sentenceEnd[0] < 200 {
		return fmt.Sprintf("[%s] %s", block.Type, content[:sentenceEnd[0]+1])
	}

	// Fallback to first line
	if idx := strings.Index(content, "\n"); idx > 0 && idx < 200 {
		return fmt.Sprintf("[%s] %s", block.Type, content[:idx])
	}

	// Truncate
	if len(content) > 150 {
		return fmt.Sprintf("[%s] %s...", block.Type, content[:150])
	}

	return fmt.Sprintf("[%s] %s", block.Type, content)
}

// joinBlocks joins blocks into a single string.
func (dc *DynamicCompressor) joinBlocks(blocks []ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		parts = append(parts, block.Content)
	}
	return strings.Join(parts, "\n\n")
}

// Helper functions

func isComment(line string) bool {
	prefixes := []string{"//", "#", "/*", "*", "<!--", "\"\"\"", "'''"}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func isImportantComment(line string) bool {
	important := []string{"TODO:", "FIXME:", "HACK:", "NOTE:", "IMPORTANT:", "WARNING:"}
	upper := strings.ToUpper(line)
	for _, keyword := range important {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

func isExampleContent(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "example:") ||
		strings.Contains(lower, "for example") ||
		strings.Contains(lower, "e.g.") ||
		strings.Contains(lower, "such as")
}

// CompressHandoff compresses a handoff to fit within budget.
func CompressHandoff(h Handoff, maxTokens int) string {
	full := h.Full()
	if EstimateTokens(full) <= maxTokens {
		return full
	}

	// Try ForTokenBudget first
	budgeted := h.ForTokenBudget(maxTokens)
	if EstimateTokens(budgeted) <= maxTokens {
		return budgeted
	}

	// Use dynamic compression as fallback
	compressor := NewDynamicCompressor(maxTokens)

	blocks := []ContentBlock{
		{Type: h.Type(), Content: budgeted, Priority: 50},
	}

	return compressor.Compress(blocks)
}
