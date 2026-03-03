'use client';

import React, { useEffect, useRef, useMemo, useCallback } from 'react';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectScan, fetchScans, selectAllScans, selectSelectedScanId } from '@/store/slices/scansSlice';
import { Badge } from '@/components/ui';
import { formatTime, getStatusText, isScanActive } from '@/lib/api';
import { RefreshCw } from 'lucide-react';
import styles from './Sidebar.module.css';

export const Sidebar = React.memo(function Sidebar() {
  const dispatch = useAppDispatch();
  const scans = useAppSelector(selectAllScans);
  const selectedScanId = useAppSelector(selectSelectedScanId);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Derive active state once, not in effect dependencies
  const hasActiveScans = useMemo(() =>
    scans.some(scan => isScanActive(scan.status)),
    [scans]
  );

  // Stable callback functions
  const handleRefresh = useCallback(() => {
    dispatch(fetchScans({}));
  }, [dispatch]);

  const handleSelectScan = useCallback((scanId: string) => {
    dispatch(selectScan(scanId));
  }, [dispatch]);

  // Initial fetch
  useEffect(() => {
    handleRefresh();
  }, [handleRefresh]);

  // Poll for active scans - only depends on derived boolean
  useEffect(() => {
    if (!hasActiveScans) return;

    intervalRef.current = setInterval(() => {
      dispatch(fetchScans({}));
    }, 3000);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [hasActiveScans, dispatch]);

  return (
    <aside className={styles.sidebar}>
      <div className={styles.header}>
        <h2>已扫描仓库</h2>
        <div className={styles.stats}>
          <button
            className={styles.refreshBtn}
            onClick={handleRefresh}
            title="刷新列表"
          >
            <RefreshCw size={14} />
          </button>
          <span className={styles.statItem}>{scans.length}</span>
        </div>
      </div>
      <div className={styles.list}>
        {scans.length === 0 ? (
          <div className={styles.empty}>
            <p>暂无扫描记录</p>
          </div>
        ) : (
          scans.map((scan) => (
            <div
              key={scan.id}
              className={`${styles.item} ${scan.id === selectedScanId ? styles.active : ''}`}
              onClick={() => handleSelectScan(scan.id)}
            >
              <div className={styles.name}>{scan.repo_name || 'Unknown'}</div>
              <div className={styles.path}>{scan.repo_path || ''}</div>
              <div className={styles.meta}>
                <Badge variant={scan.status as any}>{getStatusText(scan.status)}</Badge>
                <span className={styles.time}>{formatTime(scan.started_at)}</span>
              </div>
            </div>
          ))
        )}
      </div>
    </aside>
  );
}
