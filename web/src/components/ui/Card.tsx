import React from 'react';
import styles from './Card.module.css';

export interface CardProps {
  variant?: 'default' | 'bordered' | 'elevated';
  padding?: 'none' | 'sm' | 'md' | 'lg';
  className?: string;
  children: React.ReactNode;
  onClick?: () => void;
}

export function Card({ variant = 'default', padding = 'md', className, children, onClick }: CardProps) {
  return (
    <div
      className={`${styles.card} ${styles[variant]} ${styles[padding]} ${onClick ? styles.clickable : ''} ${className || ''}`}
      onClick={onClick}
    >
      {children}
    </div>
  );
}

export interface SummaryCardProps {
  severity: 'critical' | 'high' | 'medium' | 'low';
  value: number;
  label: string;
  icon?: React.ReactNode;
}

export function SummaryCard({ severity, value, label, icon }: SummaryCardProps) {
  return (
    <Card className={`${styles.summary} ${styles[severity]}`}>
      <div className={styles.icon}>{icon}</div>
      <div className={styles.content}>
        <span className={styles.value}>{value}</span>
        <span className={styles.label}>{label}</span>
      </div>
    </Card>
  );
}
