'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import styles from './ResizableSplitPane.module.css';

interface ResizableSplitPaneProps {
  children: [React.ReactNode, React.ReactNode]; // [sidebar, main]
  defaultSidebarWidth?: number;
  minSidebarWidth?: number;
  maxSidebarWidth?: number;
  storageKey?: string;
}

export function ResizableSplitPane({
  children,
  defaultSidebarWidth = 280,
  minSidebarWidth = 200,
  maxSidebarWidth = 600,
  storageKey = 'sidebar-width',
}: ResizableSplitPaneProps) {
  const [sidebarWidth, setSidebarWidth] = useState(defaultSidebarWidth);
  const [isResizing, setIsResizing] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const resizerRef = useRef<HTMLDivElement>(null);
  const startXRef = useRef(0);
  const startWidthRef = useRef(0);

  // Load saved width from localStorage
  useEffect(() => {
    if (typeof window !== 'undefined' && storageKey) {
      try {
        const saved = localStorage.getItem(storageKey);
        if (saved) {
          const width = parseInt(saved, 10);
          if (!isNaN(width) && width >= minSidebarWidth && width <= maxSidebarWidth) {
            setSidebarWidth(width);
          }
        }
      } catch (e) {
        // Ignore localStorage errors
      }
    }
  }, [storageKey, minSidebarWidth, maxSidebarWidth]);

  // Save width to localStorage when it changes
  useEffect(() => {
    if (typeof window !== 'undefined' && storageKey) {
      try {
        localStorage.setItem(storageKey, String(sidebarWidth));
      } catch (e) {
        // Ignore localStorage errors
      }
    }
  }, [sidebarWidth, storageKey]);

  // Handle mouse down on resize handle
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setIsResizing(true);
    startXRef.current = e.clientX;
    startWidthRef.current = sidebarWidth;
  }, [sidebarWidth]);

  // Handle keyboard resize
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 50 : 10;
    let newWidth = sidebarWidth;

    switch (e.key) {
      case 'ArrowLeft':
        newWidth = Math.max(minSidebarWidth, sidebarWidth - step);
        e.preventDefault();
        break;
      case 'ArrowRight':
        newWidth = Math.min(maxSidebarWidth, sidebarWidth + step);
        e.preventDefault();
        break;
      default:
        return;
    }

    setSidebarWidth(newWidth);
  }, [sidebarWidth, minSidebarWidth, maxSidebarWidth]);

  // Handle mouse move during resize
  useEffect(() => {
    if (!isResizing) return;

    const handleMouseMove = (e: MouseEvent) => {
      const deltaX = e.clientX - startXRef.current;
      const newWidth = Math.max(
        minSidebarWidth,
        Math.min(maxSidebarWidth, startWidthRef.current + deltaX)
      );
      setSidebarWidth(newWidth);
    };

    const handleMouseUp = () => {
      setIsResizing(false);
      // Return focus to resizer after mouse up
      if (resizerRef.current) {
        resizerRef.current.focus();
      }
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isResizing, minSidebarWidth, maxSidebarWidth]);

  // Prevent text selection during resize
  useEffect(() => {
    if (isResizing) {
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'col-resize';
    } else {
      document.body.style.userSelect = '';
      document.body.style.cursor = '';
    }

    return () => {
      document.body.style.userSelect = '';
      document.body.style.cursor = '';
    };
  }, [isResizing]);

  const [sidebar, main] = children;

  return (
    <div ref={containerRef} className={styles.container}>
      <div
        className={styles.sidebar}
        style={{ width: `${sidebarWidth}px` }}
      >
        {sidebar}
      </div>
      <div
        ref={resizerRef}
        className={`${styles.resizer} ${isResizing ? styles.resizing : ''}`}
        onMouseDown={handleMouseDown}
        onKeyDown={handleKeyDown}
        tabIndex={0}
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={sidebarWidth}
        aria-valuemin={minSidebarWidth}
        aria-valuemax={maxSidebarWidth}
        aria-label="调整侧边栏宽度，使用左右箭头键微调"
        title="拖动或使用左右箭头键调整宽度"
      >
        <div className={styles.resizerHandle} />
      </div>
      <div className={styles.main}>
        {main}
      </div>
    </div>
  );
}
