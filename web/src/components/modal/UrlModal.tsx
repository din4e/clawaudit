'use client';

import React, { useState, useEffect } from 'react';
import { Modal, Input, Button, CheckboxGroup } from '@/components/ui';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectActiveModal, closeModal } from '@/store/slices/uiSlice';
import { useScanByUrlMutation, useLazyGetRemoteBranchesQuery } from '@/store/services/api';
import { selectScan, fetchScans } from '@/store/slices/scansSlice';
import type { ScanType } from '@/types/api';
import { Link } from 'lucide-react';

const scanTypeOptions: { value: ScanType; label: string; checked?: boolean }[] = [
  { value: 'security', label: '安全漏洞', checked: true },
  { value: 'quality', label: '代码质量', checked: true },
  { value: 'secrets', label: '敏感信息', checked: true },
  { value: 'compliance', label: '合规检查' },
];

export function UrlModal() {
  const dispatch = useAppDispatch();
  const isOpen = useAppSelector(selectActiveModal) === 'url';

  const [url, setUrl] = useState('');
  const [branch, setBranch] = useState('');
  const [scanTypes, setScanTypes] = useState<ScanType[]>(['security', 'quality', 'secrets']);
  const [batchSize, setBatchSize] = useState(5);
  const [maxContext, setMaxContext] = useState(100000);

  const [getBranches, { data: branchesData, isLoading: branchesLoading }] = useLazyGetRemoteBranchesQuery();
  const [scanByUrl, { isLoading: isScanning }] = useScanByUrlMutation();

  const branches = branchesData?.branches || [];

  const handleClose = () => {
    dispatch(closeModal());
  };

  const handleLoadBranches = () => {
    if (!url.trim()) {
      alert('请先输入仓库 URL');
      return;
    }
    getBranches({ url });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!url.trim()) {
      alert('请输入仓库 URL');
      return;
    }

    if (scanTypes.length === 0) {
      alert('请至少选择一种扫描类型');
      return;
    }

    try {
      const result = await scanByUrl({
        url: url.trim(),
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
      title="从 URL 导入项目"
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={handleClose}>取消</Button>
          <Button variant="primary" onClick={handleSubmit} loading={isScanning}>开始扫描</Button>
        </>
      }
    >
      <form onSubmit={handleSubmit}>
        <Input
          label="仓库 URL"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://github.com/owner/repo.git 或 git@github.com:owner/repo.git"
          fullWidth
          style={{ fontFamily: 'monospace' }}
          helperText="支持 GitHub、GitLab、Gitee 等平台的公开仓库（无需 token）"
        />
        <div className="form-group">
          <label>分支 (可选)</label>
          <select
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
            disabled={branchesLoading || branches.length === 0}
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
            {branches.map((b: string) => (
              <option key={b} value={b}>{b}</option>
            ))}
          </select>
          {branches.length === 0 && !branchesLoading && (
            <button
              type="button"
              onClick={handleLoadBranches}
              style={{
                marginTop: '4px',
                background: 'none',
                border: 'none',
              }}
            >
              <small style={{ color: 'var(--color-primary)', cursor: 'pointer' }}>
                点击获取分支列表
              </small>
            </button>
          )}
        </div>
        <CheckboxGroup
          label="扫描类型"
          name="urlScanTypes"
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
    </Modal>
  );
}
