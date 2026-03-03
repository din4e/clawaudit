# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**CodeAuditClaw** is an automated code security audit tool that:
- Scans code repositories for security vulnerabilities (XSS, SSTI, RCE, SQLi, SSRF, etc.)
- Splits large repositories into batches to avoid API context limits
- Provides both CLI and Web UI interfaces
- Integrates with GitLab/GitHub for remote repository scanning

## Commands

### Development
```bash
# Hot reload (recommended for development)
air

# Run server directly
go run main.go
go run main.go -addr :9090

# Build
go build -o ./tmp/main.exe .
```

### CLI Mode (Scan directly)
```bash
# Basic scan
go run main.go scan /path/to/project

# With custom parameters
go run main.go scan /path/to/project --batch 10 --tokens 200000 --branch main
```

### Frontend (Next.js)
```bash
cd web
npm install
npm run dev
```

## Architecture

### Core Components

**`internal/scanner/`** - Scanning engine
- `scanner.go` - Main orchestration, batch concurrency, Claude CLI invocation
- `batch.go` - File batching strategy (by count and token limit)
- `context.go` - Context window management, overflow handling, system prompt generation

**`internal/database/`** - Persistence layer
- Supports SQLite (default) and MySQL
- Schema: scans → batches → issues hierarchy
- `Repository` pattern for data access

**`internal/gitlab/` & `internal/github/`** - Remote repository integration
- API clients for fetching repository metadata
- Branch listing, cloning, cache management

**`internal/sandbox/`** - Isolated execution environment
- Clones repos to `repo/{uuid}/` for safe scanning
- Platform-specific (sandbox_windows.go, sandbox_unix.go)

**`api/router.go`** - REST API + WebSocket server
- WebSocket for real-time scan progress updates
- REST endpoints for scan management and Git integration

### Data Flow

1. User creates scan (Web UI or CLI)
2. `Scanner.Scan()` lists code files, creates batches
3. Batches scanned concurrently (max 3 parallel)
4. Each batch: `ContextManager.BuildContext()` → calls Claude CLI → parses JSON response
5. Progress emitted via callback → WebSocket push to frontend
6. Results persisted to database

### Batch Strategy

Files are grouped by:
- Primary: batch size (default 5 files per batch)
- Secondary: token limit (default 100,000 tokens)
- Overflow handling: truncate large files, retry with 25% reduction on context errors

Excluded directories: `vendor`, `node_modules`, `.git`, `dist`, `build`, `target`, `.idea`, `.vscode`, `coverage`, `__pycache__`

### Claude Integration

The tool calls Claude CLI via stdin with a JSON-formatted prompt expecting:
```json
{
  "project_analysis": "...",
  "issues": [{
    "id": "...",
    "title_cn": "...",
    "title_en": "...",
    "severity": "critical|high|medium|low|info",
    "type": "XSS|SSTI|RCE|SQLi|SSRF|Other",
    "file": "...",
    "line": 123,
    "code_snippet": "...",
    "description": "...",
    "introduction_cn": "...",
    "introduction_en": "...",
    "affected_versions": "...",
    "analysis_detail": "...",
    "poc": "...",
    "poc_verification": "..."
  }],
  "usage": {"input_tokens": 0, "output_tokens": 0}
}
```

## Database

Default SQLite database at `./data/auditor.db`.

Tables:
- `scans` - Scan records with status, timing, summary
- `batches` - Per-batch results with token usage
- `issues` - Individual vulnerability findings
- `scan_summaries` - Aggregated statistics by severity/type
- `repositories` - Repository metadata and scan history

## Frontend

Next.js with Redux Toolkit, located in `web/`.
- Components in `web/src/components/`
- Redux slices in `web/src/store/slices/`
- API types in `web/src/types/api.ts`

Build output excluded by Air hot reload config.

## Key Patterns

- **Progress Callbacks**: Scanner accepts generic callbacks via `SetProgressCallback(scanID, func(interface{}))`, converted to internal `ProgressUpdate` type
- **Adapter Pattern**: `scannerAdapter` in main.go bridges `scanner.Scanner` to `api.ScannerInterface`
- **Stale Scan Cleanup**: On server start, pending scans older than 5 minutes are marked failed
