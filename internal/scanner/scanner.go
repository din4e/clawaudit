package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/auditor/code-audit-claw/internal/models"
	"github.com/google/uuid"
)

// Scanner 核心扫描器
type Scanner struct {
	batchManager    *BatchManager
	contextManager  *ContextManager
	claudePath      string
	progressCallback ProgressCallback
}

// ProgressCallback 进度回调函数
type ProgressCallback func(progress ProgressUpdate)

// ProgressUpdate 进度更新
type ProgressUpdate struct {
	Type    string // "start", "batch_start", "batch_progress", "batch_complete", "complete", "error"
	BatchID int
	Message string
	Data     interface{}
}

// NewScanner 创建扫描器
func NewScanner(batchSize, maxTokens int, claudePath string) *Scanner {
	return &Scanner{
		batchManager:   NewBatchManager(batchSize, maxTokens),
		contextManager: NewContextManager(maxTokens),
		claudePath:     claudePath,
	}
}

// SetProgressCallback 设置进度回调（接口实现，接受 scanID 和通用回调）
func (s *Scanner) SetProgressCallback(scanID string, cb func(interface{})) {
	s.progressCallback = func(p ProgressUpdate) {
		cb(map[string]interface{}{
			"type":     p.Type,
			"batch_id": p.BatchID,
			"message":  p.Message,
			"data":     p.Data,
		})
	}
}

// emitProgress 发送进度更新
func (s *Scanner) emitProgress(progress ProgressUpdate) {
	if s.progressCallback != nil {
		s.progressCallback(progress)
	}
}

// ScanResult 完整扫描结果
type ScanResult struct {
	ID          string
	RepoPath    string
	RepoName    string
	Branch      string
	Status      models.ScanStatus
	Batches     []models.BatchResult
	StartedAt   time.Time
	CompletedAt *time.Time
	Summary     models.ScanSummary
	Error       string
	mu          sync.Mutex
}

// Scan 扫描仓库
func (s *Scanner) Scan(ctx context.Context, req models.ScanRequest) (*ScanResult, error) {
	result := &ScanResult{
		ID:        uuid.New().String(),
		RepoPath:  req.RepoPath,
		RepoName:  filepath.Base(req.RepoPath),
		Branch:    req.Branch,
		Status:    models.StatusScanning,
		StartedAt: time.Now(),
		Batches:   make([]models.BatchResult, 0),
		Summary: models.ScanSummary{
			IssuesBySeverity: make(map[models.Severity]int),
			IssuesByType:     make(map[models.ScanType]int),
		},
	}

	// 获取所有代码文件
	files, err := s.listCodeFiles(req.RepoPath)
	if err != nil {
		result.Status = models.StatusFailed
		result.Error = err.Error()
		return result, err
	}

	result.Summary.TotalFiles = len(files)

	s.emitProgress(ProgressUpdate{
		Type:    "start",
		Message: fmt.Sprintf("开始扫描，共 %d 个文件", len(files)),
		Data: map[string]interface{}{
			"total_files":  len(files),
			"total_batches": 0,
		},
	})

	// 创建批次
	batches := s.batchManager.CreateBatches(files)
	result.Summary.TotalBatches = len(batches)

	s.emitProgress(ProgressUpdate{
		Type:    "start",
		Message: fmt.Sprintf("已创建 %d 个扫描批次", len(batches)),
		Data: map[string]interface{}{
			"total_files":  len(files),
			"total_batches": len(batches),
		},
	})

	// 并发扫描批次
	s.scanBatches(ctx, result, batches, req.ScanTypes, req.SandboxDir)

	// 完成扫描
	now := time.Now()
	result.CompletedAt = &now
	if result.Status != models.StatusFailed {
		result.Status = models.StatusCompleted
	}

	// 发送完成事件
	s.emitProgress(ProgressUpdate{
		Type:    "complete",
		Message: "扫描完成",
		Data: map[string]interface{}{
			"total_files":     result.Summary.TotalFiles,
			"total_batches":   result.Summary.TotalBatches,
			"total_issues":    result.Summary.TotalIssues,
			"completed_batches": result.Summary.CompletedBatches,
		},
	})

	return result, nil
}

