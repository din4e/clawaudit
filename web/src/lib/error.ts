// RTK Query error handling utilities

export interface RTKError {
  status?: number | string;
  data?: {
    error?: string;
    message?: string;
  };
}

export function getErrorMessage(error: unknown): string {
  if (error && typeof error === 'object') {
    // Handle RTK Query error structure
    if ('data' in error) {
      const errData = (error as RTKError).data;
      if (errData?.error) return errData.error;
      if (errData?.message) return errData.message;
    }
    // Handle standard Error
    if ('message' in error) {
      return (error as Error).message;
    }
  }
  if (typeof error === 'string') {
    return error;
  }
  return '未知错误';
}

export function formatError(prefix: string, error: unknown): string {
  return `${prefix}: ${getErrorMessage(error)}`;
}
