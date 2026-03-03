package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/auditor/code-audit-claw/internal/database"
	"github.com/auditor/code-audit-claw/internal/git"
	"github.com/auditor/code-audit-claw/internal/github"
	"github.com/auditor/code-audit-claw/internal/gitlab"
	"github.com/auditor/code-audit-claw/internal/models"
	"github.com/auditor/code-audit-claw/internal/sandbox"
	"github.com/auditor/code-audit-claw/internal/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Router API 路由器
type Router struct {
	engine         *gin.Engine
	scans          map[string]*ScanSession
	scansMutex     sync.RWMutex
	scanner        ScannerInterface
	sandboxManager *sandbox.Manager
	wshub          *websocket.Hub
	gitlabManager  *gitlab.Manager
	githubManager  *github.Manager
	repo           *database.Repository
}

// ScanSession 扫描会话
type ScanSession struct {
	ID               string        `json:"id"`
	RepoPath         string        `json:"repo_path"`
	RepoName         string        `json:"repo_name"`
	Branch           string        `json:"branch,omitempty"`
	ScanTypes        []string      `json:"scan_types,omitempty"`
	Status           string        `json:"status"`
	Progress         float64       `json:"progress"`
	Message          string        `json:"message"` // 当前执行状态描述
	Result           *models.ScanResult `json:"result,omitempty"`
	Error            string        `json:"error,omitempty"`
	TotalBatches     int           `json:"total_batches"`
	CompletedBatches int           `json:"completed_batches"`
	StartedAt        time.Time     `json:"started_at,omitempty"`
	CompletedAt      *time.Time    `json:"completed_at,omitempty"`
	TotalFiles       int           `json:"total_files,omitempty"`
	TotalIssues      int           `json:"total_issues,omitempty"`
	Batches          []*database.BatchDB `json:"batches,omitempty"`
	Summary          map[string]int `json:"summary,omitempty"`
	SandboxID        string        `json:"sandbox_id,omitempty"` // 关联的沙箱ID
	mu               sync.RWMutex  `json:"-"`
}

// ScannerInterface 扫描器接口
type ScannerInterface interface {
	Scan(ctx context.Context, req models.ScanRequest) (*models.ScanResult, error)
	SetProgressCallback(scanID string, callback func(progress interface{}))
}

// scanRequest 扫描请求结构
type scanRequest struct {
	RepoPath   string   `json:"repo_path" binding:"required"`
	Branch     string   `json:"branch"`
	ScanTypes  []string `json:"scan_types"`
	BatchSize  int      `json:"batch_size"`
	MaxContext int      `json:"max_context"`
}

// NewRouter 创建路由器
func NewRouter(scanner ScannerInterface, gitlabManager *gitlab.Manager, githubManager *github.Manager, repo *database.Repository) *Router {
	gin.SetMode(gin.ReleaseMode)
	r := &Router{
		engine:         gin.New(),
		scans:          make(map[string]*ScanSession),
		scanner:        scanner,
		sandboxManager: sandbox.NewManager(nil), // Use default config
		wshub:          websocket.NewHub(),
		gitlabManager:  gitlabManager,
		githubManager:  githubManager,
		repo:           repo,
	}
	r.setupRoutes()

	// 启动时清理旧的沙箱环境
	go r.periodicSandboxCleanup()

	return r
}

// SetGitLabManager 设置 GitLab 管理器
func (r *Router) SetGitLabManager(manager *gitlab.Manager) {
	r.gitlabManager = manager
}

// SetGitHubManager 设置 GitHub 管理器
func (r *Router) SetGitHubManager(manager *github.Manager) {
	r.githubManager = manager
}

// setupRoutes 设置路由
func (r *Router) setupRoutes() {
	// CORS 中间件
	r.engine.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")

		// 允许所有来源（生产环境应限制）
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-GitLab-Token, X-GitLab-URL, X-GitHub-Token, X-GitHub-URL, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})
	r.engine.Use(gin.Recovery(), gin.Logger())

	// 静态文件服务 - Next.js build output
	r.engine.Static("/_next", "./web/out/_next")
	r.engine.Static("/static", "./web/out/_next/static")

	// SPA 路由 - 所有非 API 请求返回 index.html
	r.engine.GET("/", r.indexHandler)
	r.engine.NoRoute(func(c *gin.Context) {
		// 如果是 API 请求，返回 404
		if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/ws") {
			c.JSON(404, gin.H{"error": "Not found"})
			return
		}
		// 否则返回 index.html (用于 SPA 路由)
		c.File("./web/out/index.html")
	})

	// API 路由
	api := r.engine.Group("/api")
	{
		// 扫描管理
		api.POST("/scan", r.createScan)
		api.GET("/scan/:id", r.getScan)
		api.GET("/scan/:id/status", r.getScanStatus)
		api.DELETE("/scan/:id", r.deleteScan)

		// 扫描列表
		api.GET("/scans", r.listScans)

		// 仓库管理
		api.GET("/repos", r.listRepos)
		api.POST("/repos/scan", r.scanRepo)

		// GitLab 集成
		gitlab := api.Group("/gitlab")
		{
			gitlab.POST("/validate", r.gitlabValidateToken)
			gitlab.GET("/projects", r.gitlabListProjects)
			gitlab.GET("/projects/:id", r.gitlabGetProject)
			gitlab.GET("/projects/:id/branches", r.gitlabGetBranches)
			gitlab.POST("/projects/:id/scan", r.gitlabScanProject)
			gitlab.DELETE("/projects/:id/cache", r.gitlabCleanupCache)
		}

		// GitHub 集成
		github := api.Group("/github")
		{
			github.POST("/validate", r.githubValidateToken)
			github.GET("/repositories", r.githubListRepositories)
			github.GET("/search", r.githubSearchRepositories)
			github.GET("/repos/:owner/:name", r.githubGetRepository)
			github.GET("/repos/:owner/:name/branches", r.githubGetBranches)
			github.POST("/repos/:owner/:name/scan", r.githubScanRepository)
			github.DELETE("/repos/:owner/:name/cache", r.githubCleanupCache)
		}

		// 通过 URL 克隆扫描（无需 token）
		api.POST("/scan/url", r.scanByUrl)
		api.GET("/git/parse-url", r.parseGitURL)
		api.GET("/git/branches", r.getRemoteBranches)

		// 批次详情
		api.GET("/scan/:id/batches", r.getScanBatches)
		api.GET("/scan/:id/batch/:batchId", r.getBatchResult)

		// 问题列表
		api.GET("/scan/:id/issues", r.getScanIssues)
		api.GET("/scan/:id/issues/severity/:severity", r.getIssuesBySeverity)

		// 统计
		api.GET("/stats", r.getStats)

		// 系统管理
		api.POST("/system/cleanup", r.cleanupSandboxes)
		api.GET("/system/sandbox-stats", r.getSandboxStats)

		// 读取 JSON 输出结果
		api.GET("/scan/:id/output", r.getScanOutput)
		api.GET("/scan/:id/output/file", r.getScanOutputFile)
	}

	// WebSocket 支持（实时推送）
	r.engine.GET("/ws/scan/:id", r.scanWebSocket)
}

// indexHandler 首页
func (r *Router) indexHandler(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.File("./web/out/index.html")
}

