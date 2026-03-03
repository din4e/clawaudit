import { ScanStatus, ScanType, Severity } from './api';

// Redux State Models

export interface ScansState {
  scans: Scan[];
  selectedScanId: string | null;
  total: number;
  filters: ScansFilters;
}

export interface ScansFilters {
  status?: ScanStatus;
  limit: number;
  offset: number;
}

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
}

export interface UiState {
  activeModal: ActiveModal | null;
  modalData: ModalData;
  filters: IssuesFilters;
  sidebarCollapsed: boolean;
}

export type ActiveModal =
  | 'local-scan'
  | 'gitlab'
  | 'github'
  | 'url'
  | null;

export interface ModalData {
  gitlab?: {
    token: string;
    url: string;
    step: number;
    selectedProject: GitLabProjectData | null;
    projects: GitLabProjectData[];
    branches: GitLabBranchData[];
  };
  github?: {
    token: string;
    url: string;
    step: number;
    selectedRepo: GitHubRepoData | null;
    repos: GitHubRepoData[];
    branches: GitHubBranchData[];
  };
  url?: {
    url: string;
    branches: string[];
  };
}

export interface GitLabProjectData {
  id: number;
  name: string;
  name_with_namespace: string;
  path_with_namespace: string;
  description: string;
  star_count: number;
  forks_count: number;
  archived: boolean;
  default_branch: string;
}

export interface GitLabBranchData {
  name: string;
  default: boolean;
}

export interface GitHubRepoData {
  owner: string;
  name: string;
  full_name: string;
  description: string;
  private: boolean;
  fork: boolean;
  archived: boolean;
  stargazers_count: number;
  forks_count: number;
  default_branch: string;
}

export interface GitHubBranchData {
  name: string;
}

export interface IssuesFilters {
  severity?: Severity;
  type?: ScanType;
  batchId?: number;
}

export interface ThemeState {
  theme: 'light' | 'dark';
}
