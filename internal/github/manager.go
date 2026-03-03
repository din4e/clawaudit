package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager GitHub 仓库管理器
type Manager struct {
	client        *Client
	clonedRepos   map[string]*CloneResult // key: owner/name
	clonedReposMu sync.RWMutex
	tempDir       string
}

// NewManager 创建 GitHub 管理器
func NewManager(token string, githubURL string) *Manager {
	// 使用项目目录下的 repo 文件夹作为缓存目录
	cacheDir := "./repo/github"
	os.MkdirAll(cacheDir, 0755)

	return &Manager{
		client:      NewClient(token, githubURL),
		clonedRepos: make(map[string]*CloneResult),
		tempDir:     cacheDir,
	}
}

// ListRepositories 列出仓库
func (m *Manager) ListRepositories(affiliation string) ([]Repository, error) {
	opts := ListRepositoriesOptions{
		Affiliation: affiliation,
		Sort:        "updated",
		PerPage:     100,
	}
	return m.client.ListRepositories(opts)
}

// SearchRepositories 搜索仓库
func (m *Manager) SearchRepositories(query string) (*SearchResponse, error) {
	opts := SearchOptions{
		Sort:    "updated",
		Order:   "desc",
		PerPage: 100,
	}
	return m.client.SearchRepositories(query, opts)
}

// CloneAndScan 克隆项目并返回本地路径
func (m *Manager) CloneAndScan(ctx context.Context, owner, name, branch string) (string, error) {
	// 构建仓库 key
	repoKey := fmt.Sprintf("%s/%s", owner, name)

	// 检查是否已克隆
	m.clonedReposMu.RLock()
	if existing, ok := m.clonedRepos[repoKey]; ok {
		m.clonedReposMu.RUnlock()
		return existing.LocalPath, nil
	}
	m.clonedReposMu.RUnlock()

	// 获取仓库信息
	repo, err := m.client.GetRepository(owner, name)
	if err != nil {
		return "", fmt.Errorf("get repository failed: %w", err)
	}

	// 确定目标目录（使用 tempDir）
	targetDir := filepath.Join(m.tempDir, fmt.Sprintf("%s-%s", owner, name))
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 构建 git clone 命令
	cloneURL := repo.CloneURL

	// 如果使用 token，在 URL 中嵌入认证信息
	if m.client.Token != "" {
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://x-access-token:%s@", m.client.Token), 1)
		}
	}

	// 确定分支
	cloneBranch := branch
	if cloneBranch == "" && repo.DefaultBranch != "" {
		cloneBranch = repo.DefaultBranch
	}

	args := []string{"clone", "--single-branch"}
	if cloneBranch != "" {
		args = append(args, "--branch", cloneBranch)
	}
	args = append(args, cloneURL, targetDir)

	// 执行克隆
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		os.RemoveAll(targetDir)
		return "", fmt.Errorf("git clone failed: %w", err)
	}

	// 获取当前 commit
	commit, _ := m.getCurrentCommit(targetDir)

	result := &CloneResult{
		LocalPath: targetDir,
		Owner:     owner,
		Name:      name,
		Branch:    cloneBranch,
		Commit:    commit,
		IsTemp:    false,
	}

	// 记录克隆信息
	m.clonedReposMu.Lock()
	m.clonedRepos[repoKey] = result
	m.clonedReposMu.Unlock()

	return targetDir, nil
}

// CloneIntoDir 克隆项目到指定目录（用于沙箱环境）
func (m *Manager) CloneIntoDir(ctx context.Context, owner, name, branch, targetDir string) error {
	// 获取仓库信息
	repo, err := m.client.GetRepository(owner, name)
	if err != nil {
		return fmt.Errorf("get repository failed: %w", err)
	}

	// 创建目标目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 构建 git clone 命令
	cloneURL := repo.CloneURL

	// 如果使用 token，在 URL 中嵌入认证信息
	if m.client.Token != "" {
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://x-access-token:%s@", m.client.Token), 1)
		}
	}

	// 确定分支
	cloneBranch := branch
	if cloneBranch == "" && repo.DefaultBranch != "" {
		cloneBranch = repo.DefaultBranch
	}

	args := []string{"clone", "--single-branch"}
	if cloneBranch != "" {
		args = append(args, "--branch", cloneBranch)
	}
	args = append(args, cloneURL, targetDir)

	// 执行克隆
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		os.RemoveAll(targetDir)
		return fmt.Errorf("git clone failed: %w", err)
	}

	return nil
}

// getCurrentCommit 获取当前 commit hash
func (m *Manager) getCurrentCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetCloneResult 获取克隆结果
func (m *Manager) GetCloneResult(owner, name string) (*CloneResult, bool) {
	m.clonedReposMu.RLock()
	defer m.clonedReposMu.RUnlock()
	repoKey := fmt.Sprintf("%s/%s", owner, name)
	result, ok := m.clonedRepos[repoKey]
	return result, ok
}

// CleanupProject 清理指定项目的克隆
func (m *Manager) CleanupProject(owner, name string) error {
	repoKey := fmt.Sprintf("%s/%s", owner, name)

	m.clonedReposMu.Lock()
	defer m.clonedReposMu.Unlock()

	if result, ok := m.clonedRepos[repoKey]; ok {
		err := m.client.Cleanup(result)
		delete(m.clonedRepos, repoKey)
		return err
	}
	return nil
}

// CleanupAll 清理所有临时克隆
func (m *Manager) CleanupAll() error {
	m.clonedReposMu.Lock()
	defer m.clonedReposMu.Unlock()

	var lastErr error
	for key, result := range m.clonedRepos {
		if err := m.client.Cleanup(result); err != nil {
			lastErr = err
		}
		delete(m.clonedRepos, key)
	}

	return lastErr
}

// GetRepository 获取仓库信息
func (m *Manager) GetRepository(owner, name string) (*Repository, error) {
	return m.client.GetRepository(owner, name)
}

// GetBranches 获取仓库分支
func (m *Manager) GetBranches(owner, name string) ([]Branch, error) {
	return m.client.GetBranches(owner, name)
}

// ValidateToken 验证 token
func (m *Manager) ValidateToken() (bool, string) {
	return m.client.ValidateToken()
}

// GetClient 获取底层客户端
func (m *Manager) GetClient() *Client {
	return m.client
}

// RepoScanTask 仓库扫描任务
type RepoScanTask struct {
	Owner      string
	Name       string
	FullName   string
	Branch     string
	LocalPath  string
	StartedAt  time.Time
	Status     string
}

// ActiveScanTasks 活跃的扫描任务
var ActiveScanTasks = make(map[string]*RepoScanTask)
var ActiveScanTasksMu sync.RWMutex

// RegisterScanTask 注册扫描任务
func RegisterScanTask(scanID string, task *RepoScanTask) {
	ActiveScanTasksMu.Lock()
	defer ActiveScanTasksMu.Unlock()
	ActiveScanTasks[scanID] = task
}

// UnregisterScanTask 取消注册扫描任务
func UnregisterScanTask(scanID string) {
	ActiveScanTasksMu.Lock()
	defer ActiveScanTasksMu.Unlock()
	delete(ActiveScanTasks, scanID)
}

// GetScanTask 获取扫描任务
func GetScanTask(scanID string) (*RepoScanTask, bool) {
	ActiveScanTasksMu.RLock()
	defer ActiveScanTasksMu.RUnlock()
	task, ok := ActiveScanTasks[scanID]
	return task, ok
}