// createScan 创建扫描
func (r *Router) createScan(c *gin.Context) {
	var req scanRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建扫描会话
	sessionID := uuid.New().String()
	session := &ScanSession{
		ID:        sessionID,
		RepoPath:  req.RepoPath,
		Branch:    req.Branch,
		ScanTypes: req.ScanTypes,
		Status:    "pending",
		Progress:  0,
		Message:   "等待中...",
		StartedAt: time.Now(),
	}

	r.scansMutex.Lock()
	r.scans[sessionID] = session
	r.scansMutex.Unlock()

	// 保存到数据库
	scanTypes := make([]string, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = st
	}

	scanDB := &database.ScanDB{
		ID:        sessionID,
		RepoPath:  req.RepoPath,
		RepoName:  req.RepoPath[strings.LastIndex(req.RepoPath, "/")+1:],
		Branch:    req.Branch,
		Status:    "pending",
		ScanTypes: scanTypes,
		StartedAt: time.Now(),
	}
	if err := r.repo.CreateScan(context.Background(), scanDB); err != nil {
		// 数据库保存失败不影响扫描进行
		fmt.Printf("Warning: failed to save scan to database: %v\n", err)
	}

	// 启动异步扫描
	go r.runScan(session, req)

	c.JSON(http.StatusCreated, gin.H{
		"scan_id": sessionID,
		"status":  "pending",
	})
}

// runScan 运行扫描
func (r *Router) runScan(session *ScanSession, req scanRequest) {
	ctx := context.Background()
	repoName := filepath.Base(req.RepoPath)

	// 直接在 repo/{scanID}/ 目录中操作，代码不删除
	scanDir := filepath.Join("repo", session.ID)
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "创建扫描目录失败: " + err.Error()
		session.Progress = 100
		session.mu.Unlock()
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	// 复制本地仓库到扫描目录（保留原始代码）
	targetRepoPath := filepath.Join(scanDir, "repo")
	session.mu.Lock()
	session.Status = "copying"
	session.Progress = 0
	session.Message = fmt.Sprintf("正在准备代码目录: %s -> %s", req.RepoPath, targetRepoPath)
	session.RepoName = repoName
	msg := session.Message
	progress := session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 如果是本地路径，复制代码
	if err := copyPath(req.RepoPath, targetRepoPath); err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "复制代码失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		progress = session.Progress
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	session.mu.Lock()
	session.Status = "scanning"
	session.Progress = 5
	session.Message = fmt.Sprintf("正在扫描代码 (目录: %s)...", targetRepoPath)
	session.RepoPath = targetRepoPath
	msg = session.Message
	progress = session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 更新状态为扫描中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "scanning", "")
	}

	// 设置进度回调
	r.scanner.SetProgressCallback(session.ID, func(progress interface{}) {
		r.handleProgressUpdate(session.ID, progress)
	})

	// 构建扫描请求
	scanTypes := make([]models.ScanType, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = models.ScanType(st)
	}

	// 使用目标路径进行扫描
	scanRequest := models.ScanRequest{
		RepoPath:   targetRepoPath,
		Branch:     req.Branch,
		ScanTypes:  scanTypes,
		BatchSize:  req.BatchSize,
		MaxContext: req.MaxContext,
		SandboxDir: scanDir,
	}

	// 调用扫描器
	result, err := r.scanner.Scan(ctx, scanRequest)

	session.mu.Lock()
	defer session.mu.Unlock()

	completedAt := time.Now()

	if err != nil {
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "扫描失败: " + err.Error()
		session.Progress = 100
		// 更新数据库
		r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		r.repo.CompleteScan(ctx, session.ID, completedAt, 0)
		return
	}

	session.Status = "completed"
	session.Progress = 100
	session.Result = result

	// 保存结果到数据库
	r.saveScanResultToDB(ctx, session.ID, result, repoName)

	// 保存结果为 JSON 文件到 repo/{scanID}/output_result.json
	if err := r.saveScanOutputJSON(session.ID, result, repoName); err != nil {
		fmt.Printf("Warning: failed to save output JSON: %v\n", err)
	}

	fmt.Printf("[Scan] Scan completed. Code preserved at: %s\n", targetRepoPath)
	fmt.Printf("[Scan] Output saved to: %s\n", filepath.Join(scanDir, "output_result.json"))
}

// saveScanResultToDB 保存扫描结果到数据库
func (r *Router) saveScanResultToDB(ctx context.Context, scanID string, result *models.ScanResult, repoName string) {
	// 转换批次
	batches := make([]*database.BatchDB, len(result.Batches))
	for i, b := range result.Batches {
		batches[i] = &database.BatchDB{
			ScanID:       scanID, // 使用 sessionID
			BatchID:      b.BatchID,
			Files:        b.Files,
			Status:       string(b.Status),
			StartedAt:    b.StartedAt,
			CompletedAt:  b.CompletedAt,
			TokensUsed:   b.TokensUsed,
			ErrorMessage: b.Error,
		}
	}

	// 转换问题
	issues := make([]*database.IssueDB, 0)
	for _, b := range result.Batches {
		for _, iss := range b.Issues {
			issues = append(issues, &database.IssueDB{
				ScanID:       scanID, // 使用 sessionID
				BatchID:      b.BatchID,
				IssueID:      iss.ID,
				FilePath:     iss.File,
				LineNumber:   iss.Line,
				ColumnNumber: iss.Column,
				Severity:     string(iss.Severity),
				ScanType:     string(iss.ScanType),
				Title:        iss.Title,
				Description:  iss.Description,
				CodeSnippet:  iss.CodeSnippet,
				RuleID:       iss.RuleID,
				CWE:          iss.CWE,
				References:   iss.References,
			})
		}
	}

	// 转换摘要
	summary := &database.ScanSummaryDB{
		ScanID:          scanID, // 使用 sessionID
		SeverityCritical: result.Summary.IssuesBySeverity[models.SeverityCritical],
		SeverityHigh:     result.Summary.IssuesBySeverity[models.SeverityHigh],
		SeverityMedium:   result.Summary.IssuesBySeverity[models.SeverityMedium],
		SeverityLow:      result.Summary.IssuesBySeverity[models.SeverityLow],
		SeverityInfo:     result.Summary.IssuesBySeverity[models.SeverityInfo],
		TypeSecurity:     result.Summary.IssuesByType[models.ScanTypeSecurity],
		TypeQuality:      result.Summary.IssuesByType[models.ScanTypeQuality],
		TypeSecrets:      result.Summary.IssuesByType[models.ScanTypeSecrets],
		TypeCompliance:   result.Summary.IssuesByType[models.ScanTypeCompliance],
	}

	// 保存批次和问题到数据库
	for _, batch := range batches {
		if err := r.repo.CreateBatch(ctx, batch); err != nil {
			fmt.Printf("Warning: failed to save batch to database: %v\n", err)
		}
	}

	if len(issues) > 0 {
		if err := r.repo.CreateIssues(ctx, issues); err != nil {
			fmt.Printf("Warning: failed to save issues to database: %v\n", err)
		}
	}

	// 保存摘要
	if err := r.repo.CreateSummary(ctx, summary); err != nil {
		fmt.Printf("Warning: failed to save summary to database: %v\n", err)
	}

	// 更新扫描记录
	completedAt := time.Now()
	if result.CompletedAt != nil {
		completedAt = *result.CompletedAt
	}
	scanDB := &database.ScanDB{
		ID:               scanID,
		RepoPath:         result.RepoPath,
		RepoName:         repoName,
		Branch:           result.Branch,
		Status:           string(result.Status),
		ScanTypes:        scanTypeToStringSlice(result.ScanTypes),
		StartedAt:        result.StartedAt,
		CompletedAt:      &completedAt,
		TotalFiles:       result.Summary.TotalFiles,
		TotalBatches:     result.Summary.TotalBatches,
		CompletedBatches: result.Summary.CompletedBatches,
		TotalIssues:      result.Summary.TotalIssues,
	}
	if err := r.repo.UpdateScan(ctx, scanDB); err != nil {
		fmt.Printf("Warning: failed to update scan in database: %v\n", err)
	}
}

