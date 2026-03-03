package gitlab

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

// Manager GitLab 仓库管理器
type Manager struct {
	client        *Client
	clonedRepos   map[int]*CloneResult
	clonedReposMu sync.RWMutex
	tempDir       string
}

// NewManager 创建 GitLab 管理器
func NewManager(token string, gitlabURL string) *Manager {
	// 使用项目目录下的 repo 文件夹作为缓存目录
	cacheDir := "./repo/gitlab"
	os.MkdirAll(cacheDir, 0755)

	return &Manager{
		client:      NewClient(token, gitlabURL),
		clonedRepos: make(map[int]*CloneResult),
		tempDir:     cacheDir,
	}
}

// ListProjects 列出项目
func (m *Manager) ListProjects(search string, membership bool) ([]Project, error) {
	opts := ListProjectsOptions{
		Search:     search,
		Membership: membership,
		PerPage:    100,
	}
	return m.client.ListProjects(opts)
}

// CloneAndScan 克隆项目并返回本地路径
func (m *Manager) CloneAndScan(ctx context.Context, projectID int, branch string) (string, error) {
	// 检查是否已克隆
	m.clonedReposMu.RLock()
	if existing, ok := m.clonedRepos[projectID]; ok {
		m.clonedReposMu.RUnlock()
		return existing.LocalPath, nil
	}
	m.clonedReposMu.RUnlock()

	// 获取项目信息
	project, err := m.client.GetProject(projectID)
	if err != nil {
		return "", fmt.Errorf("get project failed: %w", err)
	}

	// 确定目标目录（使用 tempDir）
	targetDir := filepath.Join(m.tempDir, fmt.Sprintf("project-%d", projectID))
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 构建 git clone 命令
	cloneURL := project.HTTPURLToRepo

	// 如果使用 token，在 URL 中嵌入认证信息
	if m.client.Token != "" {
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://oauth2:%s@", m.client.Token), 1)
		}
	}

	// 确定分支
	cloneBranch := branch
	if cloneBranch == "" && project.DefaultBranch != "" {
		cloneBranch = project.DefaultBranch
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
		ProjectID: projectID,
		Branch:    cloneBranch,
		Commit:    commit,
		IsTemp:    false,
	}

	// 记录克隆信息
	m.clonedReposMu.Lock()
	m.clonedRepos[projectID] = result
	m.clonedReposMu.Unlock()

	return targetDir, nil
}

// CloneIntoDir 克隆项目到指定目录（用于沙箱环境）
func (m *Manager) CloneIntoDir(ctx context.Context, projectID int, branch, targetDir string) error {
	// 获取项目信息
	project, err := m.client.GetProject(projectID)
	if err != nil {
		return fmt.Errorf("get project failed: %w", err)
	}

	// 创建目标目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 构建 git clone 命令
	cloneURL := project.HTTPURLToRepo

	// 如果使用 token，在 URL 中嵌入认证信息
	if m.client.Token != "" {
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://oauth2:%s@", m.client.Token), 1)
		}
	}

	// 确定分支
	cloneBranch := branch
	if cloneBranch == "" && project.DefaultBranch != "" {
		cloneBranch = project.DefaultBranch
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
func (m *Manager) GetCloneResult(projectID int) (*CloneResult, bool) {
	m.clonedReposMu.RLock()
	defer m.clonedReposMu.RUnlock()
	result, ok := m.clonedRepos[projectID]
	return result, ok
}

// CleanupProject 清理指定项目的克隆
func (m *Manager) CleanupProject(projectID int) error {
	m.clonedReposMu.Lock()
	defer m.clonedReposMu.Unlock()

	if result, ok := m.clonedRepos[projectID]; ok {
		err := m.client.Cleanup(result)
		delete(m.clonedRepos, projectID)
		return err
	}
	return nil
}

// CleanupAll 清理所有临时克隆
func (m *Manager) CleanupAll() error {
	m.clonedReposMu.Lock()
	defer m.clonedReposMu.Unlock()

	var lastErr error
	for id, result := range m.clonedRepos {
		if err := m.client.Cleanup(result); err != nil {
			lastErr = err
		}
		delete(m.clonedRepos, id)
	}

	return lastErr
}

// GetProject 获取项目信息
func (m *Manager) GetProject(projectID int) (*Project, error) {
	return m.client.GetProject(projectID)
}

// GetBranches 获取项目分支
func (m *Manager) GetBranches(projectID int) ([]Branch, error) {
	return m.client.GetBranches(projectID)
}

// ValidateToken 验证 token
func (m *Manager) ValidateToken() (bool, error) {
	return m.client.ValidateToken()
}

// GetClient 获取底层客户端
func (m *Manager) GetClient() *Client {
	return m.client
}

// RepoScanTask 仓库扫描任务
type RepoScanTask struct {
	ProjectID   int
	ProjectName string
	Branch      string
	LocalPath   string
	StartedAt   time.Time
	Status      string
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
