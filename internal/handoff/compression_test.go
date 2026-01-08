package handoff

import (
	"strings"
	"testing"
)

func TestNewDynamicCompressor(t *testing.T) {
	compressor := NewDynamicCompressor(5000)

	if compressor.TargetTokens != 5000 {
		t.Errorf("Expected target 5000, got %d", compressor.TargetTokens)
	}
	if compressor.MinTokens != 500 {
		t.Errorf("Expected min 500, got %d", compressor.MinTokens)
	}
}

func TestCompressUnderBudget(t *testing.T) {
	compressor := NewDynamicCompressor(10000)

	blocks := []ContentBlock{
		{Type: "requirements", Content: "Short requirement", Priority: 90},
		{Type: "code", Content: "func main() {}", Priority: 30},
	}

	result := compressor.Compress(blocks)

	// Should contain both blocks unchanged
	if !strings.Contains(result, "Short requirement") {
		t.Error("Should contain requirements")
	}
	if !strings.Contains(result, "func main()") {
		t.Error("Should contain code")
	}
}

func TestCompressOverBudget(t *testing.T) {
	compressor := NewDynamicCompressor(50) // Very small budget

	longContent := strings.Repeat("This is a long piece of content. ", 100)

	blocks := []ContentBlock{
		{Type: "requirements", Content: "Important requirement", Priority: 90, Required: true},
		{Type: "context", Content: longContent, Priority: 20},
	}

	result := compressor.Compress(blocks)

	// Required content should be present (possibly compressed)
	if !strings.Contains(result, "requirement") {
		t.Error("Required content should be present")
	}

	// Low priority content might be dropped or heavily truncated
	if len(result) > 500 { // 50 tokens * ~4 chars * ~2.5 margin
		t.Errorf("Result should be compressed, got length %d", len(result))
	}
}

func TestPriorityOrdering(t *testing.T) {
	compressor := NewDynamicCompressor(200) // Small budget

	blocks := []ContentBlock{
		{Type: "context", Content: "Low priority content that is very long", Priority: 20},
		{Type: "issues", Content: "High priority issues", Priority: 100},
		{Type: "patterns", Content: "Medium priority patterns", Priority: 50},
	}

	result := compressor.Compress(blocks)

	// High priority should definitely be present
	if !strings.Contains(result, "issues") {
		t.Error("High priority content should be present")
	}
}

func TestRequiredContent(t *testing.T) {
	compressor := NewDynamicCompressor(100) // Very small budget

	blocks := []ContentBlock{
		{Type: "issues", Content: "Required issues content", Required: true, Priority: 100},
		{Type: "context", Content: strings.Repeat("Optional context ", 50), Priority: 20},
	}

	result := compressor.Compress(blocks)

	// Required content should always be present
	if !strings.Contains(result, "Required issues") {
		t.Error("Required content should always be present")
	}
}

func TestLightCompress(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	content := "Line 1\n\n\n\nLine 2    with   spaces"
	result := compressor.lightCompress(content, 100)

	// Should normalize whitespace
	if strings.Contains(result, "\n\n\n") {
		t.Error("Should normalize multiple newlines")
	}
	if strings.Contains(result, "   ") {
		t.Error("Should normalize multiple spaces")
	}
}

func TestMediumCompress(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	content := `// This is a comment
func main() {
    // Another comment
    println("hello")
    // Example: this shows how to use it
}`

	result := compressor.mediumCompress(content, 100)

	// Should keep code but may remove comments
	if !strings.Contains(result, "func main()") {
		t.Error("Should keep function definition")
	}
}

func TestExtractCodeSignature(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	code := `package main

import "fmt"

func main() {
    x := 1
    y := 2
    println(x + y)
}

func helper(a int) int {
    return a * 2
}

type Config struct {
    Name string
}
`

	result := compressor.extractCodeSignature(code, 500)

	// Should extract function and type signatures
	if !strings.Contains(result, "func main()") {
		t.Error("Should extract main function")
	}
	if !strings.Contains(result, "func helper") {
		t.Error("Should extract helper function")
	}
	if !strings.Contains(result, "type Config struct") {
		t.Error("Should extract type definition")
	}
}

func TestExtractBulletPoints(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	content := `# Requirements

This is some intro text.

- First requirement
- Second requirement
- Third requirement

Some more text here.

1. Numbered item one
2. Numbered item two

Conclusion paragraph.`

	result := compressor.extractBulletPoints(content, 500)

	// Should extract bullet points
	if !strings.Contains(result, "First requirement") {
		t.Error("Should extract bullet points")
	}
	if !strings.Contains(result, "Numbered item") {
		t.Error("Should extract numbered items")
	}
	// Should not include non-bullet text
	if strings.Contains(result, "intro text") {
		t.Error("Should not include non-bullet text")
	}
}

