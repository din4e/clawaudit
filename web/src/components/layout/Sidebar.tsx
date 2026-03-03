'use client';

import React, { useEffect, useRef } from 'react';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectScan, fetchScans, selectAllScans, selectSelectedScanId } from '@/store/slices/scansSlice';
import { Badge } from '@/components/ui';
import { formatTime, getStatusText, isScanActive } from '@/lib/api';
import { RefreshCw } from 'lucide-react';
import styles from './Sidebar.module.css';

export function Sidebar() {
  const dispatch = useAppDispatch();
  const scans = useAppSelector(selectAllScans);
  const selectedScanId = useAppSelector(selectSelectedScanId);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Initial fetch
  useEffect(() => {
    dispatch(fetchScans({}));
  }, [dispatch]);

  // Poll for active scans
  useEffect(() => {
    const hasActiveScans = scans.some(scan => isScanActive(scan.status));

    if (hasActiveScans) {
      // Poll every 3 seconds when there are active scans
      intervalRef.current = setInterval(() => {
        dispatch(fetchScans({}));
      }, 3000);
    } else {
      // Clear interval when no active scans
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [scans, dispatch]);

  const handleRefresh = () => {
    dispatch(fetchScans({}));
  };

  const handleSelectScan = (scanId: string) => {
    dispatch(selectScan(scanId));
  };

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
