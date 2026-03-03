import React from 'react';
import styles from './Badge.module.css';

export interface BadgeProps {
  variant?: 'default' | 'pending' | 'scanning' | 'completed' | 'failed' | 'cloning' | 'cloned' | 'severity';
  severity?: 'critical' | 'high' | 'medium' | 'low' | 'info';
  children: React.ReactNode;
}

export function Badge({ variant = 'default', severity, children }: BadgeProps) {
  let className = styles.badge;

  if (variant === 'severity' && severity) {
    className += ` ${styles[severity]}`;
  } else if (variant !== 'default') {
    className += ` ${styles[variant]}`;
  }

  return <span className={className}>{children}</span>;
}