// scanTypeToStringSlice 转换扫描类型
func scanTypeToStringSlice(scanTypes []models.ScanType) []string {
	result := make([]string, len(scanTypes))
	for i, st := range scanTypes {
		result[i] = string(st)
	}
	return result
}

// getScan 获取扫描详情
func (r *Router) getScan(c *gin.Context) {
	id := c.Param("id")

	// 先从内存中查找
	r.scansMutex.RLock()
	session, exists := r.scans[id]
	r.scansMutex.RUnlock()

	if exists {
		session.mu.RLock()
		defer session.mu.RUnlock()
		c.JSON(http.StatusOK, session)
		return
	}

	// 从数据库中查找
	scanDB, err := r.repo.GetScan(context.Background(), id)
	if err != nil || scanDB == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}

	// 从数据库获取批次
	batchesDB, err := r.repo.ListBatches(context.Background(), id)
	if err != nil {
		batchesDB = []*database.BatchDB{}
	}

	// 计算进度
	progress := 0.0
	if scanDB.Status == "completed" || scanDB.Status == "failed" {
		progress = 100
	} else if scanDB.Status == "scanning" || scanDB.Status == "cloning" {
		if scanDB.TotalBatches > 0 {
			progress = float64(scanDB.CompletedBatches) / float64(scanDB.TotalBatches) * 100
		}
	}

	// 构建响应
	c.JSON(http.StatusOK, gin.H{
		"id":          scanDB.ID,
		"repo_path":   scanDB.RepoPath,
		"repo_name":   scanDB.RepoName,
		"branch":      scanDB.Branch,
		"status":      scanDB.Status,
		"scan_types":  scanDB.ScanTypes,
		"started_at":  scanDB.StartedAt,
		"completed_at": scanDB.CompletedAt,
		"total_files": scanDB.TotalFiles,
		"total_batches": scanDB.TotalBatches,
		"completed_batches": scanDB.CompletedBatches,
		"total_issues": scanDB.TotalIssues,
		"error":       scanDB.ErrorMessage,
		"batches":     batchesDB,
		"progress":    progress,
		"message":     getStatusMessage(scanDB.Status),
	})
}

// getScanStatus 获取扫描状态（轻量级）
func (r *Router) getScanStatus(c *gin.Context) {
	id := c.Param("id")

	// 先从内存中查找
	r.scansMutex.RLock()
	session, exists := r.scans[id]
	r.scansMutex.RUnlock()

	if exists {
		session.mu.RLock()
		defer session.mu.RUnlock()

		c.JSON(http.StatusOK, gin.H{
			"scan_id":  id,
			"status":   session.Status,
			"progress": session.Progress,
			"message":  session.Message,
		})
		return
	}

	// 从数据库中查找
	scanDB, err := r.repo.GetScan(context.Background(), id)
	if err != nil || scanDB == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}

	// 计算进度
	progress := 0.0
	if scanDB.Status == "completed" || scanDB.Status == "failed" {
		progress = 100
	} else if scanDB.Status == "scanning" || scanDB.Status == "cloning" {
		if scanDB.TotalBatches > 0 {
			progress = float64(scanDB.CompletedBatches) / float64(scanDB.TotalBatches) * 100
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"scan_id":  id,
		"status":   scanDB.Status,
		"progress": progress,
		"message":  getStatusMessage(scanDB.Status),
	})
}

// getStatusMessage 获取状态消息
func getStatusMessage(status string) string {
	messages := map[string]string{
		"pending":  "等待中...",
		"copying":  "正在复制到沙箱...",
		"cloning":  "正在克隆仓库...",
		"scanning": "正在扫描代码...",
		"completed": "扫描完成",
		"failed":   "扫描失败",
	}
	if msg, ok := messages[status]; ok {
		return msg
	}
	return status
}

