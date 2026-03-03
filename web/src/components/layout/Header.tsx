'use client';

import React from 'react';
import { Moon, Sun, Link2, Github, Gitlab as GitLabIcon, FolderOpen } from 'lucide-react';
import { useAppDispatch } from '@/hooks/redux';
import { openModal } from '@/store/slices/uiSlice';
import { useTheme } from '@/hooks/useTheme';
import { Button } from '@/components/ui';
import styles from './Header.module.css';

export function Header() {
  const dispatch = useAppDispatch();
  const { theme, toggleTheme } = useTheme();

  const handleOpenUrlModal = () => {
    dispatch(openModal({ modal: 'url' }));
  };

  const handleOpenGitHubModal = () => {
    dispatch(openModal({ modal: 'github' }));
  };

  const handleOpenGitLabModal = () => {
    dispatch(openModal({ modal: 'gitlab' }));
  };

  const handleOpenLocalModal = () => {
    dispatch(openModal({ modal: 'local-scan' }));
  };

  return (
    <header className={styles.header}>
      <div className={styles.content}>
        <h1 className={styles.logo}>
          <svg width="32" height="32" viewBox="0 0 32 32" fill="none" className={styles.logoIcon} xmlns="http://www.w3.org/2000/svg">
            <path d="M16 4L4 10V22L16 28L28 22V10L16 4Z" stroke="currentColor" strokeWidth="2" fill="none"/>
            <path d="M16 4V28M4 10L16 16L28 10" stroke="currentColor" strokeWidth="2"/>
            <path d="M16 16V28" stroke="currentColor" strokeWidth="2"/>
          </svg>
          CodeAuditClaw
        </h1>
        <div className={styles.actions}>
          <Button variant="secondary" onClick={toggleTheme} title="切换主题">
            {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </Button>
          <Button variant="secondary" onClick={handleOpenUrlModal}>
            <Link2 size={18} />
            从 URL 导入
          </Button>
          <Button variant="secondary" onClick={handleOpenGitHubModal}>
            <Github size={18} />
            GitHub
          </Button>
          <Button variant="secondary" onClick={handleOpenGitLabModal}>
            <GitLabIcon size={18} />
            GitLab
          </Button>
          <Button variant="primary" onClick={handleOpenLocalModal}>
            <FolderOpen size={18} />
            本地扫描
          </Button>
        </div>
      </div>
    </header>
  );
}
