package github

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
	defaultGitHubURL = "https://github.com"
	defaultAPIURL    = "https://api.github.com"
)

// Client GitHub 客户端
type Client struct {
	BaseURL    string
	APIURL     string
	Token      string
	HTTPClient *http.Client
}

// NewClient 创建 GitHub 客户端
func NewClient(token string, githubURL string) *Client {
	apiURL := defaultAPIURL
	baseURL := defaultGitHubURL

	if githubURL != "" {
		// 处理企业版 GitHub
		baseURL = strings.TrimSuffix(githubURL, "/")
		if strings.Contains(baseURL, "github.com") == false {
			// 企业版 API URL 格式: https://github.example.com/api/v3
			apiURL = baseURL + "/api/v3"
		}
	}

	return &Client{
		BaseURL: baseURL,
		APIURL:  apiURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Repository GitHub 仓库
type Repository struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	PushedAt    string `json:"pushed_at"`
	StargazersCount int `json:"stargazers_count"`
	WatchersCount  int `json:"watchers_count"`
	ForksCount     int `json:"forks_count"`
	Language       string `json:"language"`
	Archived       bool   `json:"archived"`
	Owner          struct {
		Login     string `json:"login"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	} `json:"owner"`
}

// ListRepositoriesOptions 列出仓库选项
type ListRepositoriesOptions struct {
	Visibility string // public, private, all
	Affiliation string // owner, collaborator, organization_member
	Sort        string // created, updated, pushed, full_name
	Direction   string // asc, desc
	PerPage     int
	Page        int
}

// ListRepositories 列出用户的仓库
func (c *Client) ListRepositories(opts ListRepositoriesOptions) ([]Repository, error) {
	u, err := url.Parse(c.APIURL + "/user/repos")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if opts.Visibility != "" {
		q.Set("visibility", opts.Visibility)
	}
	if opts.Affiliation != "" {
		q.Set("affiliation", opts.Affiliation)
	} else {
		// 默认获取用户拥有的和协作的仓库
		q.Set("affiliation", "owner,collaborator")
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	} else {
		q.Set("sort", "updated")
	}
	if opts.Direction != "" {
		q.Set("direction", opts.Direction)
	}
	if opts.PerPage > 0 {
		q.Set("per_page", fmt.Sprintf("%d", opts.PerPage))
	} else {
		q.Set("per_page", "100")
	}
	if opts.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	c.setAuthHeader(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var repos []Repository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return repos, nil
}

// SearchRepositories 搜索仓库
func (c *Client) SearchRepositories(query string, opts SearchOptions) (*SearchResponse, error) {
	u, err := url.Parse(c.APIURL + "/search/repositories")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("per_page", fmt.Sprintf("%d", perPageOrDefault(opts.PerPage)))
	if opts.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	if opts.Order != "" {
		q.Set("order", opts.Order)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	c.setAuthHeader(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return &result, nil
}

// SearchOptions 搜索选项
type SearchOptions struct {
	PerPage int
	Page    int
	Sort    string // stars, forks, help-wanted-issues, updated
	Order   string // desc, asc
}

// SearchResponse 搜索响应
type SearchResponse struct {
	TotalCount int          `json:"total_count"`
	Incomplete bool         `json:"incomplete_results"`
	Items      []Repository `json:"items"`
}

// CloneOptions 克隆选项
type CloneOptions struct {
	Owner      string // 仓库所有者
	Name       string // 仓库名称
	Branch     string // 分支名（空则使用默认分支）
	TargetDir  string // 目标目录（空则使用临时目录）
	Depth      int    // 克隆深度
}

// CloneResult 克隆结果
type CloneResult struct {
	LocalPath  string `json:"local_path"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	IsTemp     bool   `json:"is_temp"`
}

// CloneRepository 克隆仓库到本地
func (c *Client) CloneRepository(owner, name, branch string, targetDir string) (*CloneResult, error) {
	// 确定目标目录
	isTemp := false
	if targetDir == "" {
		// 创建临时目录
		tempDir := os.TempDir()
		targetDir = filepath.Join(tempDir, fmt.Sprintf("code-audit-claw-%s-%s-%d", owner, name, time.Now().Unix()))
		isTemp = true
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// 构建 clone URL
	// 格式: https://token@github.com/owner/repo.git
	cloneURL := fmt.Sprintf("https://%s@%s/%s/%s.git", c.Token, strings.TrimPrefix(c.BaseURL, "https://"), owner, name)

	// 构建 git clone 命令
	args := []string{"clone", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, cloneURL, targetDir)

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
	commit, _ := c.getCurrentCommit(targetDir)

	result := &CloneResult{
		LocalPath: targetDir,
		Owner:     owner,
		Name:      name,
		Branch:    branch,
		Commit:    commit,
		IsTemp:    isTemp,
	}

	return result, nil
}

// GetRepository 获取单个仓库信息
func (c *Client) GetRepository(owner, name string) (*Repository, error) {
	u := fmt.Sprintf("%s/repos/%s/%s", c.APIURL, owner, name)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	c.setAuthHeader(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var repo Repository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return &repo, nil
}

// GetBranches 获取仓库分支列表
func (c *Client) GetBranches(owner, name string) ([]Branch, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/branches", c.APIURL, owner, name)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	c.setAuthHeader(req)

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
	Protected bool   `json:"protected"`
	Commit    struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
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

// ValidateToken 验证 token 是否有效
func (c *Client) ValidateToken() (bool, string) {
	// 使用获取用户信息接口验证 token
	u := c.APIURL + "/user"

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return false, ""
	}

	c.setAuthHeader(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, ""
	}

	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&user)

	return true, user.Login
}

// setAuthHeader 设置认证头
func (c *Client) setAuthHeader(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// perPageOrDefault 返回每页数量或默认值
func perPageOrDefault(n int) int {
	if n > 0 {
		return n
	}
	return 100
}
