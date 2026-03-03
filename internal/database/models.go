package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ScanDB 数据库扫描记录
type ScanDB struct {
	ID              string     `json:"id"`
	RepoPath        string     `json:"repo_path"`
	RepoName        string     `json:"repo_name"`
	Branch          string     `json:"branch"`
	Status          string     `json:"status"`
	ScanTypes       ScanTypesSlice `json:"scan_types"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	TotalFiles      int        `json:"total_files"`
	TotalBatches    int        `json:"total_batches"`
	CompletedBatches int       `json:"completed_batches"`
	TotalIssues     int        `json:"total_issues"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	Error           string     `json:"error,omitempty"` // 前端兼容
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// MarshalJSON 自定义 JSON 序列化，将 Error 和 ErrorMessage 统一处理
func (s ScanDB) MarshalJSON() ([]byte, error) {
	type Alias ScanDB
	errorValue := s.ErrorMessage
	if errorValue == "" {
		errorValue = s.Error
	}

	aux := &struct {
		Error string `json:"error"`
		*Alias
	}{
		Error: errorValue,
		Alias: (*Alias)(&s),
	}
	return json.Marshal(aux)
}

// BatchDB 数据库批次记录
type BatchDB struct {
	ID          int
	ScanID      string
	BatchID     int
	Files       []string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	TokensUsed  int
	ErrorMessage string
	CreatedAt   time.Time
}

// IssueDB 数据库问题记录
type IssueDB struct {
	ID           int
	ScanID       string
	BatchID      int
	IssueID      string
	FilePath     string
	LineNumber   int
	ColumnNumber int
	Severity     string
	ScanType     string
	Title        string
	Description  string
	CodeSnippet  string
	RuleID       string
	CWE          string
	References   []string
	CreatedAt    time.Time
}

// ScanSummaryDB 数据库扫描摘要
type ScanSummaryDB struct {
	ScanID          string
	SeverityCritical int
	SeverityHigh     int
	SeverityMedium   int
	SeverityLow      int
	SeverityInfo     int
	TypeSecurity     int
	TypeQuality      int
	TypeSecrets      int
	TypeCompliance   int
}

// RepositoryDB 数据库仓库记录
type RepositoryDB struct {
	ID          int
	Path        string
	Name        string
	Branch      string
	LastScanned *time.Time
	ScanCount   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ScanTypesSlice 扫描类型切片（用于数据库存储）
type ScanTypesSlice []string

// Value 实现 driver.Valuer 接口
func (s ScanTypesSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	data, err := json.Marshal(s)
	return string(data), err
}

// Scan 实现 sql.Scanner 接口
func (s *ScanTypesSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into ScanTypesSlice", value)
	}

	return json.Unmarshal(data, s)
}

// StringSliceSlice 字符串切片的切片（用于 Files 字段）
type StringSliceSlice []string

// Value 实现 driver.Valuer 接口（存储为 JSON）
func (s StringSliceSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	data, err := json.Marshal(s)
	return string(data), err
}

// Scan 实现 sql.Scanner 接口
func (s *StringSliceSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into StringSliceSlice", value)
	}

	return json.Unmarshal(data, s)
}

// ReferencesSlice 引用链接切片
type ReferencesSlice []string

// Value 实现 driver.Valuer 接口
func (r ReferencesSlice) Value() (driver.Value, error) {
	if r == nil {
		return "[]", nil
	}
	data, err := json.Marshal(r)
	return string(data), err
}

// Scan 实现 sql.Scanner 接口
func (r *ReferencesSlice) Scan(value interface{}) error {
	if value == nil {
		*r = []string{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into ReferencesSlice", value)
	}

	return json.Unmarshal(data, r)
}

// ListScansFilter 扫描列表查询过滤器
type ListScansFilter struct {
	Status  string
	Limit   int
	Offset  int
	OrderBy string // "started_at", "completed_at", "total_issues"
	Order   string // "ASC", "DESC"
}

// DefaultListScansFilter 返回默认过滤器
func DefaultListScansFilter() *ListScansFilter {
	return &ListScansFilter{
		Limit:   50,
		Offset:  0,
		OrderBy: "started_at",
		Order:   "DESC",
	}
}

// BuildOrderBy 构建 ORDER BY 子句
func (f *ListScansFilter) BuildOrderBy() string {
	orderBy := strings.ToLower(f.OrderBy)
	switch orderBy {
	case "started_at", "completed_at", "total_issues", "repo_name", "status":
		// 有效字段
	default:
		orderBy = "started_at"
	}

	order := strings.ToUpper(f.Order)
	if order != "ASC" && order != "DESC" {
		order = "DESC"
	}

	return fmt.Sprintf("%s %s", orderBy, order)
}

// ListIssuesFilter 问题列表查询过滤器
type ListIssuesFilter struct {
	ScanID   string
	BatchID  int
	Severity string
	ScanType string
	Limit    int
	Offset   int
}

// DefaultListIssuesFilter 返回默认过滤器
func DefaultListIssuesFilter() *ListIssuesFilter {
	return &ListIssuesFilter{
		Limit:  100,
		Offset: 0,
	}
}

// StatsResult 统计结果
type StatsResult struct {
	TotalScans     int64
	CompletedScans int64
	FailedScans    int64
	RunningScans   int64
	TotalIssues    int64
	TotalRepos     int64
}
