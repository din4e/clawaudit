'use client';

import React, { useEffect, useState, useCallback, useMemo } from 'react';
import { useAppSelector, useAppDispatch } from '@/hooks/redux';
import { selectSelectedScanId, fetchScans } from '@/store/slices/scansSlice';
import { useGetScanQuery, useDeleteScanMutation, useGetScanOutputQuery } from '@/store/services/api';
import { clearSelectedScan } from '@/store/slices/scansSlice';
import { useWebSocket } from '@/hooks/useWebSocket';
import { Progress, Badge, Button, Card, EmptyState, SummaryCard } from '@/components/ui';
import { RefreshCw, Trash2, CheckCircle2, AlertTriangle, AlertCircle, Info, Loader2, FileText, Activity, Download, Shield, Code, Zap } from 'lucide-react';
import { formatTime, getStatusText, isScanActive, API_BASE } from '@/lib/api';
import styles from './ScanDetail.module.css';

interface ProgressLog {
  id: string;
  type: 'progress' | 'batch_start' | 'batch_complete' | 'file_scan' | 'complete';
  message: string;
  timestamp: Date;
  data?: any;
}

export const ScanDetail = React.memo(function ScanDetail() {
  const dispatch = useAppDispatch();
  const scanId = useAppSelector(selectSelectedScanId);

  const { data: scan, isLoading, refetch } = useGetScanQuery(scanId || '', {
    skip: !scanId,
  });

  const [deleteScan, { isLoading: isDeleting }] = useDeleteScanMutation();
  const [progressLogs, setProgressLogs] = useState<ProgressLog[]>([]);
  const [currentBatch, setCurrentBatch] = useState<{ id: number; files: string[] } | null>(null);

  // Memoize summary to avoid recalculations
  const summary = useMemo(() => scan?.summary || {
    severity_critical: 0,
    severity_high: 0,
    severity_medium: 0,
    severity_low: 0,
    severity_info: 0,
  }, [scan?.summary]);

  // Stable callbacks
  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  const handleDelete = useCallback(async () => {
    if (!scanId) return;
    if (!confirm('确定要删除这条扫描记录吗？此操作不可恢复。')) {
      return;
    }
    try {
      await deleteScan(scanId).unwrap();
      dispatch(clearSelectedScan());
      dispatch(fetchScans({}));
    } catch (error) {
      alert('删除失败: ' + (error as Error).message);
    }
  }, [scanId, deleteScan, dispatch]);

  const handleDownloadJson = useCallback(async () => {
    if (!scanId) return;

    try {
      const response = await fetch(`${API_BASE.replace('/api', '')}/api/scan/${scanId}/output/file`);
      if (!response.ok) {
        throw new Error('下载失败');
      }

      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `scan-${scanId}-output.json`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error) {
      alert('下载 JSON 失败: ' + (error as Error).message);
    }
  }, [scanId]);

  // WebSocket connection for real-time updates
  useWebSocket(scanId || '', {
    onMessage: (message) => {
      const log: ProgressLog = {
        id: Date.now().toString() + Math.random(),
        type: message.type as any,
        message: message.data.message || '',
        timestamp: new Date(),
        data: message.data,
      };
      setProgressLogs(prev => [...prev.slice(-20), log]); // Keep last 20 logs
    },
    onProgress: (progress, message) => {
      // Progress is handled through the scan data from API
    },
    onBatchStart: (batchId, files) => {
      setCurrentBatch({ id: batchId, files });
    },
    onBatchComplete: (batchId, issuesFound) => {
      setCurrentBatch(null);
      // Refetch to get updated issue data
      refetch();
    },
    onComplete: () => {
      refetch();
    },
  });

  if (!scanId) {
    return (
      <div className={styles.welcome}>
        <div className={styles.welcomeIcon}>
          <CheckCircle2 size={40} />
        </div>
        <h2>欢迎使用 CodeAuditClaw</h2>
        <p>自动化代码安全审计工具，支持多种扫描类型</p>
        <div className={styles.features}>
          <div className={styles.feature}>
            <Shield size={16} />
            <span>安全扫描</span>
          </div>
          <div className={styles.feature}>
            <Code size={16} />
            <span>代码审计</span>
          </div>
          <div className={styles.feature}>
            <Zap size={16} />
            <span>快速分析</span>
          </div>
        </div>
      </div>
    );
  }

  if (isLoading && !scan) {
    return (
      <div className={styles.loading}>
        <Loader2 size={24} className={styles.spinner} />
        <p>加载中...</p>
      </div>
    );
  }

  if (!scan) {
    return (
      <EmptyState
        title="扫描不存在"
        description="未找到该扫描记录"
      />
    );
  }

  const summary = scan.summary || {
    severity_critical: 0,
    severity_high: 0,
    severity_medium: 0,
    severity_low: 0,
    severity_info: 0,
  };

  const severityIcons = {
    critical: <AlertCircle size={24} />,
    high: <AlertTriangle size={24} />,
    medium: <AlertTriangle size={24} />,
    low: <Info size={24} />,
    info: <Info size={24} />,
  };

  const isActive = isScanActive(scan.status);

  return (
    <div className={styles.detail}>
      <div className={styles.header}>
        <div className={styles.info}>
          <h2>{scan.repo_name || 'Unknown'}</h2>
          <div className={styles.meta}>
            <Badge variant={scan.status as any}>{getStatusText(scan.status)}</Badge>
            <span className={styles.time}>{formatTime(scan.started_at)}</span>
            {isActive && (
              <span className={styles.liveIndicator}>
                <span className={styles.liveDot}></span>
                实时更新中
              </span>
            )}
          </div>
        </div>
        <div className={styles.actions}>
          <Button variant="secondary" onClick={handleRefresh}>
            <RefreshCw size={16} />
            刷新
          </Button>
          {scan.status === 'completed' && (
            <Button variant="secondary" onClick={handleDownloadJson}>
              <Download size={16} />
              下载 JSON
            </Button>
          )}
          <Button variant="secondary" onClick={handleDelete} loading={isDeleting}>
            <Trash2 size={16} />
            删除
          </Button>
        </div>
      </div>

      {/* Progress Section with enhanced display */}
      {isActive && (
        <div className={styles.progressSection}>
          <div className={styles.progressHeader}>
            <div className={styles.progressInfo}>
              <Activity size={16} className={styles.progressIcon} />
              <span className={styles.progressLabel}>
                正在执行: {scan.message || getStatusText(scan.status)}
              </span>
            </div>
            <span className={styles.progressPercent}>
              {Math.round(scan.progress || 0)}%
            </span>
          </div>
          <Progress value={scan.progress || 0} />

          {/* Current batch info */}
          {currentBatch && (
            <div className={styles.currentBatch}>
              <FileText size={14} />
              <span>批次 {currentBatch.id + 1}: {currentBatch.files.length} 个文件</span>
            </div>
          )}

          {/* Progress logs */}
          {progressLogs.length > 0 && (
            <div className={styles.progressLogs}>
              {progressLogs.slice(-5).reverse().map((log) => (
                <div key={log.id} className={styles.logItem}>
                  <span className={styles.logTime}>
                    {log.timestamp.toLocaleTimeString()}
                  </span>
                  <span className={styles.logMessage}>
                    {log.message}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Summary Cards */}
      <div className={styles.summaryCards}>
        <SummaryCard
          severity="critical"
          value={summary.severity_critical}
          label="严重"
          icon={severityIcons.critical}
        />
        <SummaryCard
          severity="high"
          value={summary.severity_high}
          label="高危"
          icon={severityIcons.high}
        />
        <SummaryCard
          severity="medium"
          value={summary.severity_medium}
          label="中危"
          icon={severityIcons.medium}
        />
        <SummaryCard
          severity="low"
          value={summary.severity_low}
          label="低危"
          icon={severityIcons.low}
        />
      </div>

      {/* Batches */}
      {scan.batches && scan.batches.length > 0 && (
        <div className={styles.batchesSection}>
          <h3>扫描批次</h3>
          <div className={styles.batchesList}>
            {scan.batches.map((batch) => (
              <Card
                key={batch.batch_id}
                className={`${styles.batchItem} ${currentBatch?.id === batch.batch_id ? styles.batchActive : ''}`}
                padding="sm"
              >
                <div className={styles.batchHeader}>
                  <span className={styles.batchId}>批次 {batch.batch_id + 1}</span>
                  <span className={`${styles.batchStatus} ${styles[batch.status || 'pending']}`} />
                </div>
                <div className={styles.batchInfo}>
                  {batch.files?.length || 0} 个文件, {batch.issues?.length || 0} 个问题
                </div>
                {batch.status === 'scanning' && (
                  <div className={styles.batchProgress}>
                    <Loader2 size={12} className={styles.batchSpinner} />
                    <span>扫描中...</span>
                  </div>
                )}
              </Card>
            ))}
          </div>
        </div>
      )}

      {/* Issues */}
      {scan.batches && scan.batches.some(b => b.issues && b.issues.length > 0) && (
        <div className={styles.issuesSection}>
          <div className={styles.issuesHeader}>
            <h3>发现的问题</h3>
            <span className={styles.issuesCount}>
              {scan.batches.reduce((sum, b) => sum + (b.issues?.length || 0), 0)} 个问题
            </span>
          </div>
          <div className={styles.issuesList}>
            {scan.batches.flatMap(batch =>
              (batch.issues || []).map(issue => (
                <div key={issue.id} className={styles.issueItem}>
                  <div className={styles.issueHeader}>
                    <span className={styles.issueTitle}>{issue.title}</span>
                    <Badge variant="severity" severity={issue.severity as any}>
                      {issue.severity}
                    </Badge>
                  </div>
                  <div className={styles.issueLocation}>
                    {issue.file_path}:{issue.line_number}
                  </div>
                  <div className={styles.issueDescription}>{issue.description}</div>
                  {issue.code_snippet && (
                    <pre className={styles.issueCode}>{issue.code_snippet}</pre>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* No issues */}
      {!scan.batches || scan.batches.every(b => !b.issues || b.issues.length === 0) ? (
        scan.status === 'completed' && (
          <EmptyState
            title="未发现问题"
            description="扫描完成，未发现安全问题"
          />
        )
      ) : null}
    </div>
  );
});
