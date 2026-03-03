// Package sandbox provides isolated execution environment for code scanning tasks.
//
// SECURITY: This sandbox provides basic isolation through:
// 1. Temporary workspace with automatic cleanup
// 2. Restricted environment variables
// 3. Process group isolation (where supported)
// 4. Resource limits via OS (where supported)
//
// Note: For production use, consider container-based isolation (Docker, gVisor, etc.)
package sandbox

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sandbox represents an isolated execution environment
type Sandbox struct {
	ID          string
	rootPath    string
	workPath    string
	repoPath    string
	cleanupOnce sync.Once
	ctx         context.Context
	cancel      context.CancelFunc
}

// Config holds sandbox configuration
type Config struct {
	// BaseDir is the base directory for sandbox workspaces
	BaseDir string

	// Timeout is the maximum duration for sandbox operations
	Timeout time.Duration

	// MaxMemoryMB is the maximum memory in MB (Linux only)
	MaxMemoryMB int

	// EnableChroot enables chroot isolation (Linux only, requires root)
	EnableChroot bool
}

// DefaultConfig returns the default sandbox configuration
func DefaultConfig() *Config {
	// Use repo/ for unified repository and sandbox management
	// Each scan gets its own subdirectory: repo/{scanID}/
	return &Config{
		BaseDir:     "repo",
		Timeout:     30 * time.Minute,
		MaxMemoryMB: 2048,
		EnableChroot: false, // Requires root, disabled by default
	}
}

// Manager manages sandbox lifecycle
type Manager struct {
	Config *Config
	mu     sync.RWMutex
	sandboxes map[string]*Sandbox
}

// NewManager creates a new sandbox manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}
	return &Manager{
		Config: config,
		sandboxes: make(map[string]*Sandbox),
	}
}

// Create creates a new sandbox environment
func (m *Manager) Create(scanID string) (*Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate unique sandbox ID
	sandboxID := fmt.Sprintf("%s-%d", scanID, time.Now().UnixNano())

	// Create sandbox directories
	sandboxRoot := filepath.Join(m.Config.BaseDir, sandboxID)
	workPath := filepath.Join(sandboxRoot, "work")
	repoPath := filepath.Join(sandboxRoot, "repo")

	for _, dir := range []string{sandboxRoot, workPath, repoPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create sandbox directory %s: %w", dir, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.Config.Timeout)

	sb := &Sandbox{
		ID:       sandboxID,
		rootPath: sandboxRoot,
		workPath: workPath,
		repoPath: repoPath,
		ctx:      ctx,
		cancel:   cancel,
	}

	m.sandboxes[sandboxID] = sb

	log.Printf("[Sandbox] Created sandbox_id=%s scan_id=%s path=%s", sandboxID, scanID, sandboxRoot)

	return sb, nil
}

// Get retrieves an existing sandbox
func (m *Manager) Get(sandboxID string) (*Sandbox, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[sandboxID]
	return sb, ok
}

// Remove removes and cleans up a sandbox
func (m *Manager) Remove(sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[sandboxID]
	if !ok {
		return nil
	}

	sb.Cleanup()
	delete(m.sandboxes, sandboxID)

	return nil
}

// CleanupAll removes all sandboxes
func (m *Manager) CleanupAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sb := range m.sandboxes {
		sb.Cleanup()
		delete(m.sandboxes, id)
	}
}

// GetRootPath returns the sandbox root directory
func (s *Sandbox) GetRootPath() string {
	return s.rootPath
}

// GetWorkPath returns the sandbox work directory
func (s *Sandbox) GetWorkPath() string {
	return s.workPath
}

// GetRepoPath returns the sandbox repository directory
func (s *Sandbox) GetRepoPath() string {
	return s.repoPath
}

// Context returns the sandbox context
func (s *Sandbox) Context() context.Context {
	return s.ctx
}

// Exec executes a command within the sandbox environment
func (s *Sandbox) Exec(name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(s.ctx, name, args...)

	// Set working directory to sandbox workspace
	cmd.Dir = s.workPath

	// Restrict environment variables
	cmd.Env = s.restrictedEnv()

	// Set platform-specific process attributes
	s.execPlatform(cmd)

	return cmd, nil
}

// ExecCombined executes a command and returns combined output
func (s *Sandbox) ExecCombined(name string, args ...string) (string, error) {
	cmd, err := s.Exec(name, args...)
	if err != nil {
		return "", err
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %s %v: %w: %s", name, args, err, string(output))
	}

	return string(output), nil
}

// CopyIn copies a file or directory into the sandbox
func (s *Sandbox) CopyIn(src string, destRelPath string) error {
	dest := filepath.Join(s.workPath, destRelPath)

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Copy file or directory
	return copyPath(src, dest)
}

