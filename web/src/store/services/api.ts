import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';
import { API_BASE } from '@/lib/api';
import type {
  Scan,
  ScanStatusResponse,
  CreateScanRequest,
  CreateScanResponse,
  Batch,
  Issue,
  ScansListResponse,
  IssuesListResponse,
  GitLabProject,
  GitLabBranch,
  GitLabValidateRequest,
  GitLabValidateResponse,
  GitLabScanRequest,
  GitHubRepository,
  GitHubBranch,
  GitHubValidateRequest,
  GitHubValidateResponse,
  GitHubScanRequest,
  UrlScanRequest,
  GitUrlInfo,
  StatsResponse,
} from '@/types/api';

// Create a base query with proper headers handling
const baseQuery = fetchBaseQuery({
  baseUrl: API_BASE,
  prepareHeaders: (headers) => {
    headers.set('Content-Type', 'application/json');
    return headers;
  },
});

export const apiSlice = createApi({
  reducerPath: 'api',
  baseQuery,
  tagTypes: ['Scan', 'Issue', 'Batch'],
  endpoints: (builder) => ({
    // ==================== Scans ====================

    listScans: builder.query<ScansListResponse, { status?: string; limit?: number; offset?: number }>({
      query: (params) => ({
        url: '/scans',
        params: {
          status: params.status,
          limit: params.limit || 50,
          offset: params.offset || 0,
        },
      }),
      providesTags: (result) =>
        result
          ? [
              ...result.scans.map(({ id }) => ({ type: 'Scan' as const, id })),
              { type: 'Scan', id: 'LIST' },
            ]
          : [{ type: 'Scan', id: 'LIST' }],
    }),

    getScan: builder.query<Scan, string>({
      query: (id) => `/scan/${id}`,
      providesTags: (result, error, id) => [{ type: 'Scan', id }],
    }),

    getScanStatus: builder.query<ScanStatusResponse, string>({
      query: (id) => `/scan/${id}/status`,
      providesTags: (result, error, id) => [{ type: 'Scan', id }],
    }),

    createScan: builder.mutation<CreateScanResponse, CreateScanRequest>({
      query: (data) => ({
        url: '/scan',
        method: 'POST',
        body: data,
      }),
      invalidatesTags: [{ type: 'Scan', id: 'LIST' }],
    }),

    deleteScan: builder.mutation<void, string>({
      query: (id) => ({
        url: `/scan/${id}`,
        method: 'DELETE',
      }),
      invalidatesTags: (result, error, id) => [
        { type: 'Scan', id },
        { type: 'Scan', id: 'LIST' },
      ],
    }),

    getScanBatches: builder.query<{ scan_id: string; batches: Batch[] }, string>({
      query: (id) => `/scan/${id}/batches`,
      providesTags: (result, error, id) => [{ type: 'Batch', id }],
    }),

    getScanIssues: builder.query<IssuesListResponse, { id: string; limit?: number; offset?: number }>({
      query: ({ id, limit = 100, offset = 0 }) => `/scan/${id}/issues?limit=${limit}&offset=${offset}`,
      providesTags: (result, error, { id }) => [{ type: 'Issue', id }],
    }),

    getIssuesBySeverity: builder.query<{ scan_id: string; issues: Issue[]; total: number }, { id: string; severity: string }>({
      query: ({ id, severity }) => `/scan/${id}/issues/severity/${severity}`,
      providesTags: (result, error, { id }) => [{ type: 'Issue', id }],
    }),

    getScanOutput: builder.query<any, string>({
      query: (id) => `/scan/${id}/output`,
      providesTags: (result, error, id) => [{ type: 'Scan', id }],
    }),

    getStats: builder.query<StatsResponse, void>({
      query: () => '/stats',
    }),

    // ==================== GitLab ====================

    gitlabValidate: builder.mutation<GitLabValidateResponse, GitLabValidateRequest>({
      query: (data) => ({
        url: '/gitlab/validate',
        method: 'POST',
        body: data,
      }),
    }),

    gitlabListProjects: builder.query<
      { projects: GitLabProject[]; total: number },
      { token: string; gitlabUrl?: string; search?: string; membership?: boolean }
    >({
      query: ({ token, gitlabUrl, search, membership }) => ({
        url: '/gitlab/projects',
        params: { search, membership },
        headers: {
          'X-GitLab-Token': token,
          'X-GitLab-URL': gitlabUrl || 'https://gitlab.com',
        },
      }),
    }),

    gitlabGetProject: builder.query<GitLabProject, { id: number; token: string; gitlabUrl?: string }>({
      query: ({ id, token, gitlabUrl }) => ({
        url: `/gitlab/projects/${id}`,
        headers: {
          'X-GitLab-Token': token,
          'X-GitLab-URL': gitlabUrl || 'https://gitlab.com',
        },
      }),
    }),

    gitlabGetBranches: builder.query<{ branches: GitLabBranch[]; total: number }, { id: number; token: string; gitlabUrl?: string }>({
      query: ({ id, token, gitlabUrl }) => ({
        url: `/gitlab/projects/${id}/branches`,
        headers: {
          'X-GitLab-Token': token,
          'X-GitLab-URL': gitlabUrl || 'https://gitlab.com',
        },
      }),
    }),

    gitlabScanProject: builder.mutation<CreateScanResponse, GitLabScanRequest & { id: number }>({
      query: ({ id, ...data }) => ({
        url: `/gitlab/projects/${id}/scan`,
        method: 'POST',
        body: data,
      }),
      invalidatesTags: [{ type: 'Scan', id: 'LIST' }],
    }),

    // ==================== GitHub ====================

    githubValidate: builder.mutation<GitHubValidateResponse, GitHubValidateRequest>({
      query: (data) => ({
        url: '/github/validate',
        method: 'POST',
        body: data,
      }),
    }),

    githubListRepositories: builder.query<
      { repositories: GitHubRepository[]; total: number },
      { token: string; githubUrl?: string; affiliation?: string }
    >({
      query: ({ token, githubUrl, affiliation }) => ({
        url: '/github/repositories',
        params: { affiliation },
        headers: {
          'X-GitHub-Token': token,
          'X-GitHub-URL': githubUrl || 'https://github.com',
        },
      }),
    }),

    githubSearchRepositories: builder.query<
      { repositories: GitHubRepository[]; total: number },
      { token: string; githubUrl?: string; query: string }
    >({
      query: ({ token, githubUrl, query }) => ({
        url: '/github/search',
        params: { q: query },
        headers: {
          'X-GitHub-Token': token,
          'X-GitHub-URL': githubUrl || 'https://github.com',
        },
      }),
    }),

    githubGetRepository: builder.query<GitHubRepository, { owner: string; name: string; token: string; githubUrl?: string }>({
      query: ({ owner, name, token, githubUrl }) => ({
        url: `/github/repos/${owner}/${name}`,
        headers: {
          'X-GitHub-Token': token,
          'X-GitHub-URL': githubUrl || 'https://github.com',
        },
      }),
    }),

    githubGetBranches: builder.query<{ branches: GitHubBranch[]; total: number }, { owner: string; name: string; token: string; githubUrl?: string }>({
      query: ({ owner, name, token, githubUrl }) => ({
        url: `/github/repos/${owner}/${name}/branches`,
        headers: {
          'X-GitHub-Token': token,
          'X-GitHub-URL': githubUrl || 'https://github.com',
        },
      }),
    }),

    githubScanRepository: builder.mutation<CreateScanResponse, GitHubScanRequest & { owner: string; name: string }>({
      query: ({ owner, name, ...data }) => ({
        url: `/github/repos/${owner}/${name}/scan`,
        method: 'POST',
        body: data,
      }),
      invalidatesTags: [{ type: 'Scan', id: 'LIST' }],
    }),

    // ==================== URL Scan ====================

    scanByUrl: builder.mutation<CreateScanResponse, UrlScanRequest>({
      query: (data) => ({
        url: '/scan/url',
        method: 'POST',
        body: data,
      }),
      invalidatesTags: [{ type: 'Scan', id: 'LIST' }],
    }),

    parseGitUrl: builder.query<GitUrlInfo, { url: string }>({
      query: ({ url }) => ({
        url: '/git/parse-url',
        params: { url },
      }),
    }),

    getRemoteBranches: builder.query<{ branches: string[]; total: number }, { url: string }>({
      query: ({ url }) => ({
        url: '/git/branches',
        params: { url },
      }),
    }),
  }),
});

