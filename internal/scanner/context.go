package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContextManager 管理扫描上下文
// 核心功能：在上下文窗口限制下，智能组织代码内容
type ContextManager struct {
	maxTokens      int
	tokensPerLine  int
	maxLinesPerFile int
}

// NewContextManager 创建上下文管理器
func NewContextManager(maxTokens int) *ContextManager {
	return &ContextManager{
		maxTokens:      maxTokens,
		tokensPerLine:  20,  // 平均每行token数
		maxLinesPerFile: 500, // 单文件最大行数（超过则截断）
	}
}

// BuildContext 构建扫描上下文
// 返回：实际使用的文件列表、组合后的上下文文本
func (cm *ContextManager) BuildContext(files []FileWithTokens) ([]*FileContent, string, error) {
	var result []*FileContent
	var contextParts []string
	usedTokens := 0

	for _, file := range files {
		// 读取文件内容
		content, err := cm.readFileWithLimit(file.Path)
		if err != nil {
			continue // 跳过无法读取的文件
		}

		// 估算实际token数
		estimatedTokens := len(content.Lines) * cm.tokensPerLine
		if estimatedTokens > file.Tokens {
			estimatedTokens = file.Tokens
		}

		// 检查是否超出限制
		if usedTokens+estimatedTokens > cm.maxTokens {
			// 尝试截断当前文件
			remainingTokens := cm.maxTokens - usedTokens
			remainingLines := remainingTokens / cm.tokensPerLine

			if remainingLines > 10 { // 至少保留10行
				content.Lines = content.Lines[:remainingLines]
				content.Truncated = true
				result = append(result, content)
				contextParts = append(contextParts, cm.formatFileContent(content))
			}
			break // 已达到token限制
		}

		result = append(result, content)
		contextParts = append(contextParts, cm.formatFileContent(content))
		usedTokens += estimatedTokens
	}

	return result, strings.Join(contextParts, "\n\n"), nil
}

// FileContent 文件内容
type FileContent struct {
	Path      string
	Language  string
	Lines     []string
	Truncated bool
}

// readFileWithLimit 读取文件内容（带行数限制）
func (cm *ContextManager) readFileWithLimit(path string) (*FileContent, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")

	// 限制行数
	if len(lines) > cm.maxLinesPerFile {
		lines = lines[:cm.maxLinesPerFile]
	}

	// 检测语言
	language := cm.detectLanguage(path)

	return &FileContent{
		Path:      path,
		Language:  language,
		Lines:     lines,
		Truncated: len(lines) < len(strings.Split(string(content), "\n")),
	}, nil
}

// formatFileContent 格式化文件内容用于发送给Claude
func (cm *ContextManager) formatFileContent(fc *FileContent) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## File: %s (%s)", fc.Path, fc.Language))
	if fc.Truncated {
		sb.WriteString(" [TRUNCATED]")
	}
	sb.WriteString("\n```\n")

	// 添加行号
	for i, line := range fc.Lines {
		sb.WriteString(fmt.Sprintf("%4d | %s\n", i+1, line))
	}

	sb.WriteString("```\n")
	return sb.String()
}

// detectLanguage 检测文件语言
func (cm *ContextManager) detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".go":     "Go",
		".js":     "JavaScript",
		".ts":     "TypeScript",
		".tsx":    "TypeScript",
		".jsx":    "JavaScript",
		".py":     "Python",
		".java":   "Java",
		".c":      "C",
		".cpp":    "C++",
		".cc":     "C++",
		".h":      "C/C++ Header",
		".hpp":    "C++ Header",
		".cs":     "C#",
		".php":    "PHP",
		".rb":     "Ruby",
		".rs":     "Rust",
		".kt":     "Kotlin",
		".swift":  "Swift",
		".scala":  "Scala",
		".sh":     "Shell",
		".yml":    "YAML",
		".yaml":   "YAML",
		".json":   "JSON",
		".xml":    "XML",
		".html":   "HTML",
		".css":    "CSS",
		".sql":    "SQL",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "Unknown"
}

// BuildSystemPrompt 构建系统提示词
func (cm *ContextManager) BuildSystemPrompt(scanTypes []string) string {
	// 检测主要语言
	primaryLang := cm.detectPrimaryLanguage(scanTypes)

	prompt := fmt.Sprintf(`你是一个%s语言安全专家。

## 任务要求

0. 首先 commit，备份代码，已经备份就跳过；

1. 请先分析项目结构，然后给出代码**安全审计**，不需要给出修复建议及相应时间计划；

2. 审计类型包括 XSS，SSTI，RCE，SQLi，SSRF等，忽略以下类型CORS；

3. 针对每个漏洞，给出详细报告和严重性POC，给出中英文两份漏洞报告，报告包含四部分：
   - 介绍
   - 影响版本
   - 分析细节
   - POC

4. 然后针对每个漏洞展开细致的检验；

5. 然后对每个漏洞POC展开检验；

6. 梳理文档。

## 输出格式

请以JSON格式输出，包含以下字段：
{
  "project_analysis": "项目结构分析",
  "issues": [
    {
      "id": "唯一ID",
      "title_cn": "漏洞标题（中文）",
      "title_en": "漏洞标题（英文）",
      "severity": "critical|high|medium|low|info",
      "type": "XSS|SSTI|RCE|SQLi|SSRF|Other",
      "file": "文件路径",
      "line": 行号,
      "code_snippet": "问题代码",
      "description": "详细描述",
      "introduction_cn": "漏洞介绍（中文）",
      "introduction_en": "漏洞介绍（英文）",
      "affected_versions": "影响版本",
      "analysis_detail": "分析细节",
      "poc": "POC代码",
      "poc_verification": "POC验证结果"
    }
  ]
}

请开始审计：`, primaryLang)

	return prompt
}

// detectPrimaryLanguage 检测主要编程语言
func (cm *ContextManager) detectPrimaryLanguage(scanTypes []string) string {
	// 尝试从上下文中获取实际的语言信息
	// 这里简化处理，返回通用的描述
	if len(scanTypes) > 0 {
		return "多语言"
	}
	return "代码"
}

// HandleContextOverflow 处理上下文溢出
// 当Claude返回上下文超限时，自动调整策略
func (cm *ContextManager) HandleContextOverflow(attempt int) {
	// 每次溢出时减少25%的token限制
	reduction := 1.0 - (0.25 * float64(attempt))
	cm.maxTokens = int(float64(cm.maxTokens) * reduction)

	// 同时减少单文件最大行数
	cm.maxLinesPerFile = int(float64(cm.maxLinesPerFile) * reduction)
}
