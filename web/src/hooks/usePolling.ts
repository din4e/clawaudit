import { useEffect, useRef, useCallback } from 'react';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { pollScanStatus, selectSelectedScanId } from '@/store/slices/scansSlice';
import { isScanActive } from '@/lib/api';

interface PollingOptions {
  interval?: number;
  enabled?: boolean;
}

/**
 * Hook for polling scan status
 */
export function usePolling(options: PollingOptions = {}) {
  const { interval = 2000, enabled = true } = options;
  const dispatch = useAppDispatch();
  const scanId = useAppSelector(selectSelectedScanId);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const consecutiveErrorsRef = useRef(0);
  const currentIntervalRef = useRef(interval);

  const startPolling = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }

    consecutiveErrorsRef.current = 0;
    currentIntervalRef.current = interval;

    const poll = async () => {
      if (!scanId) return;

      try {
        await dispatch(pollScanStatus(scanId)).unwrap();
        consecutiveErrorsRef.current = 0;
        currentIntervalRef.current = interval; // Reset interval on success
      } catch (error) {
        consecutiveErrorsRef.current++;
        console.error('Polling error:', error);

        // Exponential backoff
        if (consecutiveErrorsRef.current > 1) {
          currentIntervalRef.current = Math.min(
            currentIntervalRef.current * 1.5,
            10000 // Max 10 seconds
          );
        }

        // Stop polling after too many errors
        if (consecutiveErrorsRef.current >= 10) {
          console.error('Too many polling errors, stopping');
          stopPolling();
          return;
        }
      }

      // Continue polling
      if (intervalRef.current) {
        intervalRef.current = setTimeout(poll, currentIntervalRef.current);
      }
    };

    intervalRef.current = setTimeout(poll, currentIntervalRef.current);
  }, [scanId, interval, dispatch]);

  const stopPolling = useCallback(() => {
    if (intervalRef.current) {
      clearTimeout(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (enabled && scanId) {
      startPolling();
      return () => stopPolling();
    }
  }, [enabled, scanId, startPolling, stopPolling]);

  return { startPolling, stopPolling };
}

/**
 * Hook for polling a specific scan
 */
export function useScanPolling(scanId: string | null, options: PollingOptions = {}) {
  const { interval = 2000, enabled = true } = options;
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const consecutiveErrorsRef = useRef(0);

  useEffect(() => {
    if (!enabled || !scanId) return;

    const poll = async () => {
      try {
        const response = await fetch(`/api/scan/${scanId}/status`);
        if (response.ok) {
          const data = await response.json();

          // Stop polling if scan is complete or failed
          if (data.status === 'completed' || data.status === 'failed') {
            if (intervalRef.current) {
              clearTimeout(intervalRef.current);
              intervalRef.current = null;
            }
            // Trigger a refresh
            window.dispatchEvent(new CustomEvent('scan:complete', { detail: { scanId, data } }));
            return;
          }
        }

        consecutiveErrorsRef.current = 0;
      } catch (error) {
        consecutiveErrorsRef.current++;
        if (consecutiveErrorsRef.current >= 10) {
          if (intervalRef.current) {
            clearTimeout(intervalRef.current);
            intervalRef.current = null;
          }
          return;
        }
      }

      // Continue polling
      intervalRef.current = setTimeout(poll, interval);
    };

    intervalRef.current = setTimeout(poll, interval);

    return () => {
      if (intervalRef.current) {
        clearTimeout(intervalRef.current);
      }
    };
  }, [scanId, interval, enabled]);

  return { isPolling: !!intervalRef.current };
}
