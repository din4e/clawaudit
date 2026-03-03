package scanner

import (
	"path/filepath"
	"strings"
)

// BatchManager 管理文件分批
type BatchManager struct {
	batchSize   int
	maxTokens   int
	extensions  []string // 支持的文件扩展名
	excludeDirs []string // 排除的目录
}

// NewBatchManager 创建批次管理器
func NewBatchManager(batchSize, maxTokens int) *BatchManager {
	return &BatchManager{
		batchSize: batchSize,
		maxTokens: maxTokens,
		extensions: []string{
			".go", ".js", ".ts", ".py", ".java", ".c", ".cpp", ".h", ".cs",
			".php", ".rb", ".rs", ".kt", ".swift", ".scala", ".sh", ".yml", ".yaml",
		},
		excludeDirs: []string{
			"vendor", "node_modules", ".git", "dist", "build", "target",
			".idea", ".vscode", "coverage", "__pycache__",
		},
	}
}

// FileWithTokens 带token估算的文件
type FileWithTokens struct {
	Path   string
	Tokens int
}

// CreateBatches 创建扫描批次
// 策略：优先按文件数量分批，同时考虑token限制
func (bm *BatchManager) CreateBatches(files []string) [][]FileWithTokens {
	// 过滤支持的文件
	filtered := bm.filterFiles(files)

	// 估算token并分组
	fileTokens := bm.estimateTokens(filtered)

	var batches [][]FileWithTokens
	var currentBatch []FileWithTokens
	currentTokens := 0

	for _, ft := range fileTokens {
		// 如果单个文件就超过限制，单独成批（截断处理）
		if ft.Tokens > bm.maxTokens {
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = nil
				currentTokens = 0
			}
			// 标记需要截断
			ft.Tokens = bm.maxTokens
			batches = append(batches, []FileWithTokens{ft})
			continue
		}

		// 检查是否需要新建批次
		if len(currentBatch) >= bm.batchSize ||
			(currentTokens+ft.Tokens > bm.maxTokens && len(currentBatch) > 0) {
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentTokens = 0
		}

		currentBatch = append(currentBatch, ft)
		currentTokens += ft.Tokens
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// filterFiles 过滤文件
func (bm *BatchManager) filterFiles(files []string) []string {
	var result []string

	for _, file := range files {
		// 检查扩展名
		ext := strings.ToLower(filepath.Ext(file))
		if !bm.isSupportedExt(ext) {
			continue
		}

		// 检查是否在排除目录中
		if bm.isInExcludedDir(file) {
			continue
		}

		result = append(result, file)
	}

	return result
}

// isSupportedExt 检查是否支持该扩展名
func (bm *BatchManager) isSupportedExt(ext string) bool {
	for _, supported := range bm.extensions {
		if ext == supported {
			return true
		}
	}
	return true // 默认都支持
}

// isInExcludedDir 检查是否在排除目录中
func (bm *BatchManager) isInExcludedDir(path string) bool {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")

	for _, part := range parts {
		for _, excluded := range bm.excludeDirs {
			if part == excluded {
				return true
			}
		}
	}
	return false
}

// estimateTokens 估算文件的token数量
// 简单估算：中文约1字符=1token，英文约4字符=1token
func (bm *BatchManager) estimateTokens(files []string) []FileWithTokens {
	result := make([]FileWithTokens, 0, len(files))

	for _, file := range files {
		// 这里应该读取文件内容进行计算
		// 简化处理：假设平均每个文件2000 tokens
		// 实际实现中应该读取文件内容并精确计算
		tokens := 2000 // 估算值
		result = append(result, FileWithTokens{
			Path:   file,
			Tokens: tokens,
		})
	}

	return result
}

// GetBatchStats 获取批次统计信息
func (bm *BatchManager) GetBatchStats(batches [][]FileWithTokens) BatchStats {
	stats := BatchStats{
		TotalBatches: len(batches),
	}

	for _, batch := range batches {
		stats.TotalFiles += len(batch)
		for _, file := range batch {
			stats.EstimatedTokens += file.Tokens
		}
	}

	stats.AvgFilesPerBatch = stats.TotalFiles / stats.TotalBatches
	if stats.TotalBatches > 0 {
		stats.AvgTokensPerBatch = stats.EstimatedTokens / stats.TotalBatches
	}

	return stats
}

// BatchStats 批次统计
type BatchStats struct {
	TotalBatches      int
	TotalFiles        int
	EstimatedTokens   int
	AvgFilesPerBatch  int
	AvgTokensPerBatch int
}
