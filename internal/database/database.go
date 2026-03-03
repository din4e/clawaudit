package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DBType 数据库类型
type DBType string

const (
	DBTypeSQLite DBType = "sqlite"
	DBTypeMySQL  DBType = "mysql"
)

// Config 数据库配置
type Config struct {
	Type     DBType
	SQLitePath string // SQLite 数据库文件路径
	MySQLHost   string // MySQL 主机
	MySQLPort   int    // MySQL 端口
	MySQLUser   string // MySQL 用户名
	MySQLPass   string // MySQL 密码
	MySQLDBName string // MySQL 数据库名
}

// DefaultConfig 返回默认配置（SQLite）
func DefaultConfig() *Config {
	// 获取当前目录
	dbDir := "./data"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		dbDir = "."
	}
	return &Config{
		Type:       DBTypeSQLite,
		SQLitePath: filepath.Join(dbDir, "auditor.db"),
	}
}

// DB 数据库封装
type DB struct {
	*sql.DB
	Type DBType
}

// Open 打开数据库连接
func Open(cfg *Config) (*DB, error) {
	var dataSourceName string
	var driverName string

	switch cfg.Type {
	case DBTypeSQLite:
		driverName = "sqlite3"
		dataSourceName = cfg.SQLitePath
	case DBTypeMySQL:
		driverName = "mysql"
		dataSourceName = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			cfg.MySQLUser,
			cfg.MySQLPass,
			cfg.MySQLHost,
			cfg.MySQLPort,
			cfg.MySQLDBName,
		)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db, Type: cfg.Type}, nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.DB.Close()
}

// InitSchema 初始化数据库表结构
func (db *DB) InitSchema() error {
	schema := getSchema(db.Type)
	_, err := db.Exec(schema)
	return err
}

// getSchema 根据数据库类型返回建表语句
func getSchema(dbType DBType) string {
	if dbType == DBTypeMySQL {
		return getMySQLSchema()
	}
	return getSQLiteSchema()
}

// getSQLiteSchema 返回 SQLite 建表语句
func getSQLiteSchema() string {
	return `
-- 扫描记录表
CREATE TABLE IF NOT EXISTS scans (
    id TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    branch TEXT,
    status TEXT NOT NULL,
    scan_types TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    total_files INTEGER DEFAULT 0,
    total_batches INTEGER DEFAULT 0,
    completed_batches INTEGER DEFAULT 0,
    total_issues INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 批次记录表
CREATE TABLE IF NOT EXISTS batches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL,
    batch_id INTEGER NOT NULL,
    files TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    tokens_used INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE,
    UNIQUE(scan_id, batch_id)
);

-- 问题记录表
CREATE TABLE IF NOT EXISTS issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL,
    batch_id INTEGER,
    issue_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    line_number INTEGER,
    column_number INTEGER,
    severity TEXT NOT NULL,
    scan_type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    code_snippet TEXT,
    rule_id TEXT,
    cwe TEXT,
    "references" TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE
);

-- 扫描摘要统计表
CREATE TABLE IF NOT EXISTS scan_summaries (
    scan_id TEXT PRIMARY KEY,
    severity_critical INTEGER DEFAULT 0,
    severity_high INTEGER DEFAULT 0,
    severity_medium INTEGER DEFAULT 0,
    severity_low INTEGER DEFAULT 0,
    severity_info INTEGER DEFAULT 0,
    type_security INTEGER DEFAULT 0,
    type_quality INTEGER DEFAULT 0,
    type_secrets INTEGER DEFAULT 0,
    type_compliance INTEGER DEFAULT 0,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE
);

-- 仓库记录表
CREATE TABLE IF NOT EXISTS repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    branch TEXT,
    last_scanned DATETIME,
    scan_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status);
CREATE INDEX IF NOT EXISTS idx_scans_started_at ON scans(started_at);
CREATE INDEX IF NOT EXISTS idx_batches_scan_id ON batches(scan_id);
CREATE INDEX IF NOT EXISTS idx_issues_scan_id ON issues(scan_id);
CREATE INDEX IF NOT EXISTS idx_issues_severity ON issues(severity);
CREATE INDEX IF NOT EXISTS idx_issues_scan_type ON issues(scan_type);
CREATE INDEX IF NOT EXISTS idx_repositories_path ON repositories(path);
`
}

// getMySQLSchema 返回 MySQL 建表语句
func getMySQLSchema() string {
	return `
-- 扫描记录表
CREATE TABLE IF NOT EXISTS scans (
    id VARCHAR(36) PRIMARY KEY,
    repo_path VARCHAR(512) NOT NULL,
    repo_name VARCHAR(256) NOT NULL,
    branch VARCHAR(128),
    status VARCHAR(32) NOT NULL,
    scan_types VARCHAR(256) NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    total_files INT DEFAULT 0,
    total_batches INT DEFAULT 0,
    completed_batches INT DEFAULT 0,
    total_issues INT DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status (status),
    INDEX idx_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 批次记录表
CREATE TABLE IF NOT EXISTS batches (
    id INT AUTO_INCREMENT PRIMARY KEY,
    scan_id VARCHAR(36) NOT NULL,
    batch_id INT NOT NULL,
    files TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    tokens_used INT DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE,
    UNIQUE KEY uk_scan_batch (scan_id, batch_id),
    INDEX idx_scan_id (scan_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 问题记录表
CREATE TABLE IF NOT EXISTS issues (
    id INT AUTO_INCREMENT PRIMARY KEY,
    scan_id VARCHAR(36) NOT NULL,
    batch_id INT,
    issue_id VARCHAR(64) NOT NULL,
    file_path VARCHAR(512) NOT NULL,
    line_number INT,
    column_number INT,
    severity VARCHAR(32) NOT NULL,
    scan_type VARCHAR(32) NOT NULL,
    title VARCHAR(512) NOT NULL,
    description TEXT,
    code_snippet TEXT,
    rule_id VARCHAR(128),
    cwe VARCHAR(64),
    references TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE,
    INDEX idx_scan_id (scan_id),
    INDEX idx_severity (severity),
    INDEX idx_scan_type (scan_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 扫描摘要统计表
CREATE TABLE IF NOT EXISTS scan_summaries (
    scan_id VARCHAR(36) PRIMARY KEY,
    severity_critical INT DEFAULT 0,
    severity_high INT DEFAULT 0,
    severity_medium INT DEFAULT 0,
    severity_low INT DEFAULT 0,
    severity_info INT DEFAULT 0,
    type_security INT DEFAULT 0,
    type_quality INT DEFAULT 0,
    type_secrets INT DEFAULT 0,
    type_compliance INT DEFAULT 0,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 仓库记录表
CREATE TABLE IF NOT EXISTS repositories (
    id INT AUTO_INCREMENT PRIMARY KEY,
    path VARCHAR(512) UNIQUE NOT NULL,
    name VARCHAR(256) NOT NULL,
    branch VARCHAR(128),
    last_scanned DATETIME,
    scan_count INT DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_path (path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`
}
