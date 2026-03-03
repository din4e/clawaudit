// API Configuration
// Priority: 1. Environment variable, 2. localhost fallback, 3. relative path (production)

function getApiBaseUrl(): string {
  if (typeof window === 'undefined') return '/api';

  const envApiUrl = process.env.NEXT_PUBLIC_API_URL;
  if (envApiUrl) return `${envApiUrl}/api`;

  // Auto-detect backend port for development
  if (window.location.hostname === 'localhost') {
    // Backend runs on port 9999
    return 'http://localhost:9999/api';
  }

  return '/api';
}

function getWsBaseUrl(): string {
  if (typeof window === 'undefined') return 'ws://localhost:9999';

  const envWsUrl = process.env.NEXT_PUBLIC_WS_URL;
  if (envWsUrl) return envWsUrl;

  if (window.location.hostname === 'localhost') {
    const frontendPort = parseInt(window.location.port) || 4200;
    const backendPort = frontendPort + 1;
    return `ws://localhost:${backendPort}`;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}`;
}

export const API_BASE = getApiBaseUrl();
export const WS_BASE = getWsBaseUrl();

export const apiConfig = {
  base: API_BASE,
  endpoints: {
    // Scans
    scans: () => `${API_BASE}/scans`,
    scan: (id: string) => `${API_BASE}/scan/${id}`,
    scanStatus: (id: string) => `${API_BASE}/scan/${id}/status`,
    deleteScan: (id: string) => `${API_BASE}/scan/${id}`,
    scanBatches: (id: string) => `${API_BASE}/scan/${id}/batches`,
    scanIssues: (id: string) => `${API_BASE}/scan/${id}/issues`,
    scanIssuesBySeverity: (id: string, severity: string) => `${API_BASE}/scan/${id}/issues/severity/${severity}`,

    // GitLab
    gitlabValidate: () => `${API_BASE}/gitlab/validate`,
    gitlabProjects: () => `${API_BASE}/gitlab/projects`,
    gitlabProject: (id: number) => `${API_BASE}/gitlab/projects/${id}`,
    gitlabBranches: (id: number) => `${API_BASE}/gitlab/projects/${id}/branches`,
    gitlabScan: (id: number) => `${API_BASE}/gitlab/projects/${id}/scan`,
    gitlabCache: (id: number) => `${API_BASE}/gitlab/projects/${id}/cache`,

    // GitHub
    githubValidate: () => `${API_BASE}/github/validate`,
    githubRepositories: () => `${API_BASE}/github/repositories`,
    githubSearch: () => `${API_BASE}/github/search`,
    githubRepo: (owner: string, name: string) => `${API_BASE}/github/repos/${owner}/${name}`,
    githubBranches: (owner: string, name: string) => `${API_BASE}/github/repos/${owner}/${name}/branches`,
    githubScan: (owner: string, name: string) => `${API_BASE}/github/repos/${owner}/${name}/scan`,
    githubCache: (owner: string, name: string) => `${API_BASE}/github/repos/${owner}/${name}/cache`,

    // URL Scan
    scanByUrl: () => `${API_BASE}/scan/url`,
    parseGitUrl: () => `${API_BASE}/git/parse-url`,
    gitBranches: () => `${API_BASE}/git/branches`,

    // Stats
    stats: () => `${API_BASE}/stats`,
  },
};

// API Client Helper
export class ApiClient {
  private base: string;

  constructor(base: string = API_BASE) {
    this.base = base;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = endpoint.startsWith('http') ? endpoint : `${this.base}${endpoint}`;

    const config: RequestInit = {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    };

    const response = await fetch(url, config);

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Request failed' }));
      throw new Error(error.error || error.message || 'Request failed');
    }

    // DELETE requests typically have no response body
    if (options.method === 'DELETE') {
      return null as T;
    }

    return response.json();
  }

  get<T>(endpoint: string, headers?: HeadersInit): Promise<T> {
    return this.request<T>(endpoint, { method: 'GET', headers });
  }

  post<T>(endpoint: string, data?: unknown, headers?: HeadersInit): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'POST',
      headers,
      body: data ? JSON.stringify(data) : undefined,
    });
  }

  put<T>(endpoint: string, data?: unknown, headers?: HeadersInit): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'PUT',
      headers,
      body: data ? JSON.stringify(data) : undefined,
    });
  }

  delete<T>(endpoint: string, headers?: HeadersInit): Promise<T> {
    return this.request<T>(endpoint, { method: 'DELETE', headers });
  }

  // GitLab-specific method with custom headers
  getGitLab<T>(endpoint: string, token: string, gitlabUrl?: string): Promise<T> {
    const headers: HeadersInit = {
      'X-GitLab-Token': token,
    };
    if (gitlabUrl) {
      headers['X-GitLab-URL'] = gitlabUrl;
    }
    return this.get<T>(endpoint, headers);
  }

  // GitHub-specific method with custom headers
  getGitHub<T>(endpoint: string, token: string, githubUrl?: string): Promise<T> {
    const headers: HeadersInit = {
      'X-GitHub-Token': token,
    };
    if (githubUrl) {
      headers['X-GitHub-URL'] = githubUrl;
    }
    return this.get<T>(endpoint, headers);
  }

  postGitLab<T>(endpoint: string, token: string, gitlabUrl: string, data: unknown): Promise<T> {
    const headers: HeadersInit = {
      'X-GitLab-Token': token,
      'X-GitLab-URL': gitlabUrl,
    };
    return this.post<T>(endpoint, data, headers);
  }

  postGitHub<T>(endpoint: string, token: string, githubUrl: string, data: unknown): Promise<T> {
    const headers: HeadersInit = {
      'X-GitHub-Token': token,
      'X-GitHub-URL': githubUrl,
    };
    return this.post<T>(endpoint, data, headers);
  }
}

export const apiClient = new ApiClient();

// Utility functions
export function isScanActive(status: string): boolean {
  return ['pending', 'copying', 'cloning', 'scanning'].includes(status);
}

export function getStatusText(status: string): string {
  const statusMap: Record<string, string> = {
    'pending': '等待中',
    'copying': '复制中',
    'cloning': '克隆中',
    'scanning': '扫描中',
    'completed': '已完成',
    'failed': '失败',
  };
  return statusMap[status] || status;
}

export function getSeverityText(severity: string): string {
  const severityMap: Record<string, string> = {
    'critical': '严重',
    'high': '高危',
    'medium': '中危',
    'low': '低危',
    'info': '信息',
  };
  return severityMap[severity] || severity;
}

export function getSeverityColor(severity: string): string {
  const colorMap: Record<string, string> = {
    'critical': 'var(--severity-critical)',
    'high': 'var(--severity-high)',
    'medium': 'var(--severity-medium)',
    'low': 'var(--severity-low)',
    'info': 'var(--severity-info)',
  };
  return colorMap[severity] || 'var(--color-text-secondary)';
}

export function formatTime(isoString: string | null): string {
  if (!isoString) return '-';
  const date = new Date(isoString);
  return date.toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function formatNumber(num: number): string {
  return num.toLocaleString('zh-CN');
}
