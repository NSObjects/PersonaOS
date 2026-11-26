import { Bubble } from '@ant-design/x';
import { Alert, Button, Empty, Space, Tabs, Typography } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { readDemoSessionEvents } from '@/services/admin';
import { emptySessionProjection, projectSessionEvents } from './sessionProjection';
import Trajectory from './Trajectory';

const roles = {
  user: { placement: 'end' as const },
  assistant: { placement: 'start' as const },
};

// This component never sends a chat request or owns the sender's audio session.
export default function SessionObserver({ sessionId }: { sessionId: string }) {
  const projection = useRef(emptySessionProjection());
  const [snapshot, setSnapshot] = useState(projection.current);
  const [connected, setConnected] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState('');
  const [view, setView] = useState('chat');

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    const run = async () => {
      try {
        while (!controller.signal.aborted) {
          const batch = await readDemoSessionEvents(sessionId, projection.current.cursor, controller.signal);
          if (controller.signal.aborted) return;
          projection.current = projectSessionEvents(projection.current, batch);
          setSnapshot(projection.current);
          setConnected(true);
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        setError(cause instanceof Error ? cause.message : '会话连接中断');
        setConnected(false);
        setEnabled(false);
      }
    };
    void run();
    return () => controller.abort();
  }, [sessionId, enabled]);

  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Space wrap>
        <Typography.Text strong>只读会话观测</Typography.Text>
        <Typography.Text type="secondary">{!enabled ? '已断开' : connected ? '已连接' : '连接中'}</Typography.Text>
        {enabled ? (
          <Button onClick={() => { setEnabled(false); setConnected(false); }}>断开观测</Button>
        ) : (
          <Button onClick={() => { setError(''); setEnabled(true); }}>重新连接</Button>
        )}
      </Space>
      {error && <Alert type="warning" showIcon message={error} description="请确认发送端已创建会话、当前账号和角色具有访问权限，再重新连接。" />}
      {snapshot.incomplete && (view === 'chat' || !snapshot.records.length) && <Alert type="warning" showIcon message="部分历史事件已淘汰，当前显示可能不完整" />}
      {snapshot.failedTurn !== undefined && <Alert type="warning" message={`第 ${snapshot.failedTurn} 轮执行失败`} />}
      <Tabs activeKey={view} onChange={setView} items={[{ key: 'chat', label: '对话' }, { key: 'trajectory', label: '执行轨迹' }]} />
      {view === 'trajectory' ? (
        <Trajectory records={snapshot.records} messages={snapshot.messages} incomplete={snapshot.incomplete} />
      ) : (
        <div style={{ minHeight: 380, maxHeight: 540, overflow: 'auto' }}>
          {snapshot.messages.length > 0 ? <Bubble.List items={snapshot.messages} role={roles} /> :
            <Empty description="等待发送端发起对话" />}
          {snapshot.activeTurn !== undefined && <Typography.Text type="secondary">第 {snapshot.activeTurn} 轮处理中…</Typography.Text>}
        </div>
      )}
    </Space>
  );
}
