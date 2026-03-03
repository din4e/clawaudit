'use client';

import React, { useState } from 'react';
import { Modal, Input, Button, Badge, Checkbox, CheckboxGroup } from '@/components/ui';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectActiveModal, closeModal, updateModalData } from '@/store/slices/uiSlice';
import { selectModalData } from '@/store/slices/uiSlice';
import { useGitlabValidateMutation, useGitlabListProjectsQuery, useGitlabGetBranchesQuery, useGitlabScanProjectMutation } from '@/store/services/api';
import { selectScan, fetchScans } from '@/store/slices/scansSlice';
import { useDebounce } from '@/hooks/useDebounce';
import type { ScanType } from '@/types/api';
import { Star, GitFork } from 'lucide-react';

const scanTypeOptions: { value: ScanType; label: string; checked?: boolean }[] = [
  { value: 'security', label: '安全漏洞', checked: true },
  { value: 'quality', label: '代码质量', checked: true },
  { value: 'secrets', label: '敏感信息', checked: true },
  { value: 'compliance', label: '合规检查' },
];

export function GitLabModal() {
  const dispatch = useAppDispatch();
  const isOpen = useAppSelector(selectActiveModal) === 'gitlab';
  const modalData = useAppSelector(selectModalData);

  const [step, setStep] = useState(1);
  const [token, setToken] = useState('');
  const [gitlabUrl, setGitlabUrl] = useState('https://gitlab.com');
  const [search, setSearch] = useState('');
  const [membershipOnly, setMembershipOnly] = useState(false);
  const [selectedProject, setSelectedProject] = useState<any>(null);
  const [branch, setBranch] = useState('');
  const [scanTypes, setScanTypes] = useState<ScanType[]>(['security', 'quality', 'secrets']);
  const [batchSize, setBatchSize] = useState(5);
  const [maxContext, setMaxContext] = useState(100000);

  const debouncedSearch = useDebounce(search, 300);

  const [gitlabValidate] = useGitlabValidateMutation();
  const { data: projectsData, isLoading: projectsLoading } = useGitlabListProjectsQuery(
    {
      token,
      gitlabUrl,
      search: debouncedSearch,
      membership: membershipOnly,
    },
    { skip: step !== 2 || !token }
  );

  const { data: branchesData, isLoading: branchesLoading } = useGitlabGetBranchesQuery(
    {
      id: selectedProject?.id,
      token,
      gitlabUrl,
    },
    { skip: step !== 3 || !selectedProject || !token }
  );

  const [gitlabScan, { isLoading: isScanning }] = useGitlabScanProjectMutation();

  const handleClose = () => {
    dispatch(closeModal());
    setStep(1);
    resetForm();
  };

  const resetForm = () => {
    setToken('');
    setGitlabUrl('https://gitlab.com');
    setSearch('');
    setSelectedProject(null);
    setBranch('');
  };

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const result = await gitlabValidate({ token, gitlab_url: gitlabUrl }).unwrap();
      if (result.valid) {
        setStep(2);
      } else {
        alert('Token 验证失败');
      }
    } catch (error) {
      alert('连接失败: ' + (error as Error).message);
    }
  };

  const handleSelectProject = (project: any) => {
    setSelectedProject(project);
    setStep(3);
  };

  const handleStartScan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProject) return;

    if (scanTypes.length === 0) {
      alert('请至少选择一种扫描类型');
      return;
    }

    try {
      const result = await gitlabScan({
        id: selectedProject.id,
        token,
        gitlab_url: gitlabUrl,
        project_id: selectedProject.id,
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

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title="从 GitLab 导入项目"
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
            placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
            type="password"
            fullWidth
            helperText="api 和 read_api 权限"
          />
          <Input
            label="GitLab URL"
            value={gitlabUrl}
            onChange={(e) => setGitlabUrl(e.target.value)}
            placeholder="https://gitlab.com"
            fullWidth
            helperText="私有部署请输入您的 GitLab 地址"
          />
        </form>
      )}

      {/* Step 2: Select Project */}
      {step === 2 && (
        <div>
          <div style={{ display: 'flex', gap: '16px', marginBottom: '16px' }}>
            <Input
              placeholder="搜索项目..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              fullWidth
            />
            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', whiteSpace: 'nowrap' }}>
              <Checkbox
                checked={membershipOnly}
                onChange={(e) => setMembershipOnly(e.target.checked)}
              />
              <span>只显示我的项目</span>
            </label>
          </div>
          <div style={{ maxHeight: '300px', overflowY: 'auto', border: '1px solid var(--color-border)', borderRadius: '6px' }}>
            {projectsLoading ? (
              <div style={{ padding: '32px', textAlign: 'center', color: 'var(--color-text-secondary)' }}>加载中...</div>
            ) : !projectsData?.projects?.length ? (
              <div style={{ padding: '32px', textAlign: 'center', color: 'var(--color-text-secondary)' }}>未找到项目</div>
            ) : (
              projectsData.projects.map((project) => (
                <div
                  key={project.id}
                  onClick={() => handleSelectProject(project)}
                  style={{
                    padding: '16px',
                    borderBottom: '1px solid var(--color-border)',
                    cursor: 'pointer',
                  }}
                  onMouseEnter={(e) => e.currentTarget.style.backgroundColor = 'var(--color-bg-tertiary)'}
                  onMouseLeave={(e) => e.currentTarget.style.backgroundColor = ''}
                >
                  <div style={{ fontWeight: 500, marginBottom: '4px' }}>{project.name_with_namespace}</div>
                  <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: '8px' }}>{project.path_with_namespace}</div>
                  <div style={{ display: 'flex', gap: '16px', fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <Star size={12} /> {project.star_count}
                    </span>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <GitFork size={12} /> {project.forks_count}
                    </span>
                    {project.archived && <Badge variant="default">已归档</Badge>}
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
            <strong>项目:</strong> {selectedProject?.name_with_namespace}
          </div>
          <div className="form-group">
            <label>分支</label>
            <select
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              disabled={branchesLoading}
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
                  {b.name} {b.default ? '(默认)' : ''}
                </option>
              ))}
            </select>
            {branchesLoading && (
              <small style={{ color: 'var(--color-text-secondary)' }}>加载分支列表中...</small>
            )}
          </div>
          <CheckboxGroup
            label="扫描类型"
            name="gitlabScanTypes"
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
