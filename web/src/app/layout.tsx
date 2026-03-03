'use client';

import React, { useEffect, useState } from 'react';
import { Providers } from './providers';
import { Header } from '@/components/layout';
import './globals.css';

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [isClient, setIsClient] = useState(false);

  useEffect(() => {
    setIsClient(true);
  }, []);

  // During SSR or before hydration, show minimal HTML
  if (!isClient) {
    return (
      <html lang="zh-CN">
        <body>
          <div className="app">
            <div className="main-content">
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}>
                <p>Loading...</p>
              </div>
            </div>
          </div>
        </body>
      </html>
    );
  }

  // After hydration, show full app with Redux and Header
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body>
        <Providers>
          <div className="app">
            <Header />
            <div className="main-content">
              {children}
            </div>
          </div>
        </Providers>
      </body>
    </html>
  );
}
