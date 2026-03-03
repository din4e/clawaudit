package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository 数据访问层
type Repository struct {
	db *DB
}

// NewRepository 创建 Repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// ==================== Scans 操作 ====================

// CreateScan 创建扫描记录
func (r *Repository) CreateScan(ctx context.Context, scan *ScanDB) error {
	query := `
		INSERT INTO scans (
			id, repo_path, repo_name, branch, status, scan_types,
			started_at, total_files, total_batches, completed_batches,
			total_issues, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		scan.ID, scan.RepoPath, scan.RepoName, scan.Branch, scan.Status,
		scan.ScanTypes, scan.StartedAt, scan.TotalFiles, scan.TotalBatches,
		scan.CompletedBatches, scan.TotalIssues, scan.ErrorMessage,
	)
	return err
}

// GetScan 获取扫描记录
func (r *Repository) GetScan(ctx context.Context, id string) (*ScanDB, error) {
	query := `
		SELECT id, repo_path, repo_name, branch, status, scan_types,
			   started_at, completed_at, total_files, total_batches,
			   completed_batches, total_issues, error_message,
			   created_at, updated_at
		FROM scans WHERE id = ?
	`
	scan := &ScanDB{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&scan.ID, &scan.RepoPath, &scan.RepoName, &scan.Branch,
		&scan.Status, &scan.ScanTypes, &scan.StartedAt, &scan.CompletedAt,
		&scan.TotalFiles, &scan.TotalBatches, &scan.CompletedBatches,
		&scan.TotalIssues, &scan.ErrorMessage, &scan.CreatedAt, &scan.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return scan, err
}

// UpdateScan 更新扫描记录
func (r *Repository) UpdateScan(ctx context.Context, scan *ScanDB) error {
	query := `
		UPDATE scans SET
			repo_path = ?, repo_name = ?, branch = ?, status = ?,
			scan_types = ?, started_at = ?, completed_at = ?,
			total_files = ?, total_batches = ?, completed_batches = ?,
			total_issues = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		scan.RepoPath, scan.RepoName, scan.Branch, scan.Status,
		scan.ScanTypes, scan.StartedAt, scan.CompletedAt,
		scan.TotalFiles, scan.TotalBatches, scan.CompletedBatches,
		scan.TotalIssues, scan.ErrorMessage, scan.ID,
	)
	return err
}

// UpdateScanStatus 更新扫描状态
func (r *Repository) UpdateScanStatus(ctx context.Context, id string, status string, errorMsg string) error {
	query := `
		UPDATE scans SET
			status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, status, errorMsg, id)
	return err
}

// UpdateScanProgress 更新扫描进度
func (r *Repository) UpdateScanProgress(ctx context.Context, id string, completedBatches, totalIssues int) error {
	query := `
		UPDATE scans SET
			completed_batches = ?, total_issues = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, completedBatches, totalIssues, id)
	return err
}

// CompleteScan 完成扫描
func (r *Repository) CompleteScan(ctx context.Context, id string, completedAt time.Time, totalIssues int) error {
	query := `
		UPDATE scans SET
			status = 'completed', completed_at = ?,
			total_issues = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, completedAt, totalIssues, id)
	return err
}

// DeleteScan 删除扫描记录（级联删除批次和问题）
func (r *Repository) DeleteScan(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM scans WHERE id = ?", id)
	return err
}

// ListScans 列出扫描记录
func (r *Repository) ListScans(ctx context.Context, filter *ListScansFilter) ([]*ScanDB, int, error) {
	// 构建查询条件
	whereClause := ""
	args := []interface{}{}
	if filter.Status != "" {
		whereClause = " WHERE status = ?"
		args = append(args, filter.Status)
	}

	// 获取总数
	countQuery := "SELECT COUNT(*) FROM scans" + whereClause
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据列表
	query := `
		SELECT id, repo_path, repo_name, branch, status, scan_types,
			   started_at, completed_at, total_files, total_batches,
			   completed_batches, total_issues, error_message,
			   created_at, updated_at
		FROM scans
	` + whereClause + `
		ORDER BY ` + filter.BuildOrderBy() + `
		LIMIT ? OFFSET ?
	`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	scans := []*ScanDB{}
	for rows.Next() {
		scan := &ScanDB{}
		err := rows.Scan(
			&scan.ID, &scan.RepoPath, &scan.RepoName, &scan.Branch,
			&scan.Status, &scan.ScanTypes, &scan.StartedAt, &scan.CompletedAt,
			&scan.TotalFiles, &scan.TotalBatches, &scan.CompletedBatches,
			&scan.TotalIssues, &scan.ErrorMessage, &scan.CreatedAt, &scan.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		scans = append(scans, scan)
	}

	return scans, total, nil
}

// ==================== Batches 操作 ====================

// CreateBatch 创建批次记录
func (r *Repository) CreateBatch(ctx context.Context, batch *BatchDB) error {
	query := `
		INSERT INTO batches (
			scan_id, batch_id, files, status, started_at, tokens_used, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		batch.ScanID, batch.BatchID, batch.Files, batch.Status,
		batch.StartedAt, batch.TokensUsed, batch.ErrorMessage,
	)
	return err
}