// deleteScan 删除扫描
func (r *Router) deleteScan(c *gin.Context) {
	id := c.Param("id")

	// 从内存中删除
	r.scansMutex.Lock()
	delete(r.scans, id)
	r.scansMutex.Unlock()

	// 从数据库中删除（级联删除批次和问题）
	if err := r.repo.DeleteScan(context.Background(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// listScans 列出所有扫描
func (r *Router) listScans(c *gin.Context) {
	// 获取查询参数
	status := c.Query("status")
	limit := 50
	offset := 0

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit > 100 {
			limit = 100
		}
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	filter := &database.ListScansFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
		OrderBy: "started_at",
		Order:   "DESC",
	}

	scans, total, err := r.repo.ListScans(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取内存中的活跃扫描，用于合并实时进度
	r.scansMutex.RLock()
	defer r.scansMutex.RUnlock()

	// 构建响应，添加 progress 和 message 字段
	type ScanWithProgress struct {
		database.ScanDB
		Progress float64 `json:"progress"`
		Message  string  `json:"message"`
	}

	result := make([]ScanWithProgress, 0, len(scans))
	for _, scan := range scans {
		progress := 0.0
		message := getStatusMessage(scan.Status)

		// 如果是活跃扫描，使用内存中的实时数据
		if session, ok := r.scans[scan.ID]; ok {
			session.mu.RLock()
			progress = session.Progress
			message = session.Message
			session.mu.RUnlock()
		} else if scan.Status == "completed" || scan.Status == "failed" {
			progress = 100
		} else if scan.Status == "scanning" || scan.Status == "cloning" {
			if scan.TotalBatches > 0 {
				progress = float64(scan.CompletedBatches) / float64(scan.TotalBatches) * 100
			}
		}

		result = append(result, ScanWithProgress{
			ScanDB:   *scan, // 解引用指针
			Progress: progress,
			Message:  message,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"scans": result,
		"total": total,
	})
}

// listRepos 列出仓库
func (r *Router) listRepos(c *gin.Context) {
	limit := 50
	offset := 0

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	repos, total, err := r.repo.ListRepositories(context.Background(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"repos": repos,
		"total": total,
	})
}

// scanRepo 扫描仓库
func (r *Router) scanRepo(c *gin.Context) {
	// 类似 createScan
	c.JSON(http.StatusOK, gin.H{"message": "use /api/scan"})
}

// getScanBatches 获取扫描批次列表
func (r *Router) getScanBatches(c *gin.Context) {
	id := c.Param("id")

	batches, err := r.repo.ListBatches(context.Background(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scan_id": id,
		"batches": batches,
	})
}

// getBatchResult 获取单个批次结果
func (r *Router) getBatchResult(c *gin.Context) {
	id := c.Param("id")
	batchIDParam := c.Param("batchId")

	var batchID int
	fmt.Sscanf(batchIDParam, "%d", &batchID)

	batch, err := r.repo.GetBatch(context.Background(), id, batchID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	// 获取该批次的问题
	filter := &database.ListIssuesFilter{
		ScanID:  id,
		BatchID: batchID,
		Limit:   1000,
	}
	issues, _, _ := r.repo.ListIssues(context.Background(), filter)

	c.JSON(http.StatusOK, gin.H{
		"batch":  batch,
		"issues": issues,
	})
}

// getScanIssues 获取扫描问题
func (r *Router) getScanIssues(c *gin.Context) {
	id := c.Param("id")

	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	filter := &database.ListIssuesFilter{
		ScanID: id,
		Limit:  limit,
		Offset: offset,
	}

	issues, total, err := r.repo.ListIssues(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scan_id": id,
		"issues":  issues,
		"total":   total,
	})
}

// getIssuesBySeverity 按严重程度获取问题
func (r *Router) getIssuesBySeverity(c *gin.Context) {
	id := c.Param("id")
	severity := c.Param("severity")

	issues, err := r.repo.GetIssuesBySeverity(context.Background(), id, severity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scan_id": id,
		"severity": severity,
		"issues":   issues,
		"total":    len(issues),
	})
}

// getStats 获取统计信息
func (r *Router) getStats(c *gin.Context) {
	stats, err := r.repo.GetStats(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// cleanupSandboxes 清理旧的沙箱环境
func (r *Router) cleanupSandboxes(c *gin.Context) {
	// 清理超过24小时的沙箱
	err := sandbox.CleanupOldSandboxes(r.sandboxManager.Config.BaseDir, 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cleanup completed"})
}

// getSandboxStats 获取沙箱统计信息
func (r *Router) getSandboxStats(c *gin.Context) {
	size, err := sandbox.GetSandboxSize(r.sandboxManager.Config.BaseDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_size_bytes": size,
		"total_size_mb":    size / (1024 * 1024),
		"base_dir":         r.sandboxManager.Config.BaseDir,
	})
}

// periodicSandboxCleanup 定期清理旧的沙箱环境
func (r *Router) periodicSandboxCleanup() {
	// 启动时立即清理一次超过24小时的沙箱
	fmt.Printf("[Sandbox] Cleaning up old sandboxes older than 24 hours...\n")
	if err := sandbox.CleanupOldSandboxes(r.sandboxManager.Config.BaseDir, 24*time.Hour); err != nil {
		fmt.Printf("[Sandbox] Failed to cleanup old sandboxes: %v\n", err)
	} else {
		fmt.Printf("[Sandbox] Cleanup completed\n")
	}

	// 每小时清理一次
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := sandbox.CleanupOldSandboxes(r.sandboxManager.Config.BaseDir, 24*time.Hour); err != nil {
			fmt.Printf("[Sandbox] Failed to cleanup old sandboxes: %v\n", err)
		}
	}
}

// handleProgressUpdate 处理扫描进度更新
func (r *Router) handleProgressUpdate(scanID string, progress interface{}) {
	r.scansMutex.Lock()
	session, exists := r.scans[scanID]
	if !exists {
		r.scansMutex.Unlock()
		return
	}

	// 解析进度信息
	var pType string
	var pMessage string
	var pData map[string]interface{}

	if p, ok := progress.(map[string]interface{}); ok {
		if msg, ok := p["message"].(string); ok {
			pMessage = msg
		}
		if t, ok := p["type"].(string); ok {
			pType = t
		}
		if d, ok := p["data"].(map[string]interface{}); ok {
			pData = d
		}
	}

	// 打印进度到服务器终端（实时显示）
	r.printProgressToConsole(scanID, session.RepoName, pType, pMessage, pData)

	// 更新会话状态
	session.mu.Lock()
	if pMessage != "" {
		session.Message = pMessage
	}

	// 计算进度并广播到 WebSocket
	switch pType {
	case "start":
		if total, ok := pData["total_batches"].(float64); ok {
			session.TotalBatches = int(total)
		}
		// 广播扫描开始
		if totalFiles, ok := pData["total_files"].(float64); ok {
			r.wshub.BroadcastScanStart(scanID, int(totalFiles), session.TotalBatches)
		}
		fmt.Printf("\n")
	case "batch_start":
		// 广播批次开始
		if batchID, ok := pData["batch_id"].(float64); ok {
			if files, ok := pData["files"].([]interface{}); ok {
				fileNames := make([]string, len(files))
				for i, f := range files {
					fileNames[i] = fmt.Sprint(f)
				}
				r.wshub.BroadcastBatchStart(scanID, int(batchID), fileNames)
			}
		}
	case "batch_complete":
		session.CompletedBatches++
		if session.TotalBatches > 0 {
			session.Progress = float64(session.CompletedBatches) / float64(session.TotalBatches) * 100
		}
		// 广播批次完成
		if batchID, ok := pData["batch_id"].(float64); ok {
			if issues, ok := pData["issues"].(float64); ok {
				r.wshub.BroadcastBatchComplete(scanID, int(batchID), int(issues))
			}
		}
		// 广播进度更新
		r.wshub.BroadcastProgress(scanID, session.Progress, pMessage, pData)
	case "complete":
		session.Progress = 100
		// 广播完成
		r.wshub.BroadcastComplete(scanID, map[string]interface{}{
			"progress": 100,
			"message":  "扫描完成",
		})
	default:
		// 广播通用进度更新
		r.wshub.BroadcastProgress(scanID, session.Progress, pMessage, pData)
	}
	session.mu.Unlock()
	r.scansMutex.Unlock()
}

// printProgressToConsole 打印进度到服务器终端
func (r *Router) printProgressToConsole(scanID, repoName, pType, message string, data map[string]interface{}) {
	prefix := fmt.Sprintf("[Web Scan %s]", scanID[:8])

	switch pType {
	case "start":
		fmt.Printf("\n%s ════════════════════════════════════════\n", prefix)
		fmt.Printf("%s 开始扫描: %s\n", prefix, repoName)
		if totalFiles, ok := data["total_files"].(float64); ok {
			fmt.Printf("%s 总文件数: %d\n", prefix, int(totalFiles))
		}
		if totalBatches, ok := data["total_batches"].(float64); ok {
			fmt.Printf("%s 总批次数: %d\n", prefix, int(totalBatches))
		}
		fmt.Printf("%s ════════════════════════════════════════\n", prefix)

	case "batch_start":
		fmt.Printf("%s %s\n", prefix, message)

	case "batch_complete":
		if issues, ok := data["issues"].(float64); ok {
			fmt.Printf("%s   ✓ 完成 - 当前发现 %d 个问题\n", prefix, int(issues))
		} else {
			fmt.Printf("%s   ✓ 完成\n", prefix)
		}

	case "complete":
		fmt.Printf("\n%s ════════════════════════════════════════\n", prefix)
		fmt.Printf("%s 扫描完成!\n", prefix)
		fmt.Printf("%s ════════════════════════════════════════\n\n", prefix)

	case "error":
		fmt.Printf("\n%s 错误: %s\n\n", prefix, message)

	default:
		if message != "" {
			fmt.Printf("%s %s\n", prefix, message)
		}
	}
}

// scanWebSocket WebSocket 处理（用于实时推送扫描进度）
func (r *Router) scanWebSocket(c *gin.Context) {
	r.wshub.HandleWebSocket(c)
}

// ==================== GitLab 处理函数 ====================

// gitlabValidateToken 验证 GitLab Token
func (r *Router) gitlabValidateToken(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		GitlabURL string `json:"gitlab_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := gitlab.NewClient(req.Token, req.GitlabURL)
	valid, err := client.ValidateToken()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid": valid,
	})
}

// gitlabListProjects 列出 GitLab 项目
func (r *Router) gitlabListProjects(c *gin.Context) {
	token := c.GetHeader("X-GitLab-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitLab token"})
		return
	}

	gitlabURL := c.GetHeader("X-GitLab-URL")
	search := c.Query("search")
	membership := c.Query("membership") == "true"

	manager := gitlab.NewManager(token, gitlabURL)
	projects, err := manager.ListProjects(search, membership)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": projects,
		"total":    len(projects),
	})
}

// gitlabGetProject 获取 GitLab 项目详情
func (r *Router) gitlabGetProject(c *gin.Context) {
	token := c.GetHeader("X-GitLab-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitLab token"})
		return
	}

	projectID := c.Param("id")
	gitlabURL := c.GetHeader("X-GitLab-URL")

	manager := gitlab.NewManager(token, gitlabURL)
	// 这里需要解析 projectID 为 int
	// 简化处理，直接使用 client
	var pid int
	if _, err := fmt.Sscanf(projectID, "%d", &pid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	project, err := manager.GetProject(pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, project)
}

// gitlabGetBranches 获取项目分支列表
func (r *Router) gitlabGetBranches(c *gin.Context) {
	token := c.GetHeader("X-GitLab-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitLab token"})
		return
	}

	projectID := c.Param("id")
	gitlabURL := c.GetHeader("X-GitLab-URL")

	manager := gitlab.NewManager(token, gitlabURL)
	var pid int
	if _, err := fmt.Sscanf(projectID, "%d", &pid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	branches, err := manager.GetBranches(pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"branches": branches,
		"total":    len(branches),
	})
}

// gitlabScanRequest GitLab 扫描请求
type gitlabScanRequest struct {
	Token      string   `json:"token" binding:"required"`
	GitlabURL  string   `json:"gitlab_url"`
	ProjectID  int      `json:"project_id" binding:"required"`
	Branch     string   `json:"branch"`
	ScanTypes  []string `json:"scan_types"`
	BatchSize  int      `json:"batch_size"`
	MaxContext int      `json:"max_context"`
}

// gitlabScanProject 扫描 GitLab 项目
func (r *Router) gitlabScanProject(c *gin.Context) {
	var req gitlabScanRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	projectID := c.Param("id")
	var pid int
	if _, err := fmt.Sscanf(projectID, "%d", &pid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	req.ProjectID = pid

	// 创建扫描会话
	sessionID := uuid.New().String()
	session := &ScanSession{
		ID:       sessionID,
		Status:   "pending",
		Progress: 0,
		Message:  "初始化扫描任务...",
	}

	r.scansMutex.Lock()
	r.scans[sessionID] = session
	r.scansMutex.Unlock()

	// 保存到数据库（获取项目名称）
	manager := gitlab.NewManager(req.Token, req.GitlabURL)
	project, _ := manager.GetProject(req.ProjectID)
	projectName := fmt.Sprintf("GitLab 项目 #%d", req.ProjectID)
	if project != nil {
		projectName = project.NameWithNamespace
	}

	scanTypes := make([]string, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = st
	}

	scanDB := &database.ScanDB{
		ID:        sessionID,
		RepoPath:  "", // 克隆后更新
		RepoName:  projectName,
		Branch:    req.Branch,
		Status:    "pending",
		ScanTypes: scanTypes,
		StartedAt: time.Now(),
	}
	if err := r.repo.CreateScan(context.Background(), scanDB); err != nil {
		fmt.Printf("Warning: failed to save scan to database: %v\n", err)
	}

	// 异步执行扫描
	go r.runGitLabScan(session, req)

	c.JSON(http.StatusCreated, gin.H{
		"scan_id": sessionID,
		"status":  "pending",
	})
}

// runGitLabScan 执行 GitLab 项目扫描
func (r *Router) runGitLabScan(session *ScanSession, req gitlabScanRequest) {
	ctx := context.Background()

	// 获取项目信息用于显示
	manager := gitlab.NewManager(req.Token, req.GitlabURL)
	project, _ := manager.GetProject(req.ProjectID)
	projectName := fmt.Sprintf("GitLab 项目 #%d", req.ProjectID)
	if project != nil {
		projectName = project.NameWithNamespace
	}

	session.mu.Lock()
	session.Status = "cloning"
	session.Progress = 5
	session.Message = fmt.Sprintf("正在从 GitLab 克隆 %s...", projectName)
	session.RepoName = projectName
	msg := session.Message
	progress := session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 直接在 repo/{scanID}/ 目录中操作，代码不删除
	scanDir := filepath.Join("repo", session.ID)
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "创建扫描目录失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	// 更新状态为克隆中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "cloning", "")
	}

	// 设置进度回调
	r.scanner.SetProgressCallback(session.ID, func(progress interface{}) {
		r.handleProgressUpdate(session.ID, progress)
	})

	// 克隆项目到 repo/{scanID}/repo 目录
	targetRepoPath := filepath.Join(scanDir, "repo")
	err := manager.CloneIntoDir(ctx, req.ProjectID, req.Branch, targetRepoPath)
	if err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = fmt.Sprintf("克隆失败: %s", err.Error())
		session.Message = "克隆失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	session.mu.Lock()
	session.RepoPath = targetRepoPath
	session.RepoName = projectName
	session.Status = "scanning"
	session.Progress = 10
	session.Message = fmt.Sprintf("克隆成功！正在分析代码 (目录: %s)...", targetRepoPath)
	msg = session.Message
	progress = session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 更新状态为扫描中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "scanning", "")
	}

	// 构建扫描请求
	scanTypes := make([]models.ScanType, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = models.ScanType(st)
	}

	scanRequest := models.ScanRequest{
		RepoPath:   targetRepoPath,
		Branch:     req.Branch,
		ScanTypes:  scanTypes,
		BatchSize:  req.BatchSize,
		MaxContext: req.MaxContext,
		SandboxDir: scanDir,
	}

	// 执行扫描
	result, err := r.scanner.Scan(ctx, scanRequest)

	session.mu.Lock()
	defer session.mu.Unlock()

	completedAt := time.Now()

	if err != nil {
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "扫描失败: " + err.Error()
		session.Progress = 100
		r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		r.repo.CompleteScan(ctx, session.ID, completedAt, 0)
		return
	}

	session.Status = "completed"
	session.Progress = 100
	session.Result = result

	// 保存结果到数据库
	r.saveScanResultToDB(ctx, session.ID, result, projectName)

	// 保存结果为 JSON 文件
	if err := r.saveScanOutputJSON(session.ID, result, projectName); err != nil {
		fmt.Printf("Warning: failed to save output JSON: %v\n", err)
	}

	// 注册扫描任务记录
	gitlab.RegisterScanTask(session.ID, &gitlab.RepoScanTask{
		ProjectID:   req.ProjectID,
		ProjectName: projectName,
		Branch:      req.Branch,
		LocalPath:   targetRepoPath,
		StartedAt:   session.Result.StartedAt,
		Status:      "completed",
	})

	fmt.Printf("[GitLab Scan] Scan completed. Code preserved at: %s\n", targetRepoPath)
	fmt.Printf("[GitLab Scan] Output saved to: %s\n", filepath.Join(scanDir, "output_result.json"))
}

// gitlabCleanupCache 清理 GitLab 项目缓存
func (r *Router) gitlabCleanupCache(c *gin.Context) {
	token := c.GetHeader("X-GitLab-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitLab token"})
		return
	}

	projectID := c.Param("id")
	gitlabURL := c.GetHeader("X-GitLab-URL")

	manager := gitlab.NewManager(token, gitlabURL)
	var pid int
	if _, err := fmt.Sscanf(projectID, "%d", &pid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	err := manager.CleanupProject(pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cache cleaned"})
}

// ==================== GitHub 处理函数 ====================

// githubValidateToken 验证 GitHub Token
func (r *Router) githubValidateToken(c *gin.Context) {
	var req struct {
		Token     string `json:"token" binding:"required"`
		GithubURL string `json:"github_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := github.NewClient(req.Token, req.GithubURL)
	valid, username := client.ValidateToken()
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
			"error": "invalid token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":    true,
		"username": username,
	})
}

// githubListRepositories 列出 GitHub 仓库
func (r *Router) githubListRepositories(c *gin.Context) {
	token := c.GetHeader("X-GitHub-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitHub token"})
		return
	}

	githubURL := c.GetHeader("X-GitHub-URL")
	affiliation := c.Query("affiliation")

	manager := github.NewManager(token, githubURL)
	repos, err := manager.ListRepositories(affiliation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"repositories": repos,
		"total":        len(repos),
	})
}

