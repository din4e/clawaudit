package models

import "time"

// ScanType 扫描类型
type ScanType string

const (
	ScanTypeSecurity   ScanType = "security"   // 安全漏洞
	ScanTypeQuality    ScanType = "quality"    // 代码质量
	ScanTypeSecrets    ScanType = "secrets"    // 敏感信息
	ScanTypeCompliance ScanType = "compliance" // 合规检查
)

// Severity 严重程度
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// ScanStatus 扫描状态
type ScanStatus string

const (
	StatusPending   ScanStatus = "pending"
	StatusScanning  ScanStatus = "scanning"
	StatusCompleted ScanStatus = "completed"
	StatusFailed    ScanStatus = "failed"
)

// ScanRequest 扫描请求
type ScanRequest struct {
	RepoPath    string      `json:"repo_path"`
	Branch      string      `json:"branch"`
	ScanTypes   []ScanType  `json:"scan_types"`
	BatchSize   int         `json:"batch_size"`   // 每批文件数量
	MaxContext  int         `json:"max_context"`  // 最大上下文token数
	SandboxDir  string      `json:"sandbox_dir,omitempty"` // 沙箱工作目录（可选）
}

// ScanResult 扫描结果
type ScanResult struct {
	ID          string           `json:"id"`
	RepoPath    string           `json:"repo_path"`
	RepoName    string           `json:"repo_name"`
	Branch      string           `json:"branch"`
	Status      ScanStatus       `json:"status"`
	ScanTypes   []ScanType       `json:"scan_types"`
	StartedAt   time.Time        `json:"started_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	Batches     []BatchResult    `json:"batches"`
	Summary     ScanSummary      `json:"summary"`
	Error       string           `json:"error,omitempty"`
}

// BatchResult 批次扫描结果
type BatchResult struct {
	BatchID     int              `json:"batch_id"`
	Files       []string         `json:"files"`
	Status      ScanStatus       `json:"status"`
	StartedAt   time.Time        `json:"started_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	Issues      []Issue          `json:"issues"`
	TokensUsed  int              `json:"tokens_used"`
	Error       string           `json:"error,omitempty"`
}

// Issue 发现的问题
type Issue struct {
	ID          string      `json:"id"`
	File        string      `json:"file"`
	Line        int         `json:"line"`
	Column      int         `json:"column"`
	Severity    Severity    `json:"severity"`
	ScanType    ScanType    `json:"scan_type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	CodeSnippet string      `json:"code_snippet"`
	RuleID      string      `json:"rule_id"`
	CWE         string      `json:"cwe,omitempty"`
	References  []string    `json:"references,omitempty"`
}

// ScanSummary 扫描摘要
type ScanSummary struct {
	TotalFiles      int            `json:"total_files"`
	TotalBatches    int            `json:"total_batches"`
	CompletedBatches int           `json:"completed_batches"`
	TotalIssues     int            `json:"total_issues"`
	IssuesBySeverity map[Severity]int `json:"issues_by_severity"`
	IssuesByType    map[ScanType]int  `json:"issues_by_type"`
}

// RepositoryInfo 仓库信息
type RepositoryInfo struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	Branch      string    `json:"branch"`
	LastScanned time.Time `json:"last_scanned"`
	ScanCount   int       `json:"scan_count"`
}

// ClaudeRequest Claude API 请求
type ClaudeRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	System    string    `json:"system,omitempty"`
}

// ClaudeResponse Claude API 响应
type ClaudeResponse struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Content []Block  `json:"content"`
	Model   string   `json:"model"`
	Usage   Usage    `json:"usage"`
}

// Message 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Block 内容块
type Block struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Usage token 使用情况
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