// scanBatches 并发扫描所有批次
func (s *Scanner) scanBatches(ctx context.Context, result *ScanResult, batches [][]FileWithTokens, scanTypes []models.ScanType, sandboxDir string) {
	var wg sync.WaitGroup
	batchChan := make(chan int, len(batches))

	// 限制并发数
	maxConcurrent := 3
	semaphore := make(chan struct{}, maxConcurrent)

	for i, batch := range batches {
		wg.Add(1)
		go func(batchIndex int, files []FileWithTokens) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()

				// 报告批次开始
				s.emitProgress(ProgressUpdate{
					Type:    "batch_start",
					BatchID: batchIndex,
					Message: fmt.Sprintf("开始扫描批次 %d/%d (%d 个文件)", batchIndex+1, len(batches), len(files)),
					Data: map[string]interface{}{
						"files": files,
					},
				})

				batchResult := s.scanBatch(ctx, batchIndex, files, scanTypes, sandboxDir)

				// 报告批次完成
				s.emitProgress(ProgressUpdate{
					Type:    "batch_complete",
					BatchID: batchIndex,
					Message: fmt.Sprintf("批次 %d 完成，发现 %d 个问题", batchIndex+1, len(batchResult.Issues)),
					Data: map[string]interface{}{
						"issues": len(batchResult.Issues),
						"status": batchResult.Status,
					},
				})

				result.mu.Lock()
				result.Batches = append(result.Batches, batchResult)
				result.Summary.CompletedBatches++
				result.Summary.TotalIssues += len(batchResult.Issues)

				// 更新统计
				for _, issue := range batchResult.Issues {
					result.Summary.IssuesBySeverity[issue.Severity]++
					result.Summary.IssuesByType[issue.ScanType]++
				}
				result.mu.Unlock()

				batchChan <- batchIndex
			}
		}(i, batch)
	}

	go func() {
		wg.Wait()
		close(batchChan)
	}()
}

// scanBatch 扫描单个批次
func (s *Scanner) scanBatch(ctx context.Context, batchIndex int, files []FileWithTokens, scanTypes []models.ScanType, sandboxDir string) models.BatchResult {
	result := models.BatchResult{
		BatchID:   batchIndex,
		Files:     make([]string, len(files)),
		Status:    models.StatusScanning,
		StartedAt: time.Now(),
		Issues:    make([]models.Issue, 0),
	}

	for i, f := range files {
		result.Files[i] = f.Path
	}

	// 构建上下文
	_, contextText, err := s.contextManager.BuildContext(files)
	if err != nil {
		result.Status = models.StatusFailed
		result.Error = err.Error()
		return result
	}

	// 调用Claude API
	issues, tokensUsed, err := s.callClaude(ctx, contextText, scanTypes, sandboxDir)
	if err != nil {
		// 如果是上下文溢出错误，重试
		if strings.Contains(err.Error(), "context") || strings.Contains(err.Error(), "token") {
			for attempt := 1; attempt <= 3; attempt++ {
				s.contextManager.HandleContextOverflow(attempt)
				_, contextText, err = s.contextManager.BuildContext(files)
				if err != nil {
					continue
				}
				issues, tokensUsed, err = s.callClaude(ctx, contextText, scanTypes, sandboxDir)
				if err == nil {
					break
				}
			}
		}

		if err != nil {
			result.Status = models.StatusFailed
			result.Error = err.Error()
			return result
		}
	}

	result.Issues = issues
	result.TokensUsed = tokensUsed
	result.Status = models.StatusCompleted

	now := time.Now()
	result.CompletedAt = &now

	return result
}