// githubSearchRepositories 搜索 GitHub 仓库
func (r *Router) githubSearchRepositories(c *gin.Context) {
	token := c.GetHeader("X-GitHub-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitHub token"})
		return
	}

	githubURL := c.GetHeader("X-GitHub-URL")
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing query parameter"})
		return
	}

	manager := github.NewManager(token, githubURL)
	result, err := manager.SearchRepositories(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"repositories": result.Items,
		"total":        result.TotalCount,
	})
}

// githubGetRepository 获取 GitHub 仓库详情
func (r *Router) githubGetRepository(c *gin.Context) {
	token := c.GetHeader("X-GitHub-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitHub token"})
		return
	}

	owner := c.Param("owner")
	name := c.Param("name")
	githubURL := c.GetHeader("X-GitHub-URL")

	manager := github.NewManager(token, githubURL)
	repo, err := manager.GetRepository(owner, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, repo)
}

// githubGetBranches 获取仓库分支列表
func (r *Router) githubGetBranches(c *gin.Context) {
	token := c.GetHeader("X-GitHub-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitHub token"})
		return
	}

	owner := c.Param("owner")
	name := c.Param("name")
	githubURL := c.GetHeader("X-GitHub-URL")

	manager := github.NewManager(token, githubURL)
	branches, err := manager.GetBranches(owner, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"branches": branches,
		"total":    len(branches),
	})
}

