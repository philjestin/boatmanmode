// Package filesummary provides intelligent file summarization.
// For large files, it extracts key information like function signatures,
// class definitions, and important comments instead of full content.
package filesummary

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Summary represents a summarized view of a file.
type Summary struct {
	Path           string
	Language       string
	TotalLines     int
	Imports        []string
	Classes        []ClassSummary
	Functions      []FunctionSummary
	Constants      []string
	KeyComments    []string
	TodoItems      []string
	Exports        []string
	// Full content (only for small files)
	FullContent    string
	// Whether this is a full file or summary
	IsSummarized   bool
}

// ClassSummary represents a class/struct/interface.
type ClassSummary struct {
	Name       string
	Type       string // class, struct, interface, module
	Line       int
	Methods    []string
	Properties []string
	Inherits   string
}

// FunctionSummary represents a function/method.
type FunctionSummary struct {
	Name       string
	Signature  string
	Line       int
	DocComment string
	IsPublic   bool
}

// Summarizer creates file summaries.
type Summarizer struct {
	// MaxFullFileLines - files smaller than this are returned in full
	MaxFullFileLines int
	// MaxSummaryTokens - maximum tokens for summary
	MaxSummaryTokens int
}

// New creates a new Summarizer with defaults.
func New() *Summarizer {
	return &Summarizer{
		MaxFullFileLines: 200,
		MaxSummaryTokens: 2000,
	}
}

// SummarizeFile creates a summary of a file.
func (s *Summarizer) SummarizeFile(filePath string) (*Summary, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	ext := filepath.Ext(filePath)
	lang := detectLanguage(ext)

	summary := &Summary{
		Path:       filePath,
		Language:   lang,
		TotalLines: len(lines),
	}

	// Small files - return full content
	if len(lines) <= s.MaxFullFileLines {
		summary.FullContent = string(content)
		summary.IsSummarized = false
		return summary, nil
	}

	// Large files - extract key information
	summary.IsSummarized = true
	s.extractImports(lines, summary, lang)
	s.extractClassesAndFunctions(lines, summary, lang)
	s.extractConstants(lines, summary, lang)
	s.extractKeyComments(lines, summary)
	s.extractExports(lines, summary, lang)

	return summary, nil
}

