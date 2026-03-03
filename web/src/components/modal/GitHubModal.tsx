'use client';

import React, { useState } from 'react';
import { Modal, Input, Button, Badge } from '@/components/ui';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectActiveModal, closeModal } from '@/store/slices/uiSlice';
import { useGithubValidateMutation, useGithubListRepositoriesQuery, useLazyGithubSearchRepositoriesQuery, useGithubGetBranchesQuery, useGithubScanRepositoryMutation } from '@/store/services/api';
import { selectScan, fetchScans } from '@/store/slices/scansSlice';
import { useDebounce } from '@/hooks/useDebounce';
import type { ScanType } from '@/types/api';
import { Github as GithubIcon, Star, GitFork, Lock } from 'lucide-react';
import { Checkbox, CheckboxGroup } from '@/components/ui';

const scanTypeOptions: { value: ScanType; label: string; checked?: boolean }[] = [
  { value: 'security', label: '安全漏洞', checked: true },
  { value: 'quality', label: '代码质量', checked: true },
  { value: 'secrets', label: '敏感信息', checked: true },
  { value: 'compliance', label: '合规检查' },
];

export function GitHubModal() {
  const dispatch = useAppDispatch();
  const isOpen = useAppSelector(selectActiveModal) === 'github';

  const [step, setStep] = useState(1);
  const [token, setToken] = useState('');
  const [githubUrl, setGithubUrl] = useState('https://github.com');
  const [search, setSearch] = useState('');
  const [myReposOnly, setMyReposOnly] = useState(true);
  const [selectedRepo, setSelectedRepo] = useState<any>(null);
  const [branch, setBranch] = useState('');
  const [scanTypes, setScanTypes] = useState<ScanType[]>(['security', 'quality', 'secrets']);
  const [batchSize, setBatchSize] = useState(5);
  const [maxContext, setMaxContext] = useState(100000);

  const debouncedSearch = useDebounce(search, 300);

  const [githubValidate] = useGithubValidateMutation();
  const { data: reposData, isLoading: reposLoading, refetch: refetchList } = useGithubListRepositoriesQuery(
    {
      token,
      githubUrl,
      affiliation: myReposOnly ? 'owner,collaborator' : '',
    },
    { skip: step !== 2 || !token || !myReposOnly }
  );

  const [searchRepos, { data: searchData, isLoading: searchLoading }] = useLazyGithubSearchRepositoriesQuery();

  const { data: branchesData, isLoading: branchesLoading } = useGithubGetBranchesQuery(
    {
      owner: selectedRepo?.owner?.login,
      name: selectedRepo?.name,
      token,
      githubUrl,
    },
    { skip: step !== 3 || !selectedRepo || !token }
  );

  const [githubScan, { isLoading: isScanning }] = useGithubScanRepositoryMutation();

  // Handle search
  React.useEffect(() => {
    if (step === 2 && !myReposOnly && debouncedSearch && token) {
      searchRepos({ token, githubUrl, query: debouncedSearch });
    }
  }, [debouncedSearch, myReposOnly, step, token, githubUrl, searchRepos]);

  const currentData = myReposOnly ? reposData : searchData;
  const isLoading = myReposOnly ? reposLoading : searchLoading;

  const handleClose = () => {
    dispatch(closeModal());
    setStep(1);
    resetForm();
  };

  const resetForm = () => {
    setToken('');
    setGithubUrl('https://github.com');
    setSearch('');
    setSelectedRepo(null);
    setBranch('');
  };

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const result = await githubValidate({ token, github_url: githubUrl }).unwrap();
      if (result.valid) {
        setStep(2);
      } else {
        alert('Token 验证失败');
      }
    } catch (error) {
      alert('连接失败: ' + (error as Error).message);
    }
  };

  const handleSelectRepo = (repo: any) => {
    setSelectedRepo(repo);
    setStep(3);
  };

  const handleStartScan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedRepo) return;

    if (scanTypes.length === 0) {
      alert('请至少选择一种扫描类型');
      return;
    }

    try {
      const result = await githubScan({
        owner: selectedRepo.owner.login,
        name: selectedRepo.name,
        token,
        github_url: githubUrl,
        branch,
        scan_types: scanTypes,
        batch_size: batchSize,
        max_context: maxContext,
      }).unwrap();

      dispatch(selectScan(result.scan_id));
      dispatch(fetchScans({})); // Refresh the scan list in sidebar
      handleClose();
    } catch (error) {
      alert('创建扫描失败: ' + (error as Error).message);
    }
  };

  const displayedRepos = currentData?.repositories || [];

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title="从 GitHub 导入项目"
      size="lg"
      footer={
        <>
          {step === 1 && (
            <>
              <Button variant="secondary" onClick={handleClose}>取消</Button>
              <Button variant="primary" onClick={handleConnect}>连接</Button>
            </>
          )}
          {step === 2 && (
            <Button variant="secondary" onClick={() => setStep(1)}>返回</Button>
          )}
          {step === 3 && (
            <>
              <Button variant="secondary" onClick={() => setStep(2)}>返回</Button>
              <Button variant="primary" onClick={handleStartScan} loading={isScanning}>开始扫描</Button>
            </>
          )}
        </>
      }
    >
      {/* Step 1: Connect */}
      {step === 1 && (
        <form onSubmit={handleConnect}>
          <Input
            label="Personal Access Token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="ghp_xxxxxxxxxxxxxxxxxxxx"
            type="password"
            fullWidth
            helperText="repo 和 public_repo 权限"
          />
          <Input
            label="GitHub URL"
            value={githubUrl}
            onChange={(e) => setGithubUrl(e.target.value)}
            placeholder="https://github.com"
            fullWidth
            helperText="企业版请输入您的 GitHub 地址"
          />
        </form>
      )}

      {/* Step 2: Select Repository */}
      {step === 2 && (
        <div>
          <div style={{ display: 'flex', gap: '16px', marginBottom: '16px', alignItems: 'center' }}>
            <Input
              placeholder="搜索项目..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              fullWidth
            />
            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', whiteSpace: 'nowrap' }}>
              <Checkbox
                checked={myReposOnly}
                onChange={(e) => {
                  setMyReposOnly(e.target.checked);
                  if (e.target.checked && token) {
                    refetchList();
                  }
                }}
              />
              <span>只看我的仓库</span>
            </label>
          </div>
          <div style={{ maxHeight: '300px', overflowY: 'auto', border: '1px solid var(--color-border)', borderRadius: '6px' }}>
            {isLoading ? (
              <div style={{ padding: '32px', textAlign: 'center', color: 'var(--color-text-secondary)' }}>
                {myReposOnly ? '加载中...' : '搜索中...'}
              </div>
            ) : !displayedRepos?.length ? (
              <div style={{ padding: '32px', textAlign: 'center', color: 'var(--color-text-secondary)' }}>未找到仓库</div>
            ) : (
              displayedRepos.map((repo: any) => (
                <div
                  key={repo.id}
                  onClick={() => handleSelectRepo(repo)}
                  style={{
                    padding: '16px',
                    borderBottom: '1px solid var(--color-border)',
                    cursor: 'pointer',
                  }}
                  onMouseEnter={(e) => e.currentTarget.style.backgroundColor = 'var(--color-bg-tertiary)'}
                  onMouseLeave={(e) => e.currentTarget.style.backgroundColor = ''}
                >
                  <div style={{ fontWeight: 500, marginBottom: '4px' }}>{repo.full_name}</div>
                  <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: '8px' }}>
                    {repo.description || '无描述'}
                  </div>
                  <div style={{ display: 'flex', gap: '16px', fontSize: '12px', color: 'var(--color-text-secondary)', flexWrap: 'wrap' }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <Star size={12} /> {repo.stargazers_count}
                    </span>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <GitFork size={12} /> {repo.forks_count}
                    </span>
                    {repo.private && <Badge variant="default">私有</Badge>}
                    {repo.fork && <Badge variant="default">Fork</Badge>}
                    {repo.archived && <Badge variant="default">已归档</Badge>}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Step 3: Configure Scan */}
      {step === 3 && (
        <form onSubmit={handleStartScan}>
          <div style={{ padding: '16px', backgroundColor: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: '6px', marginBottom: '16px' }}>
            <strong>项目:</strong> {selectedRepo?.full_name}
          </div>
          <div className="form-group">
            <label>分支</label>
            <select
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              style={{
                width: '100%',
                padding: '8px 16px',
                border: '1px solid var(--color-border)',
                borderRadius: '6px',
                backgroundColor: 'var(--color-bg)',
                color: 'var(--color-text-primary)',
              }}
            >
              <option value="">默认分支</option>
              {branchesData?.branches?.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </select>
          </div>
          <CheckboxGroup
            label="扫描类型"
            name="githubScanTypes"
            options={scanTypeOptions}
            onChange={(values) => setScanTypes(values as ScanType[])}
          />
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
            <Input
              label="批次大小"
              type="number"
              value={batchSize}
              onChange={(e) => setBatchSize(parseInt(e.target.value) || 5)}
              min={1}
              max={20}
            />
            <Input
              label="最大上下文 (tokens)"
              type="number"
              value={maxContext}
              onChange={(e) => setMaxContext(parseInt(e.target.value) || 100000)}
              min={10000}
              max={200000}
              step={10000}
            />
          </div>
        </form>
      )}
    </Modal>
  );
}