// githubScanRequest GitHub 扫描请求
type githubScanRequest struct {
	Token      string   `json:"token" binding:"required"`
	GithubURL  string   `json:"github_url"`
	Owner      string   `json:"owner" binding:"required"`
	Name       string   `json:"name" binding:"required"`
	Branch     string   `json:"branch"`
	ScanTypes  []string `json:"scan_types"`
	BatchSize  int      `json:"batch_size"`
	MaxContext int      `json:"max_context"`
}

// githubScanRepository 扫描 GitHub 仓库
func (r *Router) githubScanRepository(c *gin.Context) {
	var req githubScanRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	owner := c.Param("owner")
	name := c.Param("name")
	req.Owner = owner
	req.Name = name

	// 创建扫描会话
	sessionID := uuid.New().String()
	session := &ScanSession{
		ID:       sessionID,
		Status:   "pending",
		Progress: 0,
		Message:  "初始化扫描任务...",
	}

	r.scansMutex.Lock()
	r.scans[sessionID] = session
	r.scansMutex.Unlock()

	// 保存到数据库（获取仓库名称）
	manager := github.NewManager(req.Token, req.GithubURL)
	repo, _ := manager.GetRepository(req.Owner, req.Name)
	repoName := fmt.Sprintf("%s/%s", req.Owner, req.Name)
	if repo != nil {
		repoName = repo.FullName
	}

	scanTypes := make([]string, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = st
	}

	scanDB := &database.ScanDB{
		ID:        sessionID,
		RepoPath:  "", // 克隆后更新
		RepoName:  repoName,
		Branch:    req.Branch,
		Status:    "pending",
		ScanTypes: scanTypes,
		StartedAt: time.Now(),
	}
	if err := r.repo.CreateScan(context.Background(), scanDB); err != nil {
		fmt.Printf("Warning: failed to save scan to database: %v\n", err)
	}

	// 异步执行扫描
	go r.runGitHubScan(session, req)

	c.JSON(http.StatusCreated, gin.H{
		"scan_id": sessionID,
		"status":  "pending",
	})
}

// runGitHubScan 执行 GitHub 仓库扫描
func (r *Router) runGitHubScan(session *ScanSession, req githubScanRequest) {
	ctx := context.Background()
	repoName := fmt.Sprintf("%s/%s", req.Owner, req.Name)

	session.mu.Lock()
	session.Status = "cloning"
	session.Progress = 5
	session.Message = fmt.Sprintf("正在从 GitHub 克隆 %s...", repoName)
	session.RepoName = repoName
	msg := session.Message
	progress := session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 直接在 repo/{scanID}/ 目录中操作，代码不删除
	scanDir := filepath.Join("repo", session.ID)
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "创建扫描目录失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	// 更新状态为克隆中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "cloning", "")
	}

	// 设置进度回调
	r.scanner.SetProgressCallback(session.ID, func(progress interface{}) {
		r.handleProgressUpdate(session.ID, progress)
	})

	// 创建 GitHub 管理器
	manager := github.NewManager(req.Token, req.GithubURL)

	// 获取项目信息用于显示
	repo, _ := manager.GetRepository(req.Owner, req.Name)
	if repo != nil {
		repoName = repo.FullName
	}

	// 克隆项目到 repo/{scanID}/repo 目录
	targetRepoPath := filepath.Join(scanDir, "repo")
	err := manager.CloneIntoDir(ctx, req.Owner, req.Name, req.Branch, targetRepoPath)
	if err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = fmt.Sprintf("克隆失败: %s", err.Error())
		session.Message = "克隆失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	session.mu.Lock()
	session.RepoPath = targetRepoPath
	session.RepoName = repoName
	session.Status = "scanning"
	session.Progress = 10
	session.Message = fmt.Sprintf("克隆成功！正在分析代码 (目录: %s)...", targetRepoPath)
	msg = session.Message
	progress = session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 更新状态为扫描中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "scanning", "")
	}

	// 构建扫描请求
	scanTypes := make([]models.ScanType, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = models.ScanType(st)
	}

	scanRequest := models.ScanRequest{
		RepoPath:   targetRepoPath,
		Branch:     req.Branch,
		ScanTypes:  scanTypes,
		BatchSize:  req.BatchSize,
		MaxContext: req.MaxContext,
		SandboxDir: scanDir,
	}

	// 执行扫描
	result, err := r.scanner.Scan(ctx, scanRequest)

	session.mu.Lock()
	defer session.mu.Unlock()

	completedAt := time.Now()

	if err != nil {
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "扫描失败: " + err.Error()
		session.Progress = 100
		r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		r.repo.CompleteScan(ctx, session.ID, completedAt, 0)
		return
	}

	session.Status = "completed"
	session.Progress = 100
	session.Result = result

	// 保存结果到数据库
	r.saveScanResultToDB(ctx, session.ID, result, repoName)

	// 保存结果为 JSON 文件
	if err := r.saveScanOutputJSON(session.ID, result, repoName); err != nil {
		fmt.Printf("Warning: failed to save output JSON: %v\n", err)
	}

	// 注册扫描任务记录
	github.RegisterScanTask(session.ID, &github.RepoScanTask{
		Owner:     req.Owner,
		Name:      req.Name,
		FullName:  repoName,
		Branch:    req.Branch,
		LocalPath: targetRepoPath,
		StartedAt: session.Result.StartedAt,
		Status:    "completed",
	})

	fmt.Printf("[GitHub Scan] Scan completed. Code preserved at: %s\n", targetRepoPath)
	fmt.Printf("[GitHub Scan] Output saved to: %s\n", filepath.Join(scanDir, "output_result.json"))
}