func TestExtractFirstParagraph(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	content := `This is the first paragraph with important content.

This is the second paragraph.

This is the third paragraph.`

	result := compressor.extractFirstParagraph(content, 200)

	if !strings.Contains(result, "first paragraph") {
		t.Error("Should extract first paragraph")
	}
}

func TestSummarizeBlock(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	block := ContentBlock{
		Type:    "issues",
		Content: "This is a long description of the issue that goes on and on.",
	}

	result := compressor.summarizeBlock(block)

	// Should be prefixed with type
	if !strings.HasPrefix(result, "[issues]") {
		t.Error("Summary should be prefixed with type")
	}

	// Should be short
	if len(result) > 200 {
		t.Errorf("Summary should be short, got length %d", len(result))
	}
}

func TestDetermineLevel(t *testing.T) {
	compressor := NewDynamicCompressor(1000)

	if compressor.determineLevel(0.95) != CompressionLight {
		t.Error("High ratio should use light compression")
	}
	if compressor.determineLevel(0.7) != CompressionMedium {
		t.Error("Medium ratio should use medium compression")
	}
	if compressor.determineLevel(0.4) != CompressionHeavy {
		t.Error("Low ratio should use heavy compression")
	}
	if compressor.determineLevel(0.2) != CompressionExtreme {
		t.Error("Very low ratio should use extreme compression")
	}
}

func TestEstimateTokens(t *testing.T) {
	// ~4 chars per token
	text := "This is a test" // 14 chars
	tokens := EstimateTokens(text)

	if tokens != 3 { // 14/4 = 3
		t.Errorf("Expected ~3 tokens, got %d", tokens)
	}
}

func TestTruncateToTokens(t *testing.T) {
	text := "This is a longer piece of text that should be truncated"

	result := TruncateToTokens(text, 5) // 5 tokens = ~20 chars

	// Should be shorter than original
	if len(result) >= len(text) {
		t.Errorf("Should be truncated, got length %d, original %d", len(result), len(text))
	}
}

func TestCompressHandoff(t *testing.T) {
	// Create a simple handoff
	handoff := &ExecutionHandoff{
		TicketID:    "ENG-123",
		Title:       "Test Feature",
		Description: "A short description",
	}

	// With large budget, should return full
	result := CompressHandoff(handoff, 10000)
	if !strings.Contains(result, "ENG-123") {
		t.Error("Should contain ticket ID")
	}

	// With small budget, should compress
	result = CompressHandoff(handoff, 10)
	if len(result) > 60 { // Very compressed
		t.Errorf("Should be heavily compressed, got length %d", len(result))
	}
}

func TestIsComment(t *testing.T) {
	if !isComment("// This is a comment") {
		t.Error("Should detect // comment")
	}
	if !isComment("# Python comment") {
		t.Error("Should detect # comment")
	}
	if !isComment("/* C style */") {
		t.Error("Should detect /* comment")
	}
	if isComment("Not a comment") {
		t.Error("Should not detect regular line as comment")
	}
}

func TestIsImportantComment(t *testing.T) {
	if !isImportantComment("// TODO: fix this") {
		t.Error("Should detect TODO")
	}
	if !isImportantComment("// FIXME: urgent bug") {
		t.Error("Should detect FIXME")
	}
	if !isImportantComment("// IMPORTANT: do not change") {
		t.Error("Should detect IMPORTANT")
	}
	if isImportantComment("// regular comment") {
		t.Error("Should not detect regular comment as important")
	}
}

func TestIsExampleContent(t *testing.T) {
	if !isExampleContent("Example: here's how to use it") {
		t.Error("Should detect Example:")
	}
	if !isExampleContent("For example, you can do this") {
		t.Error("Should detect for example")
	}
	if !isExampleContent("e.g. this works") {
		t.Error("Should detect e.g.")
	}
	if isExampleContent("Regular content") {
		t.Error("Should not detect regular content")
	}
}

func TestCompressWithMixedContent(t *testing.T) {
	compressor := NewDynamicCompressor(500)

	blocks := []ContentBlock{
		{
			Type:     "issues",
			Content:  "1. Critical bug in auth\n2. Security vulnerability",
			Priority: 100,
			Required: true,
		},
		{
			Type:     "code",
			Content:  "func authenticate() {\n    // long implementation\n}",
			Priority: 30,
		},
		{
			Type:     "context",
			Content:  "This is background context that provides history about the codebase and how it evolved over time...",
			Priority: 20,
		},
	}

	result := compressor.Compress(blocks)

	// Issues should always be present
	if !strings.Contains(result, "Critical bug") {
		t.Error("Required issues should be present")
	}

	// Result should be within budget
	tokens := EstimateTokens(result)
	if tokens > 600 { // Allow some margin
		t.Errorf("Result should be within budget, got %d tokens", tokens)
	}
}
