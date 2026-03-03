'use client';

import React, { useState, useCallback, useMemo } from 'react';
import { Modal, Input, CheckboxGroup, Button } from '@/components/ui';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectActiveModal, closeModal } from '@/store/slices/uiSlice';
import { useCreateScanMutation } from '@/store/services/api';
import { selectScan, fetchScans } from '@/store/slices/scansSlice';
import type { ScanType } from '@/types/api';
import { getErrorMessage } from '@/lib/error';

const quickPaths = [
  'D:\\tmp\\code-auditor',
  'D:\\code',
  'C:\\Users\\admin\\code',
];

const scanTypeOptions: { value: ScanType; label: string; checked?: boolean }[] = [
  { value: 'security', label: '安全漏洞', checked: true },
  { value: 'quality', label: '代码质量', checked: true },
  { value: 'secrets', label: '敏感信息', checked: true },
  { value: 'compliance', label: '合规检查' },
];

export const LocalScanModal = React.memo(function LocalScanModal() {
  const dispatch = useAppDispatch();
  const isOpen = useAppSelector(selectActiveModal) === 'local-scan';
  const [createScan, { isLoading }] = useCreateScanMutation();

  const [repoPath, setRepoPath] = useState('');
  const [branch, setBranch] = useState('main');
  const [scanTypes, setScanTypes] = useState<ScanType[]>(['security', 'quality', 'secrets']);
  const [batchSize, setBatchSize] = useState(5);
  const [maxContext, setMaxContext] = useState(100000);

  const handleClose = useCallback(() => {
    dispatch(closeModal());
  }, [dispatch]);

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();

    if (!repoPath.trim()) {
      alert('请输入本地代码路径');
      return;
    }

    if (scanTypes.length === 0) {
      alert('请至少选择一种扫描类型');
      return;
    }

    try {
      const result = await createScan({
        repo_path: repoPath.trim(),
        branch: branch || 'main',
        scan_types: scanTypes,
        batch_size: batchSize,
        max_context: maxContext,
      }).unwrap();

      dispatch(selectScan(result.scan_id));
      dispatch(fetchScans({}));
      handleClose();
    } catch (error: unknown) {
      alert('创建扫描失败: ' + getErrorMessage(error));
    }
  }, [repoPath, branch, scanTypes, batchSize, maxContext, createScan, dispatch, handleClose]);

  const handleSetQuickPath = useCallback((path: string) => {
    setRepoPath(path);
  }, []);

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title="本地扫描"
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={handleClose}>
            取消
          </Button>
          <Button variant="primary" onClick={handleSubmit} loading={isLoading}>
            开始扫描
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit}>
        <Input
          label="本地代码路径"
          value={repoPath}
          onChange={(e) => setRepoPath(e.target.value)}
          placeholder="D:\code\my-project 或 /home/user/code/project"
          fullWidth
        />
        <div className="form-group">
          <label>常用路径</label>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
            {quickPaths.map((path) => (
              <Button
                key={path}
                variant="secondary"
                size="sm"
                onClick={() => handleSetQuickPath(path)}
              >
                {path}
              </Button>
            ))}
          </div>
        </div>
        <Input
          label="分支 (可选)"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          placeholder="main"
          helperText="如果是 Git 仓库，可以指定分支"
          fullWidth
        />
        <CheckboxGroup
          label="扫描类型"
          name="scanTypes"
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
});
