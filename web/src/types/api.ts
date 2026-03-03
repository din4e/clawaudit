// ==================== Scan Types ====================

export type ScanStatus = 'pending' | 'cloning' | 'scanning' | 'completed' | 'failed';
export type ScanType = 'security' | 'quality' | 'secrets' | 'compliance';
export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export interface Scan {
  id: string;
  repo_path: string;
  repo_name: string;
  branch: string | null;
  status: ScanStatus;
  scan_types: ScanType[];
  started_at: string;
  completed_at: string | null;
  total_files: number | null;
  total_batches: number | null;
  completed_batches: number | null;
  total_issues: number | null;
  error?: string;
  message?: string;
  progress?: number;
  batches?: Batch[];
  summary?: ScanSummary;
}

export interface Batch {
  id?: number;
  scan_id: string;
  batch_id: number;
  files: string[];
  status: string;
  started_at: string;
  completed_at: string | null;
  tokens_used: number | null;
  error_message: string | null;
  issues?: Issue[];
}

export interface Issue {
  id: string;
  scan_id: string;
  batch_id: number;
  file_path: string;
  line_number: number;
  column_number: number | null;
  severity: Severity;
  scan_type: ScanType;
  title: string;
  description: string;
  code_snippet: string | null;
  rule_id: string | null;
  cwe: string | null;
  references: string | null;
}

export interface ScanSummary {
  scan_id: string;
  severity_critical: number;
  severity_high: number;
  severity_medium: number;
  severity_low: number;
  severity_info: number;
  type_security: number;
  type_quality: number;
  type_secrets: number;
  type_compliance: number;
}

export interface ScanStatusResponse {
  scan_id: string;
  status: ScanStatus;
  progress: number;
  message: string;
}

export interface CreateScanRequest {
  repo_path: string;
  branch?: string;
  scan_types: ScanType[];
  batch_size?: number;
  max_context?: number;
}

export interface CreateScanResponse {
  scan_id: string;
  status: ScanStatus;
}

// ==================== GitLab Types ====================

export interface GitLabProject {
  id: number;
  name: string;
  name_with_namespace: string;
  path: string;
  path_with_namespace: string;
  description: string;
  web_url: string;
  star_count: number;
  forks_count: number;
  archived: boolean;
  default_branch: string;
  last_activity_at: string;
  created_at: string;
}

export interface GitLabBranch {
  name: string;
  default: boolean;
  commit: {
    id: string;
    short_id: string;
    title: string;
  };
}

export interface GitLabValidateRequest {
  token: string;
  gitlab_url?: string;
}

export interface GitLabValidateResponse {
  valid: boolean;
  error?: string;
}

export interface GitLabScanRequest {
  token: string;
  gitlab_url?: string;
  project_id: number;
  branch?: string;
  scan_types: ScanType[];
  batch_size?: number;
  max_context?: number;
}

// ==================== GitHub Types ====================

export interface GitHubRepository {
  id: number;
  name: string;
  full_name: string;
  description: string;
  private: boolean;
  fork: boolean;
  archived: boolean;
  stargazers_count: number;
  forks_count: number;
  default_branch: string;
  owner: {
    login: string;
    id: number;
    avatar_url: string;
  };
  html_url: string;
  updated_at: string;
  created_at: string;
}

export interface GitHubBranch {
  name: string;
  commit: {
    sha: string;
    url: string;
  };
  protected: boolean;
}

export interface GitHubValidateRequest {
  token: string;
  github_url?: string;
}

export interface GitHubValidateResponse {
  valid: boolean;
  username?: string;
  error?: string;
}

export interface GitHubScanRequest {
  token: string;
  github_url?: string;
  owner: string;
  name: string;
  branch?: string;
  scan_types: ScanType[];
  batch_size?: number;
  max_context?: number;
}

// ==================== URL Scan Types ====================

export interface UrlScanRequest {
  url: string;
  branch?: string;
  scan_types: ScanType[];
  batch_size?: number;
  max_context?: number;
}

export interface GitUrlInfo {
  platform: string;
  owner: string;
  name: string;
  url: string;
}

// ==================== List Responses ====================

export interface ScansListResponse {
  scans: Scan[];
  total: number;
}

export interface IssuesListResponse {
  scan_id: string;
  issues: Issue[];
  total: number;
}

export interface StatsResponse {
  total_scans: number;
  total_issues: number;
  completed_scans: number;
}

// ==================== API Error ====================

export interface ApiError {
  error: string;
  message?: string;
}