// githubCleanupCache 清理 GitHub 仓库缓存
func (r *Router) githubCleanupCache(c *gin.Context) {
	token := c.GetHeader("X-GitHub-Token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing GitHub token"})
		return
	}

	owner := c.Param("owner")
	name := c.Param("name")
	githubURL := c.GetHeader("X-GitHub-URL")

	manager := github.NewManager(token, githubURL)
	err := manager.CleanupProject(owner, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cache cleaned"})
}

// ==================== Git URL 克隆处理函数 ====================

// scanByUrlRequest 通过 URL 扫描的请求
type scanByUrlRequest struct {
	URL        string   `json:"url" binding:"required"`
	Branch     string   `json:"branch"`
	ScanTypes  []string `json:"scan_types"`
	BatchSize  int      `json:"batch_size"`
	MaxContext int      `json:"max_context"`
}

// scanByUrl 通过 Git URL 直接扫描（无需 token）
func (r *Router) scanByUrl(c *gin.Context) {
	var req scanByUrlRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调试日志
	fmt.Printf("Received scan request: URL=%s, Branch=%s, ScanTypes=%v\n", req.URL, req.Branch, req.ScanTypes)

	// 验证 URL 格式
	urlInfo, err := git.ParseRepoURL(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid repository URL: " + err.Error()})
		return
	}

	fmt.Printf("Parsed URL info: Platform=%s, Owner=%s, Name=%s\n", urlInfo.Platform, urlInfo.Owner, urlInfo.Name)

	// 创建扫描会话
	sessionID := uuid.New().String()
	session := &ScanSession{
		ID:       sessionID,
		Status:   "pending",
		Progress: 0,
		Message:  "初始化扫描任务...",
	}

	r.scansMutex.Lock()
	r.scans[sessionID] = session
	r.scansMutex.Unlock()

	// 保存到数据库
	repoName := fmt.Sprintf("%s/%s", urlInfo.Owner, urlInfo.Name)
	scanTypes := make([]string, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = st
	}

	scanDB := &database.ScanDB{
		ID:        sessionID,
		RepoPath:  "", // 克隆后更新
		RepoName:  repoName,
		Branch:    req.Branch,
		Status:    "pending",
		ScanTypes: scanTypes,
		StartedAt: time.Now(),
	}
	if err := r.repo.CreateScan(context.Background(), scanDB); err != nil {
		fmt.Printf("Warning: failed to save scan to database: %v\n", err)
	}

	// 异步执行扫描
	go r.runURLScan(session, req, urlInfo)

	c.JSON(http.StatusCreated, gin.H{
		"scan_id": sessionID,
		"status":  "pending",
	})
}

// runURLScan 执行 URL 扫描
func (r *Router) runURLScan(session *ScanSession, req scanByUrlRequest, urlInfo *git.RepoURLInfo) {
	ctx := context.Background()
	repoName := fmt.Sprintf("%s/%s", urlInfo.Owner, urlInfo.Name)

	session.mu.Lock()
	session.Status = "cloning"
	session.Progress = 5
	session.Message = fmt.Sprintf("正在克隆仓库 %s...", repoName)
	session.RepoName = repoName
	msg := session.Message
	progress := session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 直接在 repo/{scanID}/ 目录中操作，代码不删除
	scanDir := filepath.Join("repo", session.ID)
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "创建扫描目录失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	// 更新状态为克隆中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "cloning", "")
	}

	// 设置进度回调
	r.scanner.SetProgressCallback(session.ID, func(progress interface{}) {
		r.handleProgressUpdate(session.ID, progress)
	})

	// 克隆项目到 repo/{scanID}/repo 目录
	targetRepoPath := filepath.Join(scanDir, "repo")
	_, err := git.CloneRepository(git.CloneOptions{
		URL:       req.URL,
		Branch:    req.Branch,
		TargetDir: targetRepoPath,
	})
	if err != nil {
		session.mu.Lock()
		session.Status = "failed"
		session.Error = fmt.Sprintf("克隆失败: %s", err.Error())
		session.Message = "克隆失败: " + err.Error()
		session.Progress = 100
		msg = session.Message
		session.mu.Unlock()
		r.wshub.BroadcastError(session.ID, msg)
		if r.repo != nil {
			r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		}
		return
	}

	session.mu.Lock()
	session.RepoPath = targetRepoPath
	session.RepoName = repoName
	session.Status = "scanning"
	session.Progress = 10
	session.Message = fmt.Sprintf("克隆成功！正在分析代码 (目录: %s)...", targetRepoPath)
	msg = session.Message
	progress = session.Progress
	session.mu.Unlock()
	// 广播进度更新到前端
	r.wshub.BroadcastProgress(session.ID, progress, msg, nil)

	// 更新状态为扫描中
	if r.repo != nil {
		r.repo.UpdateScanStatus(ctx, session.ID, "scanning", "")
	}

	// 构建扫描请求
	scanTypes := make([]models.ScanType, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = models.ScanType(st)
	}

	scanRequest := models.ScanRequest{
		RepoPath:   targetRepoPath,
		Branch:     req.Branch,
		ScanTypes:  scanTypes,
		BatchSize:  req.BatchSize,
		MaxContext: req.MaxContext,
		SandboxDir: scanDir,
	}

	// 执行扫描
	result, err := r.scanner.Scan(ctx, scanRequest)

	session.mu.Lock()
	defer session.mu.Unlock()

	completedAt := time.Now()

	if err != nil {
		session.Status = "failed"
		session.Error = err.Error()
		session.Message = "扫描失败: " + err.Error()
		session.Progress = 100
		r.repo.UpdateScanStatus(ctx, session.ID, "failed", err.Error())
		r.repo.CompleteScan(ctx, session.ID, completedAt, 0)
		return
	}

	session.Status = "completed"
	session.Progress = 100
	session.Result = result

	// 保存结果到数据库
	r.saveScanResultToDB(ctx, session.ID, result, repoName)

	// 保存结果为 JSON 文件
	if err := r.saveScanOutputJSON(session.ID, result, repoName); err != nil {
		fmt.Printf("Warning: failed to save output JSON: %v\n", err)
	}

	fmt.Printf("[URL Scan] Scan completed. Code preserved at: %s\n", targetRepoPath)
	fmt.Printf("[URL Scan] Output saved to: %s\n", filepath.Join(scanDir, "output_result.json"))
}

// parseGitURL 解析 Git URL
func (r *Router) parseGitURL(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url parameter"})
		return
	}

	urlInfo, err := git.ParseRepoURL(url)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, urlInfo)
}

// getRemoteBranches 获取远程分支列表
func (r *Router) getRemoteBranches(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url parameter"})
		return
	}

	branches, err := git.GetRemoteBranches(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"branches": branches,
		"total":    len(branches),
	})
}

