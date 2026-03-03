'use client';

import React from 'react';
import { Sidebar, ResizableSplitPane } from '@/components/layout';
import { ScanDetail } from '@/components/scan';
import { LocalScanModal, GitLabModal, GitHubModal, UrlModal } from '@/components/modal';

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
