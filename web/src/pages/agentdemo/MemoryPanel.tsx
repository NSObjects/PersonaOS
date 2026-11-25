import { Alert, Button, Modal, Popconfirm, Space, Table, Typography, message } from 'antd';
import React, { useEffect, useState } from 'react';
import { type DemoMemoryFact, type DemoMemorySnapshot, deleteDemoMemoryFact, getDemoMemory } from '@/services/admin';

const labels: Record<string, string> = {
  favorite_color: '喜欢的颜色',
  favorite_animal: '喜欢的动物',
  favorite_game: '喜欢的游戏',
  favorite_story: '喜欢的故事',
  hobby: '爱好',
  likes: '喜欢',
  dislikes: '不喜欢',
};

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

  const remove = async (key: string) => {
    if (deleting || loading || snapshot?.extracting) return;
    setDeleting(true);
    try {
      await deleteDemoMemoryFact(deviceId, key);
      message.success('偏好记忆已删除');
    } catch {
      message.error('删除失败，请检查权限或等待后台记忆提取结束');
    } finally {
      setDeleting(false);
      setRevision((current) => current + 1);
    }
  };

  return (
    <Modal title={`长期记忆${snapshot ? ` · ${snapshot.character_name}` : ''}`} open onCancel={onClose} footer={null} width={720}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">当前登录用户与角色共享的偏好事实。相同角色的不同设备类型共用这些记忆；新建会话不会清除它们。</Typography.Text>
        <Alert type="info" showIcon message="删除偏好仅影响后续长期召回，不删除已归档对话，也不会移除当前会话的短期上下文。新的记忆对话可能再次学习该偏好。" />
        {snapshot?.extracting && <Alert type="warning" showIcon message="后台正在提取记忆，请稍后刷新。" />}
        {error && <Alert type="error" showIcon message="记忆读取失败，请检查权限和服务端连接。" />}
        <Button disabled={loading || deleting} onClick={() => setRevision((current) => current + 1)}>刷新记忆</Button>
        <Table<DemoMemoryFact> rowKey="key" size="small" pagination={false} loading={loading}
          dataSource={snapshot?.facts ?? []} locale={{ emptyText: error ? '读取失败' : '暂无已保存的偏好' }}
          columns={[
            { title: '偏好', dataIndex: 'key', render: (key: string) => labels[key] ?? key },
            { title: '内容', dataIndex: 'value' },
            { title: '更新时间', dataIndex: 'updated_at', render: (value: string) => new Date(value).toLocaleString() },
            { title: '操作', render: (_, item) => (
              <Popconfirm title="删除这条偏好记忆？" onConfirm={() => remove(item.key)}>
                <Button type="link" danger size="small" disabled={loading || deleting || snapshot?.extracting}>删除</Button>
              </Popconfirm>
            ) },
          ]} />
      </Space>
    </Modal>
  );
}