// GetBatch 获取批次记录
func (r *Repository) GetBatch(ctx context.Context, scanID string, batchID int) (*BatchDB, error) {
	query := `
		SELECT id, scan_id, batch_id, files, status, started_at,
			   completed_at, tokens_used, error_message, created_at
		FROM batches WHERE scan_id = ? AND batch_id = ?
	`
	batch := &BatchDB{}
	err := r.db.QueryRowContext(ctx, query, scanID, batchID).Scan(
		&batch.ID, &batch.ScanID, &batch.BatchID, &batch.Files,
		&batch.Status, &batch.StartedAt, &batch.CompletedAt,
		&batch.TokensUsed, &batch.ErrorMessage, &batch.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return batch, err
}

// ListBatches 列出扫描的所有批次
func (r *Repository) ListBatches(ctx context.Context, scanID string) ([]*BatchDB, error) {
	query := `
		SELECT id, scan_id, batch_id, files, status, started_at,
			   completed_at, tokens_used, error_message, created_at
		FROM batches WHERE scan_id = ? ORDER BY batch_id
	`
	rows, err := r.db.QueryContext(ctx, query, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batches := []*BatchDB{}
	for rows.Next() {
		batch := &BatchDB{}
		err := rows.Scan(
			&batch.ID, &batch.ScanID, &batch.BatchID, &batch.Files,
			&batch.Status, &batch.StartedAt, &batch.CompletedAt,
			&batch.TokensUsed, &batch.ErrorMessage, &batch.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}

	return batches, nil
}

// UpdateBatch 更新批次记录
func (r *Repository) UpdateBatch(ctx context.Context, batch *BatchDB) error {
	query := `
		UPDATE batches SET
			files = ?, status = ?, started_at = ?, completed_at = ?,
			tokens_used = ?, error_message = ?
		WHERE scan_id = ? AND batch_id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		batch.Files, batch.Status, batch.StartedAt, batch.CompletedAt,
		batch.TokensUsed, batch.ErrorMessage, batch.ScanID, batch.BatchID,
	)
	return err
}

// CompleteBatch 完成批次
func (r *Repository) CompleteBatch(ctx context.Context, scanID string, batchID int, completedAt time.Time, tokensUsed int) error {
	query := `
		UPDATE batches SET
			status = 'completed', completed_at = ?, tokens_used = ?
		WHERE scan_id = ? AND batch_id = ?
	`
	_, err := r.db.ExecContext(ctx, query, completedAt, tokensUsed, scanID, batchID)
	return err
}

// ==================== Issues 操作 ====================

// CreateIssue 创建问题记录
func (r *Repository) CreateIssue(ctx context.Context, issue *IssueDB) error {
	query := `
		INSERT INTO issues (
			scan_id, batch_id, issue_id, file_path, line_number, column_number,
			severity, scan_type, title, description, code_snippet,
			rule_id, cwe, references
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		issue.ScanID, issue.BatchID, issue.IssueID, issue.FilePath,
		issue.LineNumber, issue.ColumnNumber, issue.Severity, issue.ScanType,
		issue.Title, issue.Description, issue.CodeSnippet,
		issue.RuleID, issue.CWE, issue.References,
	)
	return err
}

// CreateIssues 批量创建问题记录
func (r *Repository) CreateIssues(ctx context.Context, issues []*IssueDB) error {
	if len(issues) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO issues (
			scan_id, batch_id, issue_id, file_path, line_number, column_number,
			severity, scan_type, title, description, code_snippet,
			rule_id, cwe, references
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, issue := range issues {
		_, err := stmt.Exec(
			issue.ScanID, issue.BatchID, issue.IssueID, issue.FilePath,
			issue.LineNumber, issue.ColumnNumber, issue.Severity, issue.ScanType,
			issue.Title, issue.Description, issue.CodeSnippet,
			issue.RuleID, issue.CWE, issue.References,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListIssues 列出问题
func (r *Repository) ListIssues(ctx context.Context, filter *ListIssuesFilter) ([]*IssueDB, int, error) {
	// 构建查询条件
	whereClause := " WHERE 1=1"
	args := []interface{}{}

	if filter.ScanID != "" {
		whereClause += " AND scan_id = ?"
		args = append(args, filter.ScanID)
	}
	if filter.BatchID > 0 {
		whereClause += " AND batch_id = ?"
		args = append(args, filter.BatchID)
	}
	if filter.Severity != "" {
		whereClause += " AND severity = ?"
		args = append(args, filter.Severity)
	}
	if filter.ScanType != "" {
		whereClause += " AND scan_type = ?"
		args = append(args, filter.ScanType)
	}

	// 获取总数
	countQuery := "SELECT COUNT(*) FROM issues" + whereClause
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据列表
	query := `
		SELECT id, scan_id, batch_id, issue_id, file_path, line_number,
			   column_number, severity, scan_type, title, description,
			   code_snippet, rule_id, cwe, references, created_at
		FROM issues
	` + whereClause + `
		ORDER BY id DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	issues := []*IssueDB{}
	for rows.Next() {
		issue := &IssueDB{}
		err := rows.Scan(
			&issue.ID, &issue.ScanID, &issue.BatchID, &issue.IssueID,
			&issue.FilePath, &issue.LineNumber, &issue.ColumnNumber,
			&issue.Severity, &issue.ScanType, &issue.Title,
			&issue.Description, &issue.CodeSnippet, &issue.RuleID,
			&issue.CWE, &issue.References, &issue.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		issues = append(issues, issue)
	}

	return issues, total, nil
}

// GetIssuesBySeverity 按严重程度获取问题
func (r *Repository) GetIssuesBySeverity(ctx context.Context, scanID string, severity string) ([]*IssueDB, error) {
	query := `
		SELECT id, scan_id, batch_id, issue_id, file_path, line_number,
			   column_number, severity, scan_type, title, description,
			   code_snippet, rule_id, cwe, references, created_at
		FROM issues WHERE scan_id = ? AND severity = ?
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query, scanID, severity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issues := []*IssueDB{}
	for rows.Next() {
		issue := &IssueDB{}
		err := rows.Scan(
			&issue.ID, &issue.ScanID, &issue.BatchID, &issue.IssueID,
			&issue.FilePath, &issue.LineNumber, &issue.ColumnNumber,
			&issue.Severity, &issue.ScanType, &issue.Title,
			&issue.Description, &issue.CodeSnippet, &issue.RuleID,
			&issue.CWE, &issue.References, &issue.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}

	return issues, nil
}

// ==================== Summaries 操作 ====================

// CreateSummary 创建扫描摘要
func (r *Repository) CreateSummary(ctx context.Context, summary *ScanSummaryDB) error {
	query := `
		INSERT INTO scan_summaries (
			scan_id, severity_critical, severity_high, severity_medium,
			severity_low, severity_info, type_security, type_quality,
			type_secrets, type_compliance
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		summary.ScanID, summary.SeverityCritical, summary.SeverityHigh,
		summary.SeverityMedium, summary.SeverityLow, summary.SeverityInfo,
		summary.TypeSecurity, summary.TypeQuality, summary.TypeSecrets,
		summary.TypeCompliance,
	)
	return err
}

// GetSummary 获取扫描摘要
func (r *Repository) GetSummary(ctx context.Context, scanID string) (*ScanSummaryDB, error) {
	query := `
		SELECT scan_id, severity_critical, severity_high, severity_medium,
			   severity_low, severity_info, type_security, type_quality,
			   type_secrets, type_compliance
		FROM scan_summaries WHERE scan_id = ?
	`
	summary := &ScanSummaryDB{}
	err := r.db.QueryRowContext(ctx, query, scanID).Scan(
		&summary.ScanID, &summary.SeverityCritical, &summary.SeverityHigh,
		&summary.SeverityMedium, &summary.SeverityLow, &summary.SeverityInfo,
		&summary.TypeSecurity, &summary.TypeQuality, &summary.TypeSecrets,
		&summary.TypeCompliance,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return summary, err
}

// ==================== Repositories 操作 ====================

// CreateRepository 创建仓库记录
func (r *Repository) CreateRepository(ctx context.Context, repo *RepositoryDB) error {
	query := `
		INSERT INTO repositories (path, name, branch, last_scanned, scan_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			name = excluded.name,
			branch = excluded.branch,
			last_scanned = excluded.last_scanned,
			scan_count = repositories.scan_count + excluded.scan_count,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, query,
		repo.Path, repo.Name, repo.Branch, repo.LastScanned, repo.ScanCount,
	)
	return err
}

// GetRepository 获取仓库记录
func (r *Repository) GetRepository(ctx context.Context, path string) (*RepositoryDB, error) {
	query := `
		SELECT id, path, name, branch, last_scanned, scan_count, created_at, updated_at
		FROM repositories WHERE path = ?
	`
	repo := &RepositoryDB{}
	err := r.db.QueryRowContext(ctx, query, path).Scan(
		&repo.ID, &repo.Path, &repo.Name, &repo.Branch,
		&repo.LastScanned, &repo.ScanCount, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return repo, err
}

// ListRepositories 列出仓库记录
func (r *Repository) ListRepositories(ctx context.Context, limit, offset int) ([]*RepositoryDB, int, error) {
	// 获取总数
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repositories").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据列表
	query := `
		SELECT id, path, name, branch, last_scanned, scan_count, created_at, updated_at
		FROM repositories
		ORDER BY last_scanned DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	repos := []*RepositoryDB{}
	for rows.Next() {
		repo := &RepositoryDB{}
		err := rows.Scan(
			&repo.ID, &repo.Path, &repo.Name, &repo.Branch,
			&repo.LastScanned, &repo.ScanCount, &repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		repos = append(repos, repo)
	}

	return repos, total, nil
}

// UpdateRepositoryScan 更新仓库扫描记录
func (r *Repository) UpdateRepositoryScan(ctx context.Context, path string, lastScanned time.Time) error {
	query := `
		UPDATE repositories SET
			last_scanned = ?, scan_count = scan_count + 1, updated_at = CURRENT_TIMESTAMP
		WHERE path = ?
	`
	_, err := r.db.ExecContext(ctx, query, lastScanned, path)
	return err
}

// ==================== Stats 操作 ====================

// GetStats 获取统计信息
func (r *Repository) GetStats(ctx context.Context) (*StatsResult, error) {
	stats := &StatsResult{}

	// 扫描统计
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans").Scan(&stats.TotalScans)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans WHERE status = 'completed'").Scan(&stats.CompletedScans)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans WHERE status = 'failed'").Scan(&stats.FailedScans)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans WHERE status IN ('pending', 'scanning', 'cloning')").Scan(&stats.RunningScans)

	// 问题统计
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM issues").Scan(&stats.TotalIssues)

	// 仓库统计
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repositories").Scan(&stats.TotalRepos)

	return stats, nil
}

// ==================== 事务支持 ====================

// WithTx 在事务中执行操作
func (r *Repository) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// CreateScanWithBatchesAndIssues 创建扫描记录及其关联数据
func (r *Repository) CreateScanWithBatchesAndIssues(
	ctx context.Context,
	scan *ScanDB,
	batches []*BatchDB,
	issues []*IssueDB,
	summary *ScanSummaryDB,
) error {
	return r.WithTx(ctx, func(tx *sql.Tx) error {
		// 创建扫描记录
		if err := r.CreateScan(ctx, scan); err != nil {
			return fmt.Errorf("create scan: %w", err)
		}

		// 创建批次
		for _, batch := range batches {
			if _, err := tx.Exec(`
				INSERT INTO batches (scan_id, batch_id, files, status, started_at, tokens_used, error_message)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, batch.ScanID, batch.BatchID, batch.Files, batch.Status,
				batch.StartedAt, batch.TokensUsed, batch.ErrorMessage); err != nil {
				return fmt.Errorf("create batch: %w", err)
			}
		}

		// 创建问题
		for _, issue := range issues {
			if _, err := tx.Exec(`
				INSERT INTO issues (
					scan_id, batch_id, issue_id, file_path, line_number, column_number,
					severity, scan_type, title, description, code_snippet,
					rule_id, cwe, references
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, issue.ScanID, issue.BatchID, issue.IssueID, issue.FilePath,
				issue.LineNumber, issue.ColumnNumber, issue.Severity, issue.ScanType,
				issue.Title, issue.Description, issue.CodeSnippet,
				issue.RuleID, issue.CWE, issue.References); err != nil {
				return fmt.Errorf("create issue: %w", err)
			}
		}

		// 创建摘要
		if summary != nil {
			if _, err := tx.Exec(`
				INSERT INTO scan_summaries (
					scan_id, severity_critical, severity_high, severity_medium,
					severity_low, severity_info, type_security, type_quality,
					type_secrets, type_compliance
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, summary.ScanID, summary.SeverityCritical, summary.SeverityHigh,
				summary.SeverityMedium, summary.SeverityLow, summary.SeverityInfo,
				summary.TypeSecurity, summary.TypeQuality, summary.TypeSecrets,
				summary.TypeCompliance); err != nil {
				return fmt.Errorf("create summary: %w", err)
			}
		}

		return nil
	})
}
