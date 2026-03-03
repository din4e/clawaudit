import { useEffect, useRef, useCallback, useState } from 'react';
import { useSelector } from 'react-redux';
import { WS_BASE } from '@/lib/api';

interface WebSocketMessage {
  type: 'connected' | 'progress' | 'batch_start' | 'batch_complete' | 'complete' | 'error' | 'step' | 'file_scan' | 'claude_response';
  scan_id: string;
  data: {
    progress?: number;
    message?: string;
    batch_id?: number;
    files?: string[];
    issues_found?: number;
    total_files?: number;
    total_batches?: number;
    file?: string;
    current?: number;
    total?: number;
    step?: string;
    response?: string;
    error?: string;
    [key: string]: any;
  };
}

interface UseWebSocketOptions {
  onMessage?: (message: WebSocketMessage) => void;
  onProgress?: (progress: number, message: string) => void;
  onBatchStart?: (batchId: number, files: string[]) => void;
  onBatchComplete?: (batchId: number, issuesFound: number) => void;
  onIssue?: (issue: any) => void;
  onComplete?: (result: any) => void;
  onError?: (error: string) => void;
}

export function useWebSocket(scanId: string, options: UseWebSocketOptions = {}) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const [messages, setMessages] = useState<WebSocketMessage[]>([]);

  const connect = useCallback(() => {
    if (!scanId) return;

    // Use configured WebSocket URL
    const wsUrl = `${WS_BASE}/ws/scan/${scanId}`;

    try {
      wsRef.current = new WebSocket(wsUrl);

      wsRef.current.onopen = () => {
        console.log('[WebSocket] Connected to scan progress');
        setIsConnected(true);
      };

      wsRef.current.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data);
          console.log('[WebSocket] Received:', message);

          // Store message
          setMessages(prev => [...prev, message]);

          // Call specific handlers
          if (options.onMessage) {
            options.onMessage(message);
          }

          switch (message.type) {
            case 'connected':
              // Initial connection
              break;

            case 'progress':
              if (options.onProgress && message.data.progress !== undefined) {
                options.onProgress(message.data.progress, message.data.message || '');
              }
              break;

            case 'batch_start':
              if (options.onBatchStart && message.data.batch_id !== undefined) {
                options.onBatchStart(message.data.batch_id, message.data.files || []);
              }
              break;

            case 'batch_complete':
              if (options.onBatchComplete && message.data.batch_id !== undefined) {
                options.onBatchComplete(message.data.batch_id, message.data.issues_found || 0);
              }
              break;

            case 'complete':
              if (options.onComplete) {
                options.onComplete(message.data);
              }
              break;

            case 'error':
              if (options.onError) {
                options.onError(message.data.error || 'Unknown error');
              }
              break;

            case 'claude_response':
              // Handle Claude response - could be shown in UI
              break;
          }
        } catch (err) {
          console.error('[WebSocket] Failed to parse message:', err);
        }
      };

      wsRef.current.onclose = () => {
        console.log('[WebSocket] Disconnected');
        setIsConnected(false);
        // Auto-reconnect after 3 seconds if scan is still active
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 3000);
      };

      wsRef.current.onerror = (error) => {
        console.error('[WebSocket] Error:', error);
      };
    } catch (err) {
      console.error('[WebSocket] Failed to connect:', err);
    }
  }, [scanId]);

  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setIsConnected(false);
  }, []);

  const sendMessage = useCallback((data: any) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(data));
    }
  }, []);

  // Auto-connect when scanId changes
  useEffect(() => {
    if (scanId) {
      connect();
    }

    return () => {
      disconnect();
    };
  }, [scanId, connect, disconnect]);

  return {
    isConnected,
    messages,
    sendMessage,
    connect,
    disconnect,
  };
}
