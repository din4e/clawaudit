// Package sandbox provides sandboxed git operations
package sandbox

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitSandbox provides isolated git operations
type GitSandbox struct {
	sb *Sandbox
}

// NewGitSandbox creates a new git sandbox
func NewGitSandbox(sb *Sandbox) *GitSandbox {
	return &GitSandbox{sb: sb}
}

// Clone clones a repository into the sandbox
// Returns the path to the cloned repository within the sandbox
func (gs *GitSandbox) Clone(ctx context.Context, url, branch string) (string, error) {
	// Determine repo name from URL
	repoName := extractRepoName(url)
	destPath := filepath.Join(gs.sb.repoPath, repoName)

	// Build git clone command
	args := []string{"clone", "--depth", "1", "--single-branch"}

	if branch != "" && branch != "main" && branch != "master" {
		args = append(args, "--branch", branch)
	}

	args = append(args, url, destPath)

	// Execute in sandbox
	cmd, err := gs.sb.Exec("git", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create git command: %w", err)
	}

	cmd.Dir = gs.sb.workPath

	// Set timeout for clone operation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = cmd.Dir
	cmd.Env = cmd.Env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %w: %s", err, string(output))
	}

	log.Printf("[GitSandbox] Cloned %s to %s", url, destPath)

	return destPath, nil
}

// CloneLocal copies a local directory into the sandbox
// This is more efficient than git clone for local repositories
func (gs *GitSandbox) CloneLocal(ctx context.Context, localPath string) (string, error) {
	repoName := filepath.Base(localPath)
	destPath := filepath.Join(gs.sb.repoPath, repoName)

	// Check if local path exists
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("local path not accessible: %w", err)
	}

	// Copy directory to sandbox
	if err := copyPath(localPath, destPath); err != nil {
		return "", fmt.Errorf("failed to copy local repo: %w", err)
	}

	log.Printf("[GitSandbox] Copied local repo %s to %s", localPath, destPath)

	return destPath, nil
}

// GetBranches lists all branches in the sandboxed repository
func (gs *GitSandbox) GetBranches(ctx context.Context, repoPath string) ([]string, error) {
	output, err := gs.sb.ExecCombined("git", "-C", repoPath, "branch", "-a")
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}

	lines := strings.Split(output, "\n")
	branches := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove "* " or "  " prefix and clean up
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimSpace(line)
		// Remove "remotes/origin/" prefix
		line = strings.TrimPrefix(line, "remotes/origin/")
		// Skip HEAD reference
		if line == "HEAD" || strings.HasPrefix(line, "HEAD ->") {
			continue
		}
		branches = append(branches, line)
	}

	return branches, nil
}

// GetDefaultBranch returns the default branch of the repository
func (gs *GitSandbox) GetDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	output, err := gs.sb.ExecCombined("git", "-C", repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		// Fallback to common default branches
		branches, _ := gs.GetBranches(ctx, repoPath)
		for _, b := range []string{"main", "master"} {
			for _, branch := range branches {
				if branch == b {
					return b, nil
				}
			}
		}
		return "main", nil // Default fallback
	}

	// Output format: "refs/remotes/origin/main"
	parts := strings.Split(output, "/")
	if len(parts) >= 4 {
		return strings.TrimSpace(parts[len(parts)-1]), nil
	}

	return "main", nil
}

// Checkout switches to a specific branch
func (gs *GitSandbox) Checkout(ctx context.Context, repoPath, branch string) error {
	_, err := gs.sb.ExecCombined("git", "-C", repoPath, "checkout", branch)
	return err
}

// Pull updates the repository
func (gs *GitSandbox) Pull(ctx context.Context, repoPath string) error {
	_, err := gs.sb.ExecCombined("git", "-C", repoPath, "pull", "--depth", "1")
	return err
}

// GetInfo returns information about the repository
func (gs *GitSandbox) GetInfo(ctx context.Context, repoPath string) (*GitInfo, error) {
	// Get current branch
	branchOutput, _ := gs.sb.ExecCombined("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(branchOutput)

	// Get current commit
	commitOutput, _ := gs.sb.ExecCombined("git", "-C", repoPath, "rev-parse", "HEAD")
	commit := strings.TrimSpace(commitOutput)

	// Get remote URL
	remoteOutput, _ := gs.sb.ExecCombined("git", "-C", repoPath, "config", "--get", "remote.origin.url")
	remote := strings.TrimSpace(remoteOutput)

	return &GitInfo{
		Path:    repoPath,
		Branch:  branch,
		Commit:  commit,
		Remote:  remote,
	}, nil
}

// GitInfo holds information about a git repository
type GitInfo struct {
	Path   string
	Branch string
	Commit string
	Remote string
}

// extractRepoName extracts the repository name from a URL
func extractRepoName(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git://")
	url = strings.TrimPrefix(url, "ssh://")

	// Remove git@ prefix for SSH URLs
	url = strings.TrimPrefix(url, "git@")

	// Replace : with / for SSH URLs (git@github.com:owner/repo.git)
	url = strings.Replace(url, ":", "/", 1)

	// Split by /
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return "repo"
	}

	// Get the last part and remove .git suffix
	repoName := parts[len(parts)-1]
	repoName = strings.TrimSuffix(repoName, ".git")

	if repoName == "" {
		return "repo"
	}

	return repoName
}
