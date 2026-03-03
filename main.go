package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/auditor/code-audit-claw/api"
	"github.com/auditor/code-audit-claw/internal/database"
	"github.com/auditor/code-audit-claw/internal/models"
	"github.com/auditor/code-audit-claw/internal/scanner"
)

var (
	version   = "1.0.0"
	claudeCmd = flag.String("claude", "claude", "Path to claude command")
)

// scannerAdapter 适配器，将 scanner.Scanner 转换为 api.ScannerInterface
type scannerAdapter struct {
	s *scanner.Scanner
}

func (a *scannerAdapter) Scan(ctx context.Context, req models.ScanRequest) (*models.ScanResult, error) {
	result, err := a.s.Scan(ctx, req)
	if err != nil {
		return nil, err
	}
	modelResult := result.ToModel()
	return &modelResult, nil
}

func (a *scannerAdapter) SetProgressCallback(scanID string, cb func(interface{})) {
	a.s.SetProgressCallback(scanID, cb)
}

// CLI 命令
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

var commands = []Command{
	{
		Name:        "scan",
		Description: "扫描代码目录并输出审计报告",
		Run:         runScan,
	},
	{
		Name:        "server",
		Description: "启动 Web 服务器",
		Run:         runServer,
	},
}

func main() {
	flag.Parse()

	// 检查 claude 命令
	if _, err := exec.LookPath(*claudeCmd); err != nil {
		log.Printf("Warning: claude command not found at %s", *claudeCmd)
		log.Println("Please ensure Claude CLI is installed and accessible")
	}

	args := flag.Args()

	// 没有参数时，默认启动 server
	if len(args) == 0 {
		if err := runServer(nil); err != nil {
			log.Fatal(err)
		}
		return
	}

	// 查找并执行命令
	for _, cmd := range commands {
		if cmd.Name == args[0] {
			if err := cmd.Run(args[1:]); err != nil {
				log.Fatalf("Error: %v", err)
			}
			return
		}
	}

	fmt.Printf("Unknown command: %s\n", args[0])
	printUsage()
	os.Exit(1)
}

func printUsage() {
	fmt.Printf("CodeAuditClaw v%s\n", version)
	fmt.Println("\n用法:")
	fmt.Println("  code-auditor [选项]            # 默认启动 Web 服务器")
	fmt.Println("  code-auditor [选项] <命令> [参数...]")
	fmt.Println("\n命令:")
	for _, cmd := range commands {
		if cmd.Name == "server" {
			fmt.Printf("  %-10s %s (默认模式)\n", cmd.Name, cmd.Description)
		} else {
			fmt.Printf("  %-10s %s\n", cmd.Name, cmd.Description)
		}
	}
	fmt.Println("\nscan 命令参数:")
	fmt.Println("  <目录>              要扫描的代码目录")
	fmt.Println("  [--branch <分支>]  Git 分支（可选）")
	fmt.Println("  [--batch <N>]      每批文件数（默认: 5）")
	fmt.Println("  [--tokens <N>]     最大上下文 token 数（默认: 100000）")
	fmt.Println("\nserver 命令参数:")
	fmt.Println("  [--addr <地址>]    服务器地址（默认: :8080）")
	fmt.Println("\n全局选项:")
	flag.PrintDefaults()
	fmt.Println("\n示例:")
	fmt.Println("  code-auditor                  # 启动 Web 服务器（默认）")
	fmt.Println("  code-auditor --addr :9090     # 指定端口启动")
	fmt.Println("  code-auditor scan ./myproject # CLI 模式扫描")
	fmt.Println("  code-auditor scan ./myproject --batch 10 --tokens 200000")
}