// SummarizeMultiple summarizes multiple files efficiently.
func (s *Summarizer) SummarizeMultiple(paths []string) ([]*Summary, error) {
	summaries := make([]*Summary, 0, len(paths))

	for _, path := range paths {
		summary, err := s.SummarizeFile(path)
		if err != nil {
			// Skip files that can't be read
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// ToString converts a summary to a string representation.
func (summary *Summary) ToString() string {
	if !summary.IsSummarized {
		return summary.FullContent
	}

	var sb strings.Builder

	sb.WriteString("# File: " + summary.Path + "\n")
	sb.WriteString("# Language: " + summary.Language + "\n")
	sb.WriteString("# Lines: " + itoa(summary.TotalLines) + "\n\n")

	// Imports
	if len(summary.Imports) > 0 {
		sb.WriteString("## Imports\n")
		for _, imp := range summary.Imports {
			sb.WriteString("- " + imp + "\n")
		}
		sb.WriteString("\n")
	}

	// Classes/Structs
	if len(summary.Classes) > 0 {
		sb.WriteString("## Classes/Types\n")
		for _, class := range summary.Classes {
			sb.WriteString("### " + class.Type + " " + class.Name)
			if class.Inherits != "" {
				sb.WriteString(" < " + class.Inherits)
			}
			sb.WriteString("\n")
			if len(class.Properties) > 0 {
				sb.WriteString("Properties: " + strings.Join(class.Properties, ", ") + "\n")
			}
			if len(class.Methods) > 0 {
				sb.WriteString("Methods: " + strings.Join(class.Methods, ", ") + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Functions
	if len(summary.Functions) > 0 {
		sb.WriteString("## Functions\n")
		for _, fn := range summary.Functions {
			if fn.DocComment != "" {
				sb.WriteString("// " + fn.DocComment + "\n")
			}
			sb.WriteString(fn.Signature + "\n")
		}
		sb.WriteString("\n")
	}

	// Constants
	if len(summary.Constants) > 0 {
		sb.WriteString("## Constants\n")
		for _, c := range summary.Constants {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}

	// Key Comments (TODOs, FIXMEs, etc.)
	if len(summary.TodoItems) > 0 {
		sb.WriteString("## TODOs\n")
		for _, todo := range summary.TodoItems {
			sb.WriteString("- " + todo + "\n")
		}
		sb.WriteString("\n")
	}

	// Exports
	if len(summary.Exports) > 0 {
		sb.WriteString("## Exports\n")
		for _, exp := range summary.Exports {
			sb.WriteString("- " + exp + "\n")
		}
	}

	return sb.String()
}

// ToTokenBudget returns a summary within token budget.
func (summary *Summary) ToTokenBudget(maxTokens int) string {
	full := summary.ToString()
	tokens := len(full) / 4 // rough estimate

	if tokens <= maxTokens {
		return full
	}

	// Progressively compress
	var sb strings.Builder

	sb.WriteString("# File: " + summary.Path + " (" + itoa(summary.TotalLines) + " lines)\n\n")

	// Always include function signatures (most important)
	if len(summary.Functions) > 0 {
		sb.WriteString("## Functions\n")
		remaining := maxTokens - len(sb.String())/4
		funcBudget := remaining / 2

		for _, fn := range summary.Functions {
			sig := fn.Signature + "\n"
			if len(sb.String())/4+len(sig)/4 > funcBudget {
				sb.WriteString("... and " + itoa(len(summary.Functions)) + " more functions\n")
				break
			}
			sb.WriteString(sig)
		}
		sb.WriteString("\n")
	}

	// Include classes if space
	if len(summary.Classes) > 0 && len(sb.String())/4 < maxTokens*3/4 {
		sb.WriteString("## Types: ")
		names := make([]string, len(summary.Classes))
		for i, c := range summary.Classes {
			names[i] = c.Name
		}
		sb.WriteString(strings.Join(names, ", ") + "\n")
	}

	return sb.String()
}

// extractImports finds import statements.
func (s *Summarizer) extractImports(lines []string, summary *Summary, lang string) {
	patterns := map[string]*regexp.Regexp{
		"go":         regexp.MustCompile(`^\s*import\s+(?:\(|"([^"]+)")`),
		"python":     regexp.MustCompile(`^\s*(?:from|import)\s+([a-zA-Z_][a-zA-Z0-9_.]*)`),
		"javascript": regexp.MustCompile(`^\s*import\s+.*from\s+['"]([^'"]+)['"]`),
		"typescript": regexp.MustCompile(`^\s*import\s+.*from\s+['"]([^'"]+)['"]`),
		"ruby":       regexp.MustCompile(`^\s*require(?:_relative)?\s+['"]([^'"]+)['"]`),
	}

	pattern, ok := patterns[lang]
	if !ok {
		return
	}

	inMultiImport := false
	for _, line := range lines {
		if strings.Contains(line, "import (") {
			inMultiImport = true
			continue
		}
		if inMultiImport {
			if strings.TrimSpace(line) == ")" {
				inMultiImport = false
				continue
			}
			// Extract import from multi-line import
			if match := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(line); len(match) > 1 {
				summary.Imports = append(summary.Imports, match[1])
			}
			continue
		}

		if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
			summary.Imports = append(summary.Imports, matches[1])
		}
	}
}

// extractClassesAndFunctions finds class and function definitions.
func (s *Summarizer) extractClassesAndFunctions(lines []string, summary *Summary, lang string) {
	var currentClass *ClassSummary
	lastDocComment := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Capture doc comments
		if isDocComment(trimmed, lang) {
			lastDocComment = extractDocComment(trimmed, lang)
			continue
		}

		// Check for class/struct/interface
		if class := s.parseClassDefinition(trimmed, lang); class != nil {
			class.Line = i + 1
			if currentClass != nil {
				summary.Classes = append(summary.Classes, *currentClass)
			}
			currentClass = class
			lastDocComment = ""
			continue
		}

		// Check for function/method
		if fn := s.parseFunctionDefinition(trimmed, lang); fn != nil {
			fn.Line = i + 1
			fn.DocComment = lastDocComment

			if currentClass != nil {
				currentClass.Methods = append(currentClass.Methods, fn.Name)
			} else {
				summary.Functions = append(summary.Functions, *fn)
			}
			lastDocComment = ""
		}

		// Track properties within classes
		if currentClass != nil {
			if prop := s.parseProperty(trimmed, lang); prop != "" {
				currentClass.Properties = append(currentClass.Properties, prop)
			}
		}

		// Clear doc comment if line is not a function
		if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "/*") {
			lastDocComment = ""
		}
	}

	if currentClass != nil {
		summary.Classes = append(summary.Classes, *currentClass)
	}
}

// parseClassDefinition extracts class/struct/interface info.
func (s *Summarizer) parseClassDefinition(line string, lang string) *ClassSummary {
	patterns := map[string]struct {
		pattern *regexp.Regexp
		typeName string
	}{
		"go_struct":    {regexp.MustCompile(`^type\s+(\w+)\s+struct`), "struct"},
		"go_interface": {regexp.MustCompile(`^type\s+(\w+)\s+interface`), "interface"},
		"python":       {regexp.MustCompile(`^class\s+(\w+)(?:\(([^)]*)\))?:`), "class"},
		"ruby":         {regexp.MustCompile(`^class\s+(\w+)(?:\s*<\s*(\w+))?`), "class"},
		"javascript":   {regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?`), "class"},
		"typescript":   {regexp.MustCompile(`^(?:export\s+)?(?:class|interface)\s+(\w+)`), "class"},
	}

	for key, p := range patterns {
		if !strings.HasPrefix(key, lang) && key != lang {
			continue
		}
		if matches := p.pattern.FindStringSubmatch(line); len(matches) > 1 {
			class := &ClassSummary{
				Name: matches[1],
				Type: p.typeName,
			}
			if len(matches) > 2 && matches[2] != "" {
				class.Inherits = matches[2]
			}
			return class
		}
	}

	return nil
}

// parseFunctionDefinition extracts function info.
func (s *Summarizer) parseFunctionDefinition(line string, lang string) *FunctionSummary {
	patterns := map[string]*regexp.Regexp{
		"go":         regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)\s*\([^)]*\)`),
		"python":     regexp.MustCompile(`^def\s+(\w+)\s*\([^)]*\)`),
		"ruby":       regexp.MustCompile(`^def\s+(\w+)`),
		"javascript": regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`),
		"typescript": regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`),
	}

	pattern, ok := patterns[lang]
	if !ok {
		return nil
	}

	if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
		fn := &FunctionSummary{
			Name:      matches[1],
			Signature: strings.TrimSpace(line),
			IsPublic:  isPublicFunction(matches[1], lang),
		}
		return fn
	}

	// Also match arrow functions and method definitions for JS/TS
	if lang == "javascript" || lang == "typescript" {
		arrowPattern := regexp.MustCompile(`^(?:export\s+)?(?:const|let)\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)
		if matches := arrowPattern.FindStringSubmatch(line); len(matches) > 1 {
			return &FunctionSummary{
				Name:      matches[1],
				Signature: strings.TrimSpace(line),
				IsPublic:  true,
			}
		}
	}

	return nil
}

// parseProperty extracts property definitions.
func (s *Summarizer) parseProperty(line string, lang string) string {
	patterns := map[string]*regexp.Regexp{
		"go":         regexp.MustCompile(`^\s*(\w+)\s+\w+`),
		"python":     regexp.MustCompile(`^\s*self\.(\w+)\s*=`),
		"ruby":       regexp.MustCompile(`^\s*(?:attr_accessor|attr_reader|attr_writer)\s+:(\w+)`),
		"typescript": regexp.MustCompile(`^\s*(?:public|private|protected)?\s*(\w+)\s*[?:]`),
	}

	pattern, ok := patterns[lang]
	if !ok {
		return ""
	}

	if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// extractConstants finds constant definitions.
func (s *Summarizer) extractConstants(lines []string, summary *Summary, lang string) {
	patterns := map[string]*regexp.Regexp{
		"go":         regexp.MustCompile(`^(?:const|var)\s+(\w+)\s*=`),
		"python":     regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=`),
		"javascript": regexp.MustCompile(`^(?:export\s+)?const\s+([A-Z][A-Z0-9_]*)\s*=`),
		"ruby":       regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=`),
	}

	pattern, ok := patterns[lang]
	if !ok {
		return
	}

	for _, line := range lines {
		if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
			summary.Constants = append(summary.Constants, strings.TrimSpace(line))
			if len(summary.Constants) > 20 {
				break // Limit constants
			}
		}
	}
}

// extractKeyComments finds TODOs, FIXMEs, etc.
func (s *Summarizer) extractKeyComments(lines []string, summary *Summary) {
	keywords := []string{"TODO:", "FIXME:", "HACK:", "XXX:", "BUG:", "NOTE:", "IMPORTANT:", "WARNING:"}
	keywordPattern := regexp.MustCompile(`(?i)(TODO|FIXME|HACK|XXX|BUG|NOTE|IMPORTANT|WARNING):?\s*(.*)`)

	for i, line := range lines {
		for _, kw := range keywords {
			if strings.Contains(strings.ToUpper(line), strings.TrimSuffix(kw, ":")) {
				if matches := keywordPattern.FindStringSubmatch(line); len(matches) > 2 {
					item := matches[1] + ": " + strings.TrimSpace(matches[2]) + " (line " + itoa(i+1) + ")"
					summary.TodoItems = append(summary.TodoItems, item)
				}
				break
			}
		}

		if len(summary.TodoItems) > 10 {
			break // Limit todos
		}
	}
}

// extractExports finds exported symbols.
func (s *Summarizer) extractExports(lines []string, summary *Summary, lang string) {
	switch lang {
	case "go":
		// Go uses capitalization for exports - already captured in functions/types
		return
	case "javascript", "typescript":
		exportPattern := regexp.MustCompile(`^export\s+(?:default\s+)?(?:const|let|var|function|class|interface|type)\s+(\w+)`)
		for _, line := range lines {
			if matches := exportPattern.FindStringSubmatch(line); len(matches) > 1 {
				summary.Exports = append(summary.Exports, matches[1])
			}
		}
	case "python":
		// Check __all__ definition
		allPattern := regexp.MustCompile(`^__all__\s*=\s*\[([^\]]+)\]`)
		for _, line := range lines {
			if matches := allPattern.FindStringSubmatch(line); len(matches) > 1 {
				exports := strings.Split(matches[1], ",")
				for _, exp := range exports {
					exp = strings.Trim(strings.TrimSpace(exp), "'\"")
					if exp != "" {
						summary.Exports = append(summary.Exports, exp)
					}
				}
			}
		}
	}
}

// Helper functions

func detectLanguage(ext string) string {
	languages := map[string]string{
		".go":   "go",
		".py":   "python",
		".rb":   "ruby",
		".js":   "javascript",
		".jsx":  "javascript",
		".ts":   "typescript",
		".tsx":  "typescript",
		".java": "java",
		".rs":   "rust",
		".c":    "c",
		".cpp":  "cpp",
		".h":    "c",
		".hpp":  "cpp",
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return "unknown"
}

func isDocComment(line string, lang string) bool {
	switch lang {
	case "go":
		return strings.HasPrefix(line, "//")
	case "python":
		return strings.HasPrefix(line, "\"\"\"") || strings.HasPrefix(line, "'''")
	case "ruby":
		return strings.HasPrefix(line, "#")
	case "javascript", "typescript":
		return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/**") || strings.HasPrefix(line, "*")
	}
	return false
}

func extractDocComment(line string, lang string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "//")
	line = strings.TrimPrefix(line, "#")
	line = strings.TrimPrefix(line, "/**")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimPrefix(line, "*/")
	return strings.TrimSpace(line)
}

func isPublicFunction(name string, lang string) bool {
	switch lang {
	case "go":
		return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
	case "python":
		return !strings.HasPrefix(name, "_")
	default:
		return true
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	
	negative := n < 0
	if negative {
		n = -n
	}
	
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	
	return string(digits)
}

// SummarizeDirectory summarizes all code files in a directory.
func (s *Summarizer) SummarizeDirectory(dirPath string) ([]*Summary, error) {
	var paths []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Only include source files
		ext := filepath.Ext(path)
		if detectLanguage(ext) != "unknown" {
			paths = append(paths, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.SummarizeMultiple(paths)
}

