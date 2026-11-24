import { Alert, Button, Empty, Input, Table, Tabs, theme, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import React, { useMemo, useState } from 'react';
import { duration, type ExecutionRecord } from './trajectoryModel';

type Props = {
  records: ExecutionRecord[];
  incomplete: boolean;
};

const labels: Record<string, string> = { context: '上下文', model: '模型', tool: '工具', audio: '语音' };
const colors: Record<string, string> = { context: '#8a96a6', model: '#527ac1', tool: '#b18b4f', audio: '#4d9b8b' };

function RecordStatus({ record }: { record: ExecutionRecord }) {
  if (record.error || record.interrupted) return <Typography.Text type="danger">{record.error || '未完成'}</Typography.Text>;
  if (record.end !== undefined) return <Typography.Text type="secondary">完成</Typography.Text>;
  return <Typography.Text type="secondary">运行中</Typography.Text>;
}

function Overview({ records, selected, onSelect }: {
  records: ExecutionRecord[]; selected?: string; onSelect: (key: string) => void;
}) {
  const { token } = theme.useToken();
  const times = records.flatMap((record) => [record.start, record.end].filter((time): time is number => time !== undefined));
  if (!times.length) return null;
  const origin = Math.min(...times);
  const span = Math.max(1, Math.max(...times) - origin);
  return (
    <section aria-label="执行时间概览" style={{ borderBottom: `1px solid ${token.colorBorderSecondary}`, padding: '12px 16px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
        <Typography.Text strong>时间概览</Typography.Text>
        <Typography.Text type="secondary">{span} ms</Typography.Text>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: token.colorTextSecondary }}>
        {[0, 0.25, 0.5, 0.75, 1].map((fraction) => <span key={fraction}>{Math.round(span * fraction)} ms</span>)}
      </div>
      <div style={{ position: 'relative', height: 82, borderTop: `1px solid ${token.colorBorderSecondary}`, overflow: 'hidden' }}>
        {[0, 25, 50, 75, 100].map((percent) => (
          <div key={percent} style={{ position: 'absolute', left: `${percent}%`, top: 0, bottom: 0, borderLeft: `1px solid ${token.colorBorderSecondary}` }} />
        ))}
        {records.slice(-18).map((record, index) => {
          if (record.start === undefined) return null;
          const elapsed = duration(record);
          return (
            <button key={record.key} type="button" aria-label={record.name} title={`${record.name} · ${elapsed ?? '运行中'} ms`}
              onClick={() => onSelect(record.key)}
              style={{
                position: 'absolute', left: `${(record.start - origin) / span * 100}%`,
                top: 4 + index % 6 * 13, width: elapsed === undefined ? 3 : `${elapsed / span * 100}%`,
                minWidth: 3, height: 9, padding: 0, borderRadius: 2,
                border: selected === record.key ? `1px solid ${token.colorText}` : 0,
                background: colors[record.kind] || token.colorTextSecondary, cursor: 'pointer',
              }} />
          );
        })}
      </div>
    </section>
  );
}

function DetailText({ value }: { value?: string }) {
  const { token } = theme.useToken();
  return value ? <pre style={{ margin: 0, padding: 12, fontSize: 12, lineHeight: 1.6, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', background: token.colorFillQuaternary, borderRadius: 4 }}>{value}</pre>
    : <Typography.Text type="secondary">暂无记录</Typography.Text>;
}

function Inspector({ record, close }: { record: ExecutionRecord; close: () => void }) {
  const { token } = theme.useToken();
  const elapsed = duration(record);
  return (
    <section aria-label="执行详情" style={{ minWidth: 0, flex: '1 1 300px', padding: '12px 16px', borderLeft: `1px solid ${token.colorBorderSecondary}`, overflow: 'auto' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography.Text strong>{record.name}</Typography.Text>
        <Button type="text" size="small" onClick={close}>关闭</Button>
      </div>
      <Tabs size="small" items={[
        { key: 'overview', label: '概览', children: (
          <div style={{ display: 'grid', gap: 12 }}>
            <RecordStatus record={record} />
            <div>类型：{labels[record.kind] || record.kind}</div>
            <div>耗时：{elapsed === undefined ? '—' : `${elapsed} ms`}</div>
            <div>输入 Token：{record.usage?.prompt_tokens ?? '—'}</div>
            <div>输出 Token：{record.usage?.completion_tokens ?? '—'}</div>
            <div>总 Token：{record.usage?.total_tokens ?? '—'}</div>
          </div>
        ) },
        { key: 'input', label: '输入', children: <DetailText value={record.input} /> },
        { key: 'output', label: '输出', children: <DetailText value={record.output} /> },
        { key: 'timing', label: '计时', children: (
          <div style={{ display: 'grid', gap: 12 }}>
            <div>开始：{record.start === undefined ? '—' : new Date(record.start).toISOString()}</div>
            <div>结束：{record.end === undefined ? '—' : new Date(record.end).toISOString()}</div>
            <div>持续时间：{elapsed === undefined ? '—' : `${elapsed} ms`}</div>
          </div>
        ) },
      ]} />
    </section>
  );
}

export default function Trajectory({ records, incomplete }: Props) {
  const [selected, setSelected] = useState<string>();
  const [query, setQuery] = useState('');
  const { token } = theme.useToken();
  const record = records.find((item) => item.key === selected);
  const filtered = useMemo(() => records.filter((item) =>
    [item.name, item.kind, item.input, item.output].some((value) => value?.toLowerCase().includes(query.toLowerCase())),
  ), [query, records]);
  const columns: ColumnsType<ExecutionRecord> = [
    { title: '轮次', dataIndex: 'turn', width: 64 },
    { title: '类型', dataIndex: 'kind', width: 76, render: (kind: string) => labels[kind] || kind },
    { title: '执行记录', dataIndex: 'name', ellipsis: true, render: (name: string, item) => (
      <Button type="link" size="small" style={{ padding: 0, maxWidth: '100%' }} onClick={() => setSelected(item.key)}>{name}</Button>
    ) },
    { title: '状态', width: 88, render: (_, item) => <RecordStatus record={item} /> },
    { title: 'Token', width: 72, align: 'right', render: (_, item) => item.usage?.total_tokens ?? '—' },
    { title: '耗时', width: 90, align: 'right', render: (_, item) => duration(item) === undefined ? '—' : `${duration(item)} ms` },
  ];
  if (!records.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="发送一条消息，查看 Agent 实时执行轨迹" />;
  return (
    <div style={{ border: `1px solid ${token.colorBorderSecondary}`, borderRadius: 6, minWidth: 0 }}>
      <Overview records={records} selected={selected} onSelect={setSelected} />
      {incomplete && <Alert type="warning" message="部分执行事件未送达，轨迹可能不完整" />}
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', padding: '8px 12px', borderBottom: `1px solid ${token.colorBorderSecondary}` }}>
        <Input.Search size="small" placeholder="搜索执行记录" value={query} onChange={(event) => setQuery(event.target.value)} style={{ maxWidth: 240 }} />
        <Typography.Text type="secondary">{filtered.length} 条记录</Typography.Text>
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'stretch' }}>
        <div style={{ flex: '3 1 550px', minWidth: 0 }}>
          <Table<ExecutionRecord> rowKey="key" size="small" columns={columns} dataSource={filtered} pagination={false} scroll={{ x: 550, y: 380 }} />
        </div>
        {record && <Inspector record={record} close={() => setSelected(undefined)} />}
      </div>
    </div>
  );
}