// runScan 执行扫描命令
func runScan(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定要扫描的目录")
	}

	repoPath := args[0]
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("目录不存在: %s", repoPath)
	}

	// 解析参数
	branch := ""
	batchSize := 5
	maxTokens := 100000

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "--batch":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &batchSize)
				i++
			}
		case "--tokens":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxTokens)
				i++
			}
		}
	}

	// 创建扫描器
	coreScanner := scanner.NewScanner(batchSize, maxTokens, *claudeCmd)

	// 设置进度回调 - 实时输出到终端
	totalBatches := 0
	completedBatches := 0
	totalIssues := 0

	coreScanner.SetProgressCallback("", func(progress interface{}) {
		// 解析进度数据
		p, ok := progress.(map[string]interface{})
		if !ok {
			return
		}
		pType, _ := p["type"].(string)
		pMessage, _ := p["message"].(string)
		pData, _ := p["data"].(map[string]interface{})

		var batchID int
		if b, ok := p["batch_id"].(float64); ok {
			batchID = int(b)
		}

		switch pType {
		case "start":
			if n, ok := pData["total_batches"].(float64); ok {
				totalBatches = int(n)
			}
			if n, ok := pData["total_files"].(float64); ok {
				fmt.Printf("\n=== 开始扫描 ===\n")
				fmt.Printf("总文件数: %d\n", int(n))
				fmt.Printf("总批次数: %d\n\n", totalBatches)
			}
		case "batch_start":
			fmt.Printf("[%d/%d] %s\n", batchID+1, totalBatches, pMessage)
		case "batch_complete":
			completedBatches++
			if issues, ok := pData["issues"].(float64); ok {
				totalIssues += int(issues)
			}
			fmt.Printf("      ✓ 完成 - 发现 %d 个问题\n", totalIssues)
		case "complete":
			fmt.Printf("\n=== 扫描完成 ===\n")
		case "error":
			fmt.Printf("\n错误: %s\n", pMessage)
		}
	})

	// 构建扫描请求
	scanTypes := []models.ScanType{models.ScanTypeSecurity}
	scanRequest := models.ScanRequest{
		RepoPath:   repoPath,
		Branch:     branch,
		ScanTypes:  scanTypes,
		BatchSize:  batchSize,
		MaxContext: maxTokens,
	}

	fmt.Printf("\nCodeAuditClaw v%s - 代码安全审计工具\n", version)
	absPath, _ := filepath.Abs(repoPath)
	fmt.Printf("目标目录: %s\n", absPath)
	fmt.Printf("Claude 命令: %s\n\n", *claudeCmd)

	// 执行扫描
	ctx := context.Background()
	result, err := coreScanner.Scan(ctx, scanRequest)

	fmt.Println() // 空行

	if err != nil {
		return fmt.Errorf("扫描失败: %w", err)
	}

	// 输出结果
	printResults(result)

	return nil
}

// printResults 打印扫描结果
func printResults(result *scanner.ScanResult) {
	fmt.Println("═════════════════════════════════════════════════════")
	fmt.Println("                   扫描结果摘要")
	fmt.Println("═════════════════════════════════════════════════════")

	// 基本信息
	fmt.Printf("\n仓库: %s\n", result.RepoName)
	fmt.Printf("分支: %s\n", result.Branch)
	fmt.Printf("状态: %s\n", result.Status)
	if result.CompletedAt != nil {
		duration := result.CompletedAt.Sub(result.StartedAt)
		fmt.Printf("耗时: %v\n", duration.Round(time.Second))
	}

	// 统计信息
	fmt.Println("\n───────────────────────────────────────────────────")
	fmt.Println("统计信息")
	fmt.Println("───────────────────────────────────────────────────")
	fmt.Printf("总文件数:    %d\n", result.Summary.TotalFiles)
	fmt.Printf("总批次数:    %d\n", result.Summary.TotalBatches)
	fmt.Printf("总问题数:    %d\n", result.Summary.TotalIssues)

	// 按严重程度统计
	fmt.Println("\n按严重程度分布:")
	if count := result.Summary.IssuesBySeverity[models.SeverityCritical]; count > 0 {
		fmt.Printf("  🔴 Critical (严重):  %d\n", count)
	}
	if count := result.Summary.IssuesBySeverity[models.SeverityHigh]; count > 0 {
		fmt.Printf("  🟠 High (高):       %d\n", count)
	}
	if count := result.Summary.IssuesBySeverity[models.SeverityMedium]; count > 0 {
		fmt.Printf("  🟡 Medium (中):     %d\n", count)
	}
	if count := result.Summary.IssuesBySeverity[models.SeverityLow]; count > 0 {
		fmt.Printf("  🟢 Low (低):        %d\n", count)
	}
	if count := result.Summary.IssuesBySeverity[models.SeverityInfo]; count > 0 {
		fmt.Printf("  🔵 Info (信息):     %d\n", count)
	}

	// 按类型统计
	fmt.Println("\n按漏洞类型分布:")
	if count := result.Summary.IssuesByType[models.ScanTypeSecurity]; count > 0 {
		fmt.Printf("  安全漏洞:  %d\n", count)
	}

	// 详细问题列表
	if result.Summary.TotalIssues > 0 {
		fmt.Println("\n═════════════════════════════════════════════════════")
		fmt.Println("                   发现的问题")
		fmt.Println("═════════════════════════════════════════════════════")

		for _, batch := range result.Batches {
			for _, issue := range batch.Issues {
				printIssue(issue)
			}
		}
	}

	fmt.Println("\n═════════════════════════════════════════════════════")
}

