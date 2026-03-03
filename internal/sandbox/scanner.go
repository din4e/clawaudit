// Package sandbox provides sandboxed file scanning operations
package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scanner provides sandboxed file scanning capabilities
type Scanner struct {
	sb *Sandbox
}

// NewScanner creates a new sandboxed scanner
func NewScanner(sb *Sandbox) *Scanner {
	return &Scanner{sb: sb}
}

// ScanConfig holds scanning configuration
type ScanConfig struct {
	// MaxFileSize is the maximum file size in bytes to scan
	MaxFileSize int64

	// MaxLines is the maximum number of lines to read per file
	MaxLines int

	// IncludePatterns are glob patterns for files to include
	IncludePatterns []string

	// ExcludeDirs are directory names to exclude
	ExcludeDirs []string

	// FileExtensions are the file extensions to scan
	FileExtensions map[string]bool
}

// DefaultScanConfig returns the default scanning configuration
func DefaultScanConfig() *ScanConfig {
	exts := map[string]bool{
		// Go
		".go": true,
		// Python
		".py": true, ".pyx": true, ".pyi": true,
		// JavaScript/TypeScript
		".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true,
		// Java
		".java": true, ".kt": true, ".kts": true, ".scala": true,
		// C/C++
		".c": true, ".cpp": true, ".cc": true, ".cxx": true, ".h": true, ".hpp": true,
		// C#
		".cs": true,
		// Ruby
		".rb": true,
		// PHP
		".php": true,
		// Swift
		".swift": true,
		// Rust
		".rs": true,
		// Shell
		".sh": true, ".bash": true, ".zsh": true,
		// Config files
		".yaml": true, ".yml": true, ".json": true, ".xml": true, ".toml": true,
		".env": true, ".ini": true, ".conf": true, ".cfg": true,
		// Docker
		"Dockerfile": true, ".dockerignore": true,
		// Other
		".tf": true, ".mod": true, ".sum": true,
	}

	excludeDirs := []string{
		".git", ".svn", ".hg", ".bzr",
		"node_modules", "vendor", "third_party",
		"__pycache__", ".venv", "venv", ".virtualenv",
		"target", "build", "dist", "out", "bin", "obj",
		".next", ".nuxt", ".webpack",
		"coverage", ".idea", ".vscode",
		".terraform", "terraform_modules",
		"tmp", "temp", ".tmp",
	}

	return &ScanConfig{
		MaxFileSize:     1024 * 1024, // 1MB
		MaxLines:        500,
		IncludePatterns: []string{"*"},
		ExcludeDirs:     excludeDirs,
		FileExtensions:  exts,
	}
}

// FileResult represents a scanned file
type FileResult struct {
	Path         string
	RelativePath string
	Extension    string
	Lines        int
	Size         int64
	Content      string // First MaxLines lines
	Language     string
}

// WalkResult holds the results of walking a directory
type WalkResult struct {
	Files       []*FileResult
	TotalFiles  int
	TotalSize   int64
	Skipped     int
	Excluded    []string
	Duration    time.Duration
}

// Walk walks the sandboxed directory and returns file information
func (s *Scanner) Walk(ctx context.Context, rootPath string, config *ScanConfig) (*WalkResult, error) {
	if config == nil {
		config = DefaultScanConfig()
	}

	startTime := time.Now()
	result := &WalkResult{
		Files:    make([]*FileResult, 0),
		Excluded: make([]string, 0),
	}

	// Sanitize the root path
	safePath, err := s.sb.IsolatePath(rootPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	err = filepath.Walk(safePath, func(path string, info os.FileInfo, err error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Continue walking on error
		}

		// Get relative path
		relPath, err := filepath.Rel(safePath, path)
		if err != nil {
			return nil
		}

		// Skip excluded directories
		if info.IsDir() {
			if isExcludedDir(relPath, config.ExcludeDirs) {
				result.Excluded = append(result.Excluded, relPath)
				return filepath.SkipDir
			}
			return nil
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		if !config.FileExtensions[ext] {
			result.Skipped++
			return nil
		}

		// Check file size
		if info.Size() > config.MaxFileSize {
			result.Skipped++
			return nil
		}

		// Read file content
		content, lines, err := s.readFile(path, config.MaxLines)
		if err != nil {
			result.Skipped++
			return nil
		}

		// Create file result
		fileResult := &FileResult{
			Path:         path,
			RelativePath: relPath,
			Extension:    ext,
			Lines:        lines,
			Size:         info.Size(),
			Content:      content,
			Language:     detectLanguage(ext),
		}

		result.Files = append(result.Files, fileResult)
		result.TotalFiles++
		result.TotalSize += info.Size()

		return nil
	})

	result.Duration = time.Since(startTime)

	return result, err
}

// readFile reads a file with a limit on the number of lines
func (s *Scanner) readFile(path string, maxLines int) (string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	// Check if file is within sandbox
	if err := s.sb.ValidatePath(path); err != nil {
		return "", 0, err
	}

	var lines []string
	lineCount := 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() && lineCount < maxLines {
		lines = append(lines, scanner.Text())
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return "", 0, err
	}

	return strings.Join(lines, "\n"), lineCount, nil
}

// ReadFile reads a complete file from the sandbox
func (s *Scanner) ReadFile(relPath string) ([]byte, error) {
	return s.sb.ReadFile(relPath)
}

// WriteFile writes a file to the sandbox
func (s *Scanner) WriteFile(relPath string, data []byte) error {
	return s.sb.WriteFile(relPath, data, 0644)
}

// GetFileStats returns statistics about files in the sandbox
func (s *Scanner) GetFileStats(rootPath string) (map[string]int, error) {
	stats := make(map[string]int)

	safePath, err := s.sb.IsolatePath(rootPath)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(safePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		stats[ext]++

		return nil
	})

	return stats, err
}

