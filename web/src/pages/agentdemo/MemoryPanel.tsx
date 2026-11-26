import { Alert, Button, Modal, Popconfirm, Space, Typography, message } from 'antd';
import React, { useEffect, useState } from 'react';
import { type DemoMemorySnapshot, clearDemoMemory, getDemoMemory } from '@/services/admin';

export default function MemoryPanel({ deviceId, onClose }: { deviceId: number; onClose: () => void }) {
  const [snapshot, setSnapshot] = useState<DemoMemorySnapshot>();
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(false);
    void getDemoMemory(deviceId, controller.signal).then((value) => {
      if (!controller.signal.aborted) setSnapshot(value);
    }).catch(() => {
      if (!controller.signal.aborted) { setSnapshot(undefined); setError(true); }
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [deviceId, revision]);

  const clear = async () => {
    if (deleting || loading || snapshot?.extracting) return;
    setDeleting(true);
    try {
      await clearDemoMemory(deviceId);
      message.success('长期记忆已清除');
    } catch {
      message.error('清除失败，请检查权限或等待当前记忆保存结束');
    } finally {
      setDeleting(false);
      setRevision((current) => current + 1);
    }
  };

  return (
    <Modal title={`长期记忆${snapshot ? ` · ${snapshot.character_name}` : ''}`} open onCancel={onClose} footer={null} width={720}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">当前登录用户与角色共享的长期记忆。相同角色的不同设备类型共用这些记忆；新建会话不会清除它们。</Typography.Text>
        <Alert type="info" showIcon message="清除长期记忆同时取消已排队的整理内容，不删除对话档案或当前会话上下文。新对话仍可形成新记忆。" />
        {snapshot?.extracting && <Alert type="warning" showIcon message="后台正在保存记忆，请稍后刷新。" />}
        {error && <Alert type="error" showIcon message="记忆读取失败，请检查权限和服务端连接。" />}
        <Button disabled={loading || deleting} onClick={() => setRevision((current) => current + 1)}>刷新记忆</Button>
        {snapshot && (
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            <Typography.Text strong>长期偏好与约定</Typography.Text>
            <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
              {snapshot.stable || '暂无长期记忆'}
            </Typography.Paragraph>
            <Typography.Text strong>近期目标与进展</Typography.Text>
            <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
              {snapshot.dynamic || '暂无近期记忆'}
            </Typography.Paragraph>
            <Popconfirm title="清除该角色的全部长期记忆及待整理内容？" onConfirm={clear}>
              <Button danger disabled={loading || deleting || snapshot.extracting}>清除长期记忆</Button>
            </Popconfirm>
          </Space>
        )}
      </Space>
    </Modal>
  );
}
