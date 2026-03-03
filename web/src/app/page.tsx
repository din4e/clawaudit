'use client';

import React from 'react';
import dynamic from 'next/dynamic';
import { Sidebar, ResizableSplitPane } from '@/components/layout';
import { ScanDetail } from '@/components/scan';

// Code split modals for better bundle size
const LocalScanModal = dynamic(() => import('@/components/modal/LocalScanModal'), {
  loading: () => null,
});

const GitLabModal = dynamic(() => import('@/components/modal/GitLabModal'), {
  loading: () => null,
});

const GitHubModal = dynamic(() => import('@/components/modal/GitHubModal'), {
  loading: () => null,
});

const UrlModal = dynamic(() => import('@/components/modal/UrlModal'), {
  loading: () => null,
});

export default function HomePage() {
  return (
    <>
      <ResizableSplitPane
        defaultSidebarWidth={280}
        minSidebarWidth={200}
        maxSidebarWidth={600}
        storageKey="code-auditor-sidebar-width"
      >
        <Sidebar />
        <main>
          <ScanDetail />
        </main>
      </ResizableSplitPane>
      <LocalScanModal />
      <GitLabModal />
      <GitHubModal />
      <UrlModal />
    </>
  );
}