// printIssue 打印单个问题详情
func printIssue(issue models.Issue) {
	severityIcon := map[models.Severity]string{
		models.SeverityCritical: "🔴",
		models.SeverityHigh:     "🟠",
		models.SeverityMedium:   "🟡",
		models.SeverityLow:      "🟢",
		models.SeverityInfo:     "🔵",
	}

	icon := severityIcon[issue.Severity]
	if icon == "" {
		icon = "⚪"
	}

	fmt.Printf("\n%s [%s] %s\n", icon, issue.Severity, issue.Title)
	fmt.Printf("   类型: %s\n", issue.ScanType)
	fmt.Printf("   位置: %s:%d\n", issue.File, issue.Line)

	if issue.CodeSnippet != "" {
		fmt.Printf("   代码:\n")
		// 限制代码片段显示长度
		snippet := issue.CodeSnippet
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		fmt.Printf("   %s\n", snippet)
	}

	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 300 {
			desc = desc[:300] + "..."
		}
		fmt.Printf("   描述: %s\n", desc)
	}

	if issue.RuleID != "" {
		fmt.Printf("   规则: %s\n", issue.RuleID)
	}
}

// runServer 启动 Web 服务器
func runServer(args []string) error {
	addr := ":8080"

	// 解析参数
	for i := 0; i < len(args); i++ {
		if args[i] == "--addr" && i+1 < len(args) {
			addr = args[i+1]
			i++
		}
	}

	// 初始化数据库
	dbConfig := database.DefaultConfig()
	db, err := database.Open(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// 初始化数据库表结构
	if err := db.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// 创建 Repository
	repo := database.NewRepository(db)
	log.Println("Database initialized successfully")

	// 清理孤立的 pending 扫描记录
	if err := cleanupPendingScans(repo); err != nil {
		log.Printf("Warning: failed to cleanup pending scans: %v", err)
	}

	// 创建扫描器
	coreScanner := scanner.NewScanner(5, 100000, *claudeCmd)

	// 创建 API 路由器
	router := api.NewRouter(&scannerAdapter{s: coreScanner}, nil, nil, repo)

	// 启动服务器
	fmt.Printf("CodeAuditClaw v%s\n", version)
	fmt.Printf("Server listening on %s\n", addr)
	fmt.Println("Press Ctrl+C to stop")

	return router.Run(addr)
}

// cleanupPendingScans 清理孤立的 pending 扫描记录
func cleanupPendingScans(repo *database.Repository) error {
	filter := &database.ListScansFilter{
		Status: "pending",
		Limit:  1000,
	}
	scans, _, err := repo.ListScans(context.Background(), filter)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-5 * time.Minute)
	for _, scan := range scans {
		if scan.StartedAt.Before(cutoff) {
			log.Printf("Cleaning up stale pending scan: %s (%s)", scan.ID, scan.RepoName)
			repo.UpdateScanStatus(context.Background(), scan.ID, "failed", "扫描任务因服务重启而终止")
			repo.CompleteScan(context.Background(), scan.ID, time.Now(), 0)
		}
	}

	return nil
}