// Export hooks
export const {
  // Scans
  useListScansQuery,
  useGetScanQuery,
  useGetScanStatusQuery,
  useCreateScanMutation,
  useDeleteScanMutation,
  useGetScanBatchesQuery,
  useGetScanIssuesQuery,
  useGetIssuesBySeverityQuery,
  useGetScanOutputQuery,
  useGetStatsQuery,

  // GitLab
  useGitlabValidateMutation,
  useGitlabListProjectsQuery,
  useLazyGitlabListProjectsQuery,
  useGitlabGetProjectQuery,
  useLazyGitlabGetProjectQuery,
  useGitlabGetBranchesQuery,
  useLazyGitlabGetBranchesQuery,
  useGitlabScanProjectMutation,

  // GitHub
  useGithubValidateMutation,
  useGithubListRepositoriesQuery,
  useLazyGithubListRepositoriesQuery,
  useGithubSearchRepositoriesQuery,
  useLazyGithubSearchRepositoriesQuery,
  useGithubGetRepositoryQuery,
  useLazyGithubGetRepositoryQuery,
  useGithubGetBranchesQuery,
  useLazyGithubGetBranchesQuery,
  useGithubScanRepositoryMutation,

  // URL Scan
  useScanByUrlMutation,
  useParseGitUrlQuery,
  useGetRemoteBranchesQuery,
  useLazyGetRemoteBranchesQuery,
} = apiSlice;