// callClaude 调用Claude API
func (s *Scanner) callClaude(ctx context.Context, codeContent string, scanTypes []models.ScanType, sandboxDir string) ([]models.Issue, int, error) {
	// 构建提示词
	scanTypeStrs := make([]string, len(scanTypes))
	for i, st := range scanTypes {
		scanTypeStrs[i] = string(st)
	}

	prompt := s.contextManager.BuildSystemPrompt(scanTypeStrs)
	userMessage := fmt.Sprintf("请审计以下代码：\n\n%s", codeContent)

	// 构建命令 - 使用 stdin 传递输入，这样更可靠
	cmd := exec.CommandContext(ctx, s.claudePath,
		"--dangerously-skip-permissions",
	)

	// 设置工作目录为 repo/{id}/
	cmd.Dir = sandboxDir

	// 设置输入
	cmd.Stdin = strings.NewReader(prompt + "\n\n" + userMessage)

	// 执行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, fmt.Errorf("claude command failed: %w\nOutput: %s", err, string(output))
	}

	// 解析响应
	var response struct {
		ProjectAnalysis string `json:"project_analysis"`
		Issues         []struct {
			ID              string `json:"id"`
			TitleCN         string `json:"title_cn"`
			TitleEN         string `json:"title_en"`
			Severity        string `json:"severity"`
			Type            string `json:"type"`
			File            string `json:"file"`
			Line            int    `json:"line"`
			CodeSnippet     string `json:"code_snippet"`
			Description     string `json:"description"`
			IntroductionCN  string `json:"introduction_cn"`
			IntroductionEN  string `json:"introduction_en"`
			AffectedVersions string `json:"affected_versions"`
			AnalysisDetail  string `json:"analysis_detail"`
			POC             string `json:"poc"`
			POCVerification string `json:"poc_verification"`
		} `json:"issues"`
		Usage  struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w\nOutput: %s", err, string(output))
	}

	// 转换为标准 Issue 格式
	issues := make([]models.Issue, len(response.Issues))
	for i, iss := range response.Issues {
		issues[i] = models.Issue{
			ID:          iss.ID,
			Title:       fmt.Sprintf("%s / %s", iss.TitleCN, iss.TitleEN),
			Severity:    models.Severity(iss.Severity),
			ScanType:    models.ScanType(iss.Type),
			File:        iss.File,
			Line:        iss.Line,
			Description: fmt.Sprintf("**项目分析**\n\n%s\n\n**漏洞介绍（中文）**\n\n%s\n\n**漏洞介绍（英文）**\n\n%s\n\n**影响版本**\n\n%s\n\n**分析细节**\n\n%s\n\n**POC**\n\n%s\n\n**POC验证**\n\n%s",
				response.ProjectAnalysis,
				iss.IntroductionCN,
				iss.IntroductionEN,
				iss.AffectedVersions,
				iss.AnalysisDetail,
				iss.POC,
				iss.POCVerification),
			CodeSnippet: iss.CodeSnippet,
		}
	}

	return issues, response.Usage.InputTokens + response.Usage.OutputTokens, nil
}

// listCodeFiles 列出代码文件
func (s *Scanner) listCodeFiles(repoPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// 检查是否需要跳过此目录
			for _, excluded := range s.batchManager.excludeDirs {
				if info.Name() == excluded {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// 检查扩展名
		ext := strings.ToLower(filepath.Ext(path))
		if !s.isSupportedExt(ext) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// isSupportedExt 检查是否支持该扩展名
func (s *Scanner) isSupportedExt(ext string) bool {
	supportedExts := map[string]bool{
		".go":     true,
		".js":     true,
		".ts":     true,
		".tsx":    true,
		".jsx":    true,
		".py":     true,
		".java":   true,
		".c":      true,
		".cpp":    true,
		".cc":     true,
		".h":      true,
		".hpp":    true,
		".cs":     true,
		".php":    true,
		".rb":     true,
		".rs":     true,
		".kt":     true,
		".swift":  true,
		".scala":  true,
		".sh":     true,
		".yml":    true,
		".yaml":   true,
		".json":   true,
		".xml":    true,
		".sql":    true,
	}
	return supportedExts[ext]
}

// ToModel 转换为模型
func (sr *ScanResult) ToModel() models.ScanResult {
	batches := make([]models.BatchResult, len(sr.Batches))
	copy(batches, sr.Batches)

	return models.ScanResult{
		ID:            sr.ID,
		RepoPath:      sr.RepoPath,
		RepoName:      sr.RepoName,
		Branch:        sr.Branch,
		Status:        sr.Status,
		StartedAt:     sr.StartedAt,
		CompletedAt:   sr.CompletedAt,
		Batches:       batches,
		Summary:       sr.Summary,
		Error:         sr.Error,
	}
}
