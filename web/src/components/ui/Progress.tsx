import React from 'react';
import styles from './Progress.module.css';

export interface ProgressProps {
  value: number;
  max?: number;
  size?: 'sm' | 'md' | 'lg';
  showLabel?: boolean;
  label?: string;
  className?: string;
}

export function Progress({ value = 0, max = 100, size = 'md', showLabel = true, label, className }: ProgressProps) {
  const percentage = Math.min(Math.max((value / max) * 100, 0), 100);

  return (
    <div className={`${styles.progress} ${styles[size]} ${className || ''}`}>
      <div className={styles.bar}>
        <div className={styles.fill} style={{ width: `${percentage}%` }} />
      </div>
      {showLabel && (
        <div className={styles.label}>
          <span>{label || 'Progress'}</span>
          <span>{percentage.toFixed(0)}%</span>
        </div>
      )}
    </div>
  );
}