// Run 运行服务器
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

// GetEngine 获取 Gin 引擎
func (r *Router) GetEngine() *gin.Engine {
	return r.engine
}

// ==================== JSON 输出相关函数 ====================

// ScanOutputJSON 扫描结果 JSON 输出格式
type ScanOutputJSON struct {
	ScanID       string            `json:"scan_id"`
	RepoPath     string            `json:"repo_path"`
	RepoName     string            `json:"repo_name"`
	Branch       string            `json:"branch"`
	Status       string            `json:"status"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	TotalFiles   int               `json:"total_files"`
	TotalBatches int               `json:"total_batches"`
	TotalIssues  int               `json:"total_issues"`
	Summary      ScanSummaryJSON   `json:"summary"`
	Batches      []BatchOutputJSON `json:"batches"`
	Metadata     MetadataJSON      `json:"metadata"`
}

// ScanSummaryJSON 扫描摘要 JSON
type ScanSummaryJSON struct {
	IssuesBySeverity map[string]int `json:"issues_by_severity"`
	IssuesByType     map[string]int `json:"issues_by_type"`
	TotalFiles       int            `json:"total_files"`
	TotalBatches     int            `json:"total_batches"`
	CompletedBatches int            `json:"completed_batches"`
	TotalIssues      int            `json:"total_issues"`
}

// BatchOutputJSON 批次输出 JSON
type BatchOutputJSON struct {
	BatchID      int         `json:"batch_id"`
	Files        []string    `json:"files"`
	Status       string      `json:"status"`
	StartedAt    time.Time   `json:"started_at"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	TokensUsed   int         `json:"tokens_used"`
	Issues       []IssueJSON `json:"issues"`
}

// IssueJSON 问题输出 JSON
type IssueJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Severity    string   `json:"severity"`
	Type        string   `json:"type"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	CodeSnippet string   `json:"code_snippet"`
	Description string   `json:"description"`
	CWE         string   `json:"cwe,omitempty"`
	References  []string `json:"references,omitempty"`
}

// MetadataJSON 元数据 JSON
type MetadataJSON struct {
	Version     string    `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	Scanner     string    `json:"scanner"`
}

// saveScanOutputJSON 保存扫描结果为 JSON 文件
func (r *Router) saveScanOutputJSON(scanID string, result *models.ScanResult, repoName string) error {
	// 创建输出目录
	outputDir := filepath.Join("repo", scanID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 转换批次
	batches := make([]BatchOutputJSON, len(result.Batches))
	for i, b := range result.Batches {
		issues := make([]IssueJSON, len(b.Issues))
		for j, iss := range b.Issues {
			issues[j] = IssueJSON{
				ID:          iss.ID,
				Title:       iss.Title,
				Severity:    string(iss.Severity),
				Type:        string(iss.ScanType),
				File:        iss.File,
				Line:        iss.Line,
				CodeSnippet: iss.CodeSnippet,
				Description: iss.Description,
				CWE:         iss.CWE,
				References:  iss.References,
			}
		}

		batches[i] = BatchOutputJSON{
			BatchID:     b.BatchID,
			Files:       b.Files,
			Status:      string(b.Status),
			StartedAt:   b.StartedAt,
			CompletedAt: b.CompletedAt,
			TokensUsed:  b.TokensUsed,
			Issues:      issues,
		}
	}

	// 转换摘要
	issuesBySeverity := make(map[string]int)
	for k, v := range result.Summary.IssuesBySeverity {
		issuesBySeverity[string(k)] = v
	}

	issuesByType := make(map[string]int)
	for k, v := range result.Summary.IssuesByType {
		issuesByType[string(k)] = v
	}

	summary := ScanSummaryJSON{
		IssuesBySeverity: issuesBySeverity,
		IssuesByType:     issuesByType,
		TotalFiles:       result.Summary.TotalFiles,
		TotalBatches:     result.Summary.TotalBatches,
		CompletedBatches: result.Summary.CompletedBatches,
		TotalIssues:      result.Summary.TotalIssues,
	}

	// 构建输出
	completedAt := result.CompletedAt
	if completedAt == nil {
		now := time.Now()
		completedAt = &now
	}

	output := ScanOutputJSON{
		ScanID:       scanID,
		RepoPath:     result.RepoPath,
		RepoName:     repoName,
		Branch:       result.Branch,
		Status:       string(result.Status),
		StartedAt:    result.StartedAt,
		CompletedAt:  completedAt,
		TotalFiles:   result.Summary.TotalFiles,
		TotalBatches: result.Summary.TotalBatches,
		TotalIssues:  result.Summary.TotalIssues,
		Summary:      summary,
		Batches:      batches,
		Metadata: MetadataJSON{
			Version:     "1.0.0",
			GeneratedAt: time.Now(),
			Scanner:     "code-audit-claw",
		},
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// 写入文件
	outputFile := filepath.Join(outputDir, "output_result.json")
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	fmt.Printf("[Output] Saved scan result to %s\n", outputFile)
	return nil
}

// getScanOutput 获取扫描结果 JSON 数据
func (r *Router) getScanOutput(c *gin.Context) {
	scanID := c.Param("id")

	// 尝试从 JSON 文件读取
	outputFile := filepath.Join("repo", scanID, "output_result.json")
	data, err := os.ReadFile(outputFile)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果 JSON 文件不存在，尝试从数据库读取
			scanDB, err := r.repo.GetScan(context.Background(), scanID)
			if err != nil || scanDB == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "scan output not found"})
				return
			}

			// 获取批次和问题
			batchesDB, _ := r.repo.ListBatches(context.Background(), scanID)
			filter := &database.ListIssuesFilter{
				ScanID: scanID,
				Limit:  10000,
			}
			issues, _, _ := r.repo.ListIssues(context.Background(), filter)

			// 构建响应
			c.JSON(http.StatusOK, gin.H{
				"scan_id":       scanDB.ID,
				"repo_path":     scanDB.RepoPath,
				"repo_name":     scanDB.RepoName,
				"branch":        scanDB.Branch,
				"status":        scanDB.Status,
				"started_at":    scanDB.StartedAt,
				"completed_at":  scanDB.CompletedAt,
				"total_files":   scanDB.TotalFiles,
				"total_batches": scanDB.TotalBatches,
				"total_issues":  scanDB.TotalIssues,
				"batches":       batchesDB,
				"issues":        issues,
				"source":        "database",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 直接返回 JSON 文件内容
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(data))
}

// getScanOutputFile 获取扫描结果 JSON 文件
func (r *Router) getScanOutputFile(c *gin.Context) {
	scanID := c.Param("id")

	outputFile := filepath.Join("repo", scanID, "output_result.json")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "output file not found"})
		return
	}

	c.File(outputFile)
}

// ==================== 文件操作辅助函数 ====================

// copyPath 复制文件或目录
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

// copyFile 复制单个文件
func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := destFile.ReadFrom(srcFile); err != nil {
		return err
	}

	// 保留文件权限
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, srcInfo.Mode())
}

// copyDir 复制目录
func copyDir(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// 创建目标目录
	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	// 读取目录内容
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// 复制每个条目
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			// 跳过某些目录
			if shouldSkipDirCopy(entry.Name()) {
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

// shouldSkipDirCopy 判断是否应该跳过复制该目录
func shouldSkipDirCopy(name string) bool {
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
