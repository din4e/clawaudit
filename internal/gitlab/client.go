package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultGitLabURL = "https://gitlab.com"
	apiProjects     = "/api/v4/projects"
)

// Client GitLab 客户端
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient 创建 GitLab 客户端
func NewClient(token string, gitlabURL string) *Client {
	baseURL := defaultGitLabURL
	if gitlabURL != "" {
		baseURL = strings.TrimSuffix(gitlabURL, "/")
	}

	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Project GitLab 项目
type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
		SSHURLToRepo      string `json:"ssh_url_to_repo"`
	CreatedAt         string `json:"created_at"`
	LastActivityAt    string `json:"last_activity_at"`
	DefaultBranch     string `json:"default_branch"`
	Topics            []string `json:"topics"`
	StarCount         int    `json:"star_count"`
	ForksCount        int    `json:"forks_count"`
	Archived          bool   `json:"archived"`
}

// ListProjectsOptions 列出项目选项
type ListProjectsOptions struct {
	Search     string // 搜索关键词
	Membership bool   // 只返回用户是成员的项目
	Owned      bool   // 只返回用户拥有的项目
	Archived   bool   // 是否包含已归档项目
	PerPage    int    // 每页数量
	Page       int    // 页码
}

// ListProjects 列出项目
func (c *Client) ListProjects(opts ListProjectsOptions) ([]Project, error) {
	u, err := url.Parse(c.BaseURL + apiProjects)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if opts.Search != "" {
		q.Set("search", opts.Search)
	}
	if opts.Membership {
		q.Set("membership", "true")
	}
	if opts.Owned {
		q.Set("owned", "true")
	}
	if opts.Archived {
		q.Set("archived", "true")
	}
	if opts.PerPage > 0 {
		q.Set("per_page", fmt.Sprintf("%d", opts.PerPage))
	} else {
		q.Set("per_page", "50")
	}
	if opts.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return projects, nil
}

// CloneOptions 克隆选项
type CloneOptions struct {
	ProjectID  int    // 项目 ID
	Branch     string // 分支名（空则使用默认分支）
	TargetDir  string // 目标目录（空则使用临时目录）
	Depth      int    // 克隆深度（0 表示完整克隆）
	KeepOrigin bool   // 是否保留 .git 目录
}

// CloneResult 克隆结果
type CloneResult struct {
	LocalPath  string `json:"local_path"`
	ProjectID  int    `json:"project_id"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	IsTemp     bool   `json:"is_temp"`
}

// CloneProject 克隆项目到本地
func (c *Client) CloneProject(projectID int, branch string, targetDir string) (*CloneResult, error) {
	// 获取项目信息
	project, err := c.GetProject(projectID)
	if err != nil {
		return nil, err
	}

	// 确定目标目录
	isTemp := false
	if targetDir == "" {
		// 创建临时目录
		tempDir := os.TempDir()
		targetDir = filepath.Join(tempDir, fmt.Sprintf("code-audit-claw-%d-%d", projectID, time.Now().Unix()))
		isTemp = true
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// 构建 git clone 命令
	cloneURL := project.HTTPURLToRepo

	// 如果使用 token，在 URL 中嵌入认证信息
	if c.Token != "" {
		// 解析 URL 并嵌入 token
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://oauth2:%s@", c.Token), 1)
		}
	}

	args := []string{"clone", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	} else if project.DefaultBranch != "" {
		args = append(args, "--branch", project.DefaultBranch)
	}
	args = append(args, cloneURL, targetDir)

	// 执行克隆
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil // 克静默执行
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// 清理可能创建的目录
		if isTemp {
			os.RemoveAll(targetDir)
		}
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// 获取当前 commit
	commit, _ := c.getCurrentCommit(targetDir)

	result := &CloneResult{
		LocalPath: targetDir,
		ProjectID: projectID,
		Branch:    branch,
		Commit:    commit,
		IsTemp:    isTemp,
	}

	return result, nil
}

// GetProject 获取单个项目信息
func (c *Client) GetProject(projectID int) (*Project, error) {
	u := fmt.Sprintf("%s%s/%d", c.BaseURL, apiProjects, projectID)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return &project, nil
}

// getCurrentCommit 获取当前 commit hash
func (c *Client) getCurrentCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Cleanup 清理克隆的临时目录
func (c *Client) Cleanup(result *CloneResult) error {
	if result != nil && result.IsTemp && result.LocalPath != "" {
		return os.RemoveAll(result.LocalPath)
	}
	return nil
}

// GetBranches 获取项目分支列表
func (c *Client) GetBranches(projectID int) ([]Branch, error) {
	u := fmt.Sprintf("%s%s/%d/repository/branches", c.BaseURL, apiProjects, projectID)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var branches []Branch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return branches, nil
}

// Branch 分支信息
type Branch struct {
	Name      string `json:"name"`
	Merged    bool   `json:"merged"`
	Protected bool   `json:"protected"`
	Default   bool   `json:"default"`
	Commit    struct {
		ID        string `json:"id"`
		ShortID   string `json:"short_id"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
	} `json:"commit"`
}

// ValidateToken 验证 token 是否有效
func (c *Client) ValidateToken() (bool, error) {
	u := c.BaseURL + "/api/v4/user"

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
