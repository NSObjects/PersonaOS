import { Alert, Button, Descriptions, Empty, Input, Modal, Space, Spin, Tag, Typography } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import {
  type CapabilityResource, type KnowledgeIndexStatus, type KnowledgeSearchResult,
  getKnowledgeIndexStatus, searchKnowledgeResource,
} from '@/services/admin';

const states = {
  indexed: '已索引',
  not_indexed: '未索引或索引丢失',
  unavailable: 'Redis Search 不可用',
  unconfigured: '向量服务未配置',
};

export default function KnowledgeInspection({ resource, onClose }: { resource: CapabilityResource; onClose: () => void }) {
  const [status, setStatus] = useState<KnowledgeIndexStatus>();
  const [statusFailed, setStatusFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  const [query, setQuery] = useState('');
  const [result, setResult] = useState<KnowledgeSearchResult>();
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState('');
  const searchRequest = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setStatus(undefined);
    setStatusFailed(false);
    setResult(undefined);
    setError('');
    void getKnowledgeIndexStatus(resource.id, controller.signal).then((value) => {
      if (!controller.signal.aborted) setStatus(value);
    }).catch(() => {
      if (!controller.signal.aborted) setStatusFailed(true);
    });
    return () => controller.abort();
  }, [resource.id, revision]);

  useEffect(() => () => searchRequest.current?.abort(), []);

  const search = async () => {
    if (status?.state !== 'indexed' || !query.trim() || searchRequest.current) return;
    const controller = new AbortController();
    searchRequest.current = controller;
    setSearching(true);
    setError('');
    setResult(undefined);
    try {
      const found = await searchKnowledgeResource(resource.id, query.trim(), controller.signal);
      if (controller.signal.aborted) return;
      if (found.version !== status.version) {
        setError('知识库版本已更新，请刷新索引状态后重试。');
        return;
      }
      setResult(found);
    } catch {
      if (!controller.signal.aborted) setError('检索失败，请检查 Redis Search、Embedding 配置及服务连通性。');
    } finally {
      if (searchRequest.current === controller) searchRequest.current = null;
      if (!controller.signal.aborted) setSearching(false);
    }
  };

  return (
    <Modal title={`知识库检索测试 · ${resource.name}`} open onCancel={onClose} footer={null} width={760}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">检查当前资源版本的实际索引分片，直接执行向量检索，不调用语言模型、不改变发布配置。</Typography.Text>
        <Spin spinning={!status && !statusFailed}>
          {statusFailed && <Alert type="error" showIcon message="索引状态读取失败，请检查权限和服务端连接。" />}
          {status && <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="当前版本">{status.version}</Descriptions.Item>
            <Descriptions.Item label="索引状态"><Tag color={status.state === 'indexed' ? 'success' : 'warning'}>{states[status.state]}</Tag></Descriptions.Item>
            <Descriptions.Item label="已索引分片">{status.state === 'indexed' || status.state === 'not_indexed' ? status.chunks : '—'}</Descriptions.Item>
          </Descriptions>}
        </Spin>
        <Button size="small" disabled={searching} onClick={() => setRevision((value) => value + 1)}>刷新索引状态</Button>
        <Input.TextArea aria-label="检索问题" placeholder="输入想查询的角色知识…" rows={3} maxLength={600} value={query} onChange={(event) => setQuery(event.target.value)} />
        <Button type="primary" loading={searching} disabled={status?.state !== 'indexed' || !query.trim()} onClick={() => { void search(); }}>测试检索</Button>
        {error && <Alert type="error" showIcon message={error} />}
        {result && (
          <>
            <Typography.Text strong>命中 {result.hits.length} 个分片</Typography.Text>
            {result.hits.length === 0 && <Empty description="未检索到分片" />}
            {result.hits.map((hit, index) => (
              <div key={hit.source} style={{ padding: 12, border: '1px solid var(--ant-color-border-secondary)', borderRadius: 6 }}>
                <Typography.Text type="secondary">片段 {index + 1} · {hit.source}</Typography.Text>
                <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>{hit.content}</Typography.Paragraph>
              </div>
            ))}
          </>
        )}
      </Space>
    </Modal>
  );
}