// CopyOut copies a file from the sandbox to host
func (s *Sandbox) CopyOut(srcRelPath string, dest string) error {
	src := filepath.Join(s.workPath, srcRelPath)

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	return copyPath(src, dest)
}

// WriteFile writes data to a file in the sandbox
func (s *Sandbox) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	path := filepath.Join(s.workPath, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// ReadFile reads a file from the sandbox
func (s *Sandbox) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.workPath, relPath))
}

// Cleanup removes the sandbox directory and all its contents
func (s *Sandbox) Cleanup() error {
	s.cleanupOnce.Do(func() {
		s.cancel()

		if s.rootPath != "" {
			log.Printf("[Sandbox] Cleaning up sandbox_id=%s path=%s", s.ID, s.rootPath)

			// Remove entire sandbox directory
			if err := os.RemoveAll(s.rootPath); err != nil {
				log.Printf("[Sandbox] Failed to cleanup sandbox_id=%s error=%v", s.ID, err)
			}
		}
	})
	return nil
}

// restrictedEnv returns a restricted set of environment variables
func (s *Sandbox) restrictedEnv() []string {
	// Start with minimal environment
	baseEnv := []string{
		"PATH=" + os.Getenv("PATH"), // Keep PATH for command execution
		"HOME=" + s.workPath,         // Override HOME to sandbox
		"USER=sandbox",
		"TMPDIR=" + filepath.Join(s.workPath, "tmp"),
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}

	// Add safe environment variables
	safeVars := []string{
		"TERM", "TZ", "LANGUAGE",
	}

	for _, key := range safeVars {
		if val := os.Getenv(key); val != "" {
			baseEnv = append(baseEnv, fmt.Sprintf("%s=%s", key, val))
		}
	}

	return baseEnv
}

// copyPath copies a file or directory
func copyPath(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return copyDir(src, dest)
	}
	return copyFile(src, dest)
}

// copyFile copies a single file
func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	// Preserve file permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, srcInfo.Mode())
}

// copyDir copies a directory recursively
func copyDir(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	// Read directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			// Skip certain directories
			if shouldSkipDir(entry.Name()) {
				continue
			}
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// shouldSkipDir returns true if a directory should be skipped during copy
func shouldSkipDir(name string) bool {
	skipDirs := []string{
		".git", ".svn", ".hg",
		"node_modules", "vendor",
		"__pycache__", ".venv", "venv",
		"target", "build", "dist", "out",
		".next", ".nuxt",
	}

	for _, skip := range skipDirs {
		if name == skip {
			return true
		}
	}
	return false
}

// CleanupOldSandboxes removes sandboxes older than the specified duration
func CleanupOldSandboxes(baseDir string, olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Base directory doesn't exist, nothing to clean
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sandboxPath := filepath.Join(baseDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			log.Printf("[Sandbox] Removing old sandbox path=%s age=%v", sandboxPath, time.Since(info.ModTime()))
			os.RemoveAll(sandboxPath)
		}
	}

	return nil
}

// GetSandboxSize returns the total size of all sandboxes in bytes
func GetSandboxSize(baseDir string) (int64, error) {
	var size int64
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// IsolatePath returns a safe, absolute path within the sandbox
// It prevents path traversal attacks by resolving the path
func (s *Sandbox) IsolatePath(relPath string) (string, error) {
	// Clean the path
	cleanPath := filepath.Clean(relPath)

	// Make it absolute relative to sandbox work directory
	absPath := filepath.Join(s.workPath, cleanPath)

	// Ensure the result is within the sandbox
	if !strings.HasPrefix(absPath, s.workPath) {
		return "", fmt.Errorf("path traversal attempt detected: %s", relPath)
	}

	return absPath, nil
}

// ValidatePath validates that a path is safe within the sandbox
func (s *Sandbox) ValidatePath(path string) error {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		// Relative paths are interpreted relative to workPath
		cleanPath = filepath.Join(s.workPath, cleanPath)
	}

	if !strings.HasPrefix(cleanPath, s.workPath) {
		return fmt.Errorf("path outside sandbox: %s", path)
	}

	return nil
}

// CreateTempFile creates a temporary file in the sandbox
func (s *Sandbox) CreateTempFile(pattern string) (*os.File, error) {
	tmpDir := filepath.Join(s.workPath, "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, err
	}
	return os.CreateTemp(tmpDir, pattern)
}

// GetDiskUsage returns disk usage information for the sandbox
func (s *Sandbox) GetDiskUsage() (used int64, err error) {
	err = filepath.Walk(s.rootPath, func(_ string, info os.FileInfo, _ error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	return
}

// Kill kills any running processes in the sandbox
func (s *Sandbox) Kill() {
	s.cancel()
}

// IsActive returns true if the sandbox context is still active
func (s *Sandbox) IsActive() bool {
	return s.ctx.Err() == nil
}
