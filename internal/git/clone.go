package git

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CloneOptions 克隆选项
type CloneOptions struct {
	URL       string // 仓库 URL (https 或 ssh)
	Branch    string // 分支名
	TargetDir string // 目标目录（空则使用临时目录）
	Depth     int    // 克隆深度
}

// CloneResult 克隆结果
type CloneResult struct {
	LocalPath string `json:"local_path"`
	URL       string `json:"url"`
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	IsTemp    bool   `json:"is_temp"`
}

// CloneRepository 克隆仓库到本地
func CloneRepository(opts CloneOptions) (*CloneResult, error) {
	// 确定目标目录
	isTemp := false
	targetDir := opts.TargetDir
	if targetDir == "" {
		// 从 URL 提取仓库名称作为目录名
		repoName := extractRepoName(opts.URL)
		// 使用项目目录下的 repo/other 文件夹
		cacheDir := "./repo/other"
		os.MkdirAll(cacheDir, 0755)
		targetDir = filepath.Join(cacheDir, fmt.Sprintf("%s-%d", repoName, time.Now().Unix()))
		isTemp = true
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// 构建 git clone 命令
	args := []string{"clone", "--single-branch"}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, opts.URL, targetDir)

	// 执行克隆
	cmd := exec.Command("git")
	cmd.Args = append([]string{"git"}, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// 清理可能创建的目录
		if isTemp {
			os.RemoveAll(targetDir)
		}
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// 获取当前 commit
	commit, _ := getCurrentCommit(targetDir)

	result := &CloneResult{
		LocalPath: targetDir,
		URL:       opts.URL,
		Branch:    opts.Branch,
		Commit:    commit,
		IsTemp:    isTemp,
	}

	return result, nil
}

// CloneRepositoryByURL 根据 URL 克隆仓库（自动检测平台）
func CloneRepositoryByURL(repoURL string, branch string) (*CloneResult, error) {
	return CloneRepository(CloneOptions{
		URL:    repoURL,
		Branch: branch,
	})
}

// ParseRepoURL 解析仓库 URL，获取平台、所有者和名称
type RepoURLInfo struct {
	Platform string // github, gitlab, or other
	Owner    string // 仓库所有者
	Name     string // 仓库名称
	URL      string // 原始 URL
}

// ParseRepoURL 解析仓库 URL
func ParseRepoURL(rawURL string) (*RepoURLInfo, error) {
	// 移除末尾的 .git
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// 解析 URL
	u, err := url.Parse(rawURL)
	if err != nil {
		// 可能是 SSH 格式
		return parseSSHURL(rawURL)
	}

	// HTTPS 格式: https://github.com/owner/repo
	// 或 https://gitlab.com/owner/repo
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository URL format")
	}

	platform := extractPlatform(u.Host)
	if platform == "" {
		platform = "other"
	}

	return &RepoURLInfo{
		Platform: platform,
		Owner:    parts[0],
		Name:     parts[1],
		URL:      rawURL,
	}, nil
}

// parseSSHURL 解析 SSH 格式的 URL
func parseSSHURL(rawURL string) (*RepoURLInfo, error) {
	// SSH 格式: git@github.com:owner/repo.git
	// 或 git@gitlab.com:owner/repo.git

	if !strings.HasPrefix(rawURL, "git@") {
		return nil, fmt.Errorf("not a valid SSH URL")
	}

	// 移除 git@
	rawURL = strings.TrimPrefix(rawURL, "git@")

	// 分割主机和路径
	parts := strings.SplitN(rawURL, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid SSH URL format")
	}

	host := parts[0]
	path := parts[1]
	path = strings.TrimSuffix(path, ".git")

	// 解析路径 owner/repo
	pathParts := strings.Split(path, "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid SSH URL path format")
	}

	platform := extractPlatform(host)
	if platform == "" {
		platform = "other"
	}

	return &RepoURLInfo{
		Platform: platform,
		Owner:    pathParts[0],
		Name:     pathParts[1],
		URL:      rawURL,
	}, nil
}

// extractPlatform 从主机名提取平台
func extractPlatform(host string) string {
	switch {
	case strings.HasSuffix(host, "github.com"):
		return "github"
	case strings.HasSuffix(host, "gitlab.com"):
		return "gitlab"
	case strings.HasSuffix(host, "gitee.com"):
		return "gitee"
	case strings.HasSuffix(host, "gitea.com"):
		return "gitea"
	default:
		return ""
	}
}

// extractRepoName 从 URL 提取仓库名称
func extractRepoName(repoURL string) string {
	// 尝试解析为 URL
	u, err := url.Parse(strings.TrimSuffix(repoURL, ".git"))
	if err == nil {
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
	}

	// SSH 格式
	if strings.Contains(repoURL, ":") {
		parts := strings.Split(repoURL, ":")
		if len(parts) == 2 {
			pathParts := strings.Split(strings.TrimSuffix(parts[1], ".git"), "/")
			if len(pathParts) >= 2 {
				return pathParts[len(pathParts)-1]
			}
		}
	}

	// 如果无法解析，返回时间戳
	return fmt.Sprintf("repo-%d", time.Now().Unix())
}

// getCurrentCommit 获取当前 commit hash
func getCurrentCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Cleanup 清理克隆的目录
func Cleanup(result *CloneResult) error {
	if result != nil && result.IsTemp && result.LocalPath != "" {
		return os.RemoveAll(result.LocalPath)
	}
	return nil
}

// GetDefaultBranch 获取仓库的默认分支
func GetDefaultBranch(repoURL string) (string, error) {
	args := []string{"ls-remote", "--symref", repoURL, "HEAD"}
	cmd := exec.Command("git")
	cmd.Args = append([]string{"git"}, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// 解析输出获取默认分支
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "ref: refs/heads/") {
			parts := strings.Split(line, " ")
			if len(parts) > 0 {
				ref := strings.TrimPrefix(parts[0], "ref: refs/heads/")
				return ref, nil
			}
		}
	}

	// 默认返回 main
	return "main", nil
}

// GetRemoteBranches 获取仓库的远程分支列表
func GetRemoteBranches(repoURL string) ([]string, error) {
	args := []string{"ls-remote", "--heads", repoURL}
	cmd := exec.Command("git")
	cmd.Args = append([]string{"git"}, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var branches []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) > 1 {
			branch := strings.TrimPrefix(parts[1], "refs/heads/")
			branches = append(branches, branch)
		}
	}

	return branches, nil
}
