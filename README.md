# CodeAuditClaw - 自动化代码审计工具

## 功能特性

- **分批扫描**：自动将大仓库拆分为多个批次处理，避免超出 API 上下文限制
- **智能上下文管理**：动态调整每批文件的 token 数量，处理上下文溢出
- **综合审计**：支持安全漏洞、代码质量、敏感信息、合规检查等多种扫描类型
- **Web 界面**：实时展示扫描进度、批次状态、问题详情
- **并发处理**：支持多批次并发扫描，提高效率
- **GitLab 集成**：支持从 GitLab 拉取代码进行审计
- **GitHub 集成**：支持从 GitHub 拉取代码进行审计

## 项目结构

```
code-audit-claw/
├── main.go                 # 程序入口
├── .air.toml               # Air 热更新配置
├── go.mod
├── internal/
│   ├── scanner/           # 核心扫描引擎
│   │   ├── batch.go       # 分批处理逻辑
│   │   ├── context.go     # 上下文管理
│   │   └── scanner.go     # 扫描器实现
│   ├── gitlab/            # GitLab 集成
│   │   ├── client.go      # GitLab API 客户端
│   │   └── manager.go     # 仓库管理器
│   ├── github/            # GitHub 集成
│   │   ├── client.go      # GitHub API 客户端
│   │   └── manager.go     # 仓库管理器
│   └── models/            # 数据模型
│       └── types.go
├── api/                   # Web API
│   └── router.go
├── web/                   # 前端界面
│   └── static/
│       ├── index.html
│       ├── app.js
│       └── style.css
└── README.md
```

## 快速开始

### 1. 安装依赖

```bash
cd code-audit-claw
go mod tidy
```

### 2. 安装 air (热更新工具)

```bash
go install github.com/cosmtrek/air@latest
```

### 3. 运行服务

**使用 air 热更新（开发推荐）：**

```bash
air
```

**或使用 go run：**

```bash
# 使用默认端口 8080
go run main.go

# 或指定端口
go run main.go -addr :9090

# 指定 claude 命令路径
go run main.go -claude /path/to/claude
```

### 4. 访问 Web 界面

打开浏览器访问：http://localhost:8080

## API 接口

### 扫描管理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/scan` | 创建扫描 |
| GET | `/api/scan/:id` | 获取扫描详情 |
| GET | `/api/scan/:id/status` | 获取扫描状态（轻量级） |
| DELETE | `/api/scan/:id` | 删除扫描 |
| GET | `/api/scans` | 列出所有扫描 |

### 批次与问题

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/scan/:id/batches` | 获取批次列表 |
| GET | `/api/scan/:id/issues` | 获取问题列表 |
| GET | `/api/scan/:id/issues/severity/:severity` | 按严重程度筛选 |

### GitLab 集成

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/gitlab/validate` | 验证 GitLab Token |
| GET | `/api/gitlab/projects` | 列出 GitLab 项目 |
| GET | `/api/gitlab/projects/:id` | 获取项目详情 |
| GET | `/api/gitlab/projects/:id/branches` | 获取项目分支 |
| POST | `/api/gitlab/projects/:id/scan` | 扫描 GitLab 项目 |
| DELETE | `/api/gitlab/projects/:id/cache` | 清理项目缓存 |

#### GitLab Token 权限要求

创建 GitLab Personal Access Token 时需要以下权限：
- `api` - 完整 API 读写权限
- `read_api` - 读取 API 数据

#### GitLab 扫描请求示例

```json
{
  "token": "glpat-xxxxxxxxxxxxxxxxxxxx",
  "gitlab_url": "https://gitlab.com",
  "project_id": 123,
  "branch": "main",
  "scan_types": ["security", "quality", "secrets"],
  "batch_size": 5,
  "max_context": 100000
}
```

### GitHub 集成

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/github/validate` | 验证 GitHub Token |
| GET | `/api/github/repositories` | 列出用户仓库 |
| GET | `/api/github/search` | 搜索仓库 |
| GET | `/api/github/repos/:owner/:name` | 获取仓库详情 |
| GET | `/api/github/repos/:owner/:name/branches` | 获取分支列表 |
| POST | `/api/github/repos/:owner/:name/scan` | 扫描 GitHub 仓库 |
| DELETE | `/api/github/repos/:owner/:name/cache` | 清理仓库缓存 |

#### GitHub Token 权限要求

创建 GitHub Personal Access Token (Classic) 时需要以下权限：
- `repo` - 完整仓库访问权限
- `public_repo` - 公开仓库访问权限

#### GitHub 扫描请求示例

```json
{
  "token": "ghp_xxxxxxxxxxxxxxxxxxxx",
  "github_url": "https://github.com",
  "owner": "owner",
  "name": "repo",
  "branch": "main",
  "scan_types": ["security", "quality", "secrets"],
  "batch_size": 5,
  "max_context": 100000
}
```

### 创建扫描请求示例

```json
{
  "repo_path": "/path/to/your/repository",
  "branch": "main",
  "scan_types": ["security", "quality", "secrets"],
  "batch_size": 5,
  "max_context": 100000
}
```

## 分批扫描策略

### 上下文溢出处理

当遇到 API 上下文限制时，系统会：

1. **自动重试**：最多重试 3 次
2. **动态降级**：每次重试减少 25% 的 token 限制
3. **文件截断**：大文件自动截断到限制行数
4. **批次拆分**：超出限制的文件单独成批

### 批次创建策略

- 优先按文件数量分批（默认每批 5 个文件）
- 同时考虑 token 限制（默认 100000 tokens）
- 超大文件单独处理
- 自动排除 vendor、node_modules 等目录

## 扫描类型

| 类型 | 说明 |
|------|------|
| `security` | 安全漏洞（SQL注入、XSS、命令注入等） |
| `quality` | 代码质量（复杂度、重复代码等） |
| `secrets` | 敏感信息（密钥、凭证等） |
| `compliance` | 合规检查 |

## 问题严重程度

| 级别 | 说明 |
|------|------|
| `critical` | 严重 - 需要立即修复 |
| `high` | 高危 - 应尽快修复 |
| `medium` | 中危 - 建议修复 |
| `low` | 低危 - 可选修复 |
| `info` | 信息 - 仅供参考 |

## 配置选项

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | 服务监听地址 |
| `-claude` | `claude` | Claude CLI 命令路径 |

## 注意事项

1. **Claude CLI**：需要安装 Claude CLI 并确保可访问
2. **上下文限制**：根据 Claude API 的限制调整 `max_context` 参数
3. **并发控制**：默认最多 3 个批次并发扫描
4. **文件过滤**：自动排除常见依赖目录

## 开发

### 添加新的扫描类型

在 `internal/scanner/context.go` 的 `BuildSystemPrompt` 中添加新的提示词。

### 自定义批次策略

修改 `internal/scanner/batch.go` 中的 `CreateBatches` 方法。

### 扩展 API

在 `api/router.go` 中添加新的路由和处理函数。