// isExcludedDir checks if a directory should be excluded
func isExcludedDir(path string, excluded []string) bool {
	base := filepath.Base(path)
	for _, excl := range excluded {
		if base == excl || strings.HasPrefix(path, excl+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// detectLanguage detects the programming language from file extension
func detectLanguage(ext string) string {
	langMap := map[string]string{
		".go":    "Go",
		".py":    "Python",
		".pyx":   "Python",
		".pyi":   "Python",
		".js":    "JavaScript",
		".jsx":   "JavaScript",
		".ts":    "TypeScript",
		".tsx":   "TypeScript",
		".mjs":   "JavaScript",
		".cjs":   "JavaScript",
		".java":  "Java",
		".kt":    "Kotlin",
		".kts":   "Kotlin",
		".scala": "Scala",
		".c":     "C",
		".cpp":   "C++",
		".cc":    "C++",
		".cxx":   "C++",
		".h":     "C/C++",
		".hpp":   "C++",
		".cs":    "C#",
		".rb":    "Ruby",
		".php":   "PHP",
		".swift": "Swift",
		".rs":    "Rust",
		".sh":    "Shell",
		".bash":  "Bash",
		".zsh":   "Zsh",
		".yaml":  "YAML",
		".yml":   "YAML",
		".json":  "JSON",
		".xml":   "XML",
		".toml":  "TOML",
		".env":   "Environment",
		".ini":   "INI",
		".tf":    "Terraform",
		".mod":   "Go Module",
		"Dockerfile": "Docker",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "Unknown"
}

// EstimateTokens estimates the number of tokens in a string
// Rough estimate: 1 token ≈ 4 characters for English text
func EstimateTokens(text string) int {
	return len(text) / 4
}

// CreateBatch creates a batch of files for scanning
type Batch struct {
	ID       int
	Files    []*FileResult
	TokenEst int
}

// CreateBatches splits files into batches based on token limits
func CreateBatches(files []*FileResult, maxTokensPerBatch int) []*Batch {
	batches := make([]*Batch, 0)
	currentBatch := &Batch{
		Files: make([]*FileResult, 0),
	}
	currentTokens := 0

	for _, file := range files {
		fileTokens := EstimateTokens(file.Content)

		if currentTokens+fileTokens > maxTokensPerBatch && len(currentBatch.Files) > 0 {
			// Start new batch
			currentBatch.TokenEst = currentTokens
			batches = append(batches, currentBatch)
			currentBatch = &Batch{
				ID:    len(batches),
				Files: make([]*FileResult, 0),
			}
			currentTokens = 0
		}

		currentBatch.Files = append(currentBatch.Files, file)
		currentTokens += fileTokens
	}

	// Add last batch if it has files
	if len(currentBatch.Files) > 0 {
		currentBatch.TokenEst = currentTokens
		currentBatch.ID = len(batches)
		batches = append(batches, currentBatch)
	}

	return batches
}

// CleanupResources forces cleanup of sandbox resources
func (s *Scanner) CleanupResources() error {
	return s.sb.Cleanup()
}

// GetSandboxPath returns the sandbox root path
func (s *Scanner) GetSandboxPath() string {
	return s.sb.GetRootPath()
}

// IsActive checks if the sandbox is still active
func (s *Scanner) IsActive() bool {
	return s.sb.IsActive()
}
