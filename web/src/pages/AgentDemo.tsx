import { Bubble, Sender } from '@ant-design/x';
import { PageContainer } from '@ant-design/pro-components';
import { Alert, Card, Empty, Select, Space, Spin, Switch, Tag, Typography, message } from 'antd';
import React, { useEffect, useRef, useState } from 'react';

import {
  type ActiveDeviceConfig, type DemoChatTurn, type DeviceType,
  chatWithCharacter, getActiveDeviceConfig, listDeviceTypes,
} from '@/services/admin';

type ChatBubble = {
  key: string;
  role: 'user' | 'assistant';
  content: string;
};

const bubbleRoles = {
  user: { placement: 'end' as const },
  assistant: { placement: 'start' as const },
};

// The Demo chat deliberately uses the existing admin login and confirmed
// device-type configuration. It is not an unauthenticated device interface.
const AgentDemo: React.FC = () => {
  const [deviceTypes, setDeviceTypes] = useState<DeviceType[]>([]);
  const [deviceTypeId, setDeviceTypeId] = useState<number>();
  const [config, setConfig] = useState<ActiveDeviceConfig>();
  const [loadingConfig, setLoadingConfig] = useState(false);
  const [history, setHistory] = useState<ChatBubble[]>([]);
  const [value, setValue] = useState('');
  const [sending, setSending] = useState(false);
  const [remember, setRemember] = useState(false);
  const memorySession = useRef('');
  const pending = useRef<AbortController | null>(null);
  const nextMessageID = useRef(0);

  useEffect(() => {
    void listDeviceTypes().then((items) => {
      setDeviceTypes(items);
      if (items.length > 0) setDeviceTypeId(items[0].id);
    }).catch(() => message.error('获取设备类型失败'));
  }, []);

  useEffect(() => {
    pending.current?.abort();
    pending.current = null;
    setSending(false);
    setConfig(undefined);
    setHistory([]);
    setValue('');
    setRemember(false);
    memorySession.current = crypto.randomUUID();
    if (!deviceTypeId) return;
    let active = true;
    setLoadingConfig(true);
    void getActiveDeviceConfig(deviceTypeId)
      .then((current) => { if (active) setConfig(current); })
      .catch(() => { if (active) setConfig(undefined); })
      .finally(() => { if (active) setLoadingConfig(false); });
    return () => { active = false; };
  }, [deviceTypeId]);

  const send = async (text: string) => {
    const content = text.trim();
    if (!content || content.length > 2000 || !deviceTypeId || !config || sending) return;
    const id = deviceTypeId;
    const previous: DemoChatTurn[] = history.slice(-20).map((turn) => ({
      role: turn.role === 'assistant' ? 'assistant' : 'user',
      content: turn.content,
    }));
    nextMessageID.current += 1;
    setHistory((current) => [...current, {
      key: `u-${nextMessageID.current}`, role: 'user', content,
    }]);
    setValue('');
    setSending(true);
    const controller = new AbortController();
    pending.current = controller;
    try {
      const response = await chatWithCharacter(id, content, previous, controller.signal, remember, memorySession.current);
      if (controller.signal.aborted) return;
      if (remember && !response.memory_queued) {
        message.warning('当前记忆队列繁忙，本轮对话未进入长期记忆');
      }
      nextMessageID.current += 1;
      setHistory((current) => [...current, {
        key: `a-${nextMessageID.current}`, role: 'assistant', content: response.reply,
      }]);
    } catch {
      if (!controller.signal.aborted) message.error('对话请求失败，请检查模型配置和网络');
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        setSending(false);
      }
    }
  };

  return (
    <PageContainer title="AI 角色对话" subTitle="Ant Design X · Eino Agent">
      <Card style={{ maxWidth: 980, margin: '0 auto' }}>
        <Space wrap style={{ width: '100%', marginBottom: 16 }}>
          <Typography.Text strong>设备类型</Typography.Text>
          <Select<number> placeholder="选择后台配置的设备类型"
            style={{ width: 260 }} value={deviceTypeId}
            onChange={setDeviceTypeId}
            options={deviceTypes.map((item) => ({
              value: item.id, label: `${item.name} (${item.code})`,
            }))} />
          {config && <Tag color="success">已应用 · {config.character.name}</Tag>}
          <Space size="small">
            <Switch checked={remember} disabled={sending || !config}
              onChange={setRemember} aria-label="启用偏好记忆" />
            <Typography.Text type="secondary">偏好记忆（主动开启，仅供 Demo）</Typography.Text>
          </Space>
        </Space>
        <Spin spinning={loadingConfig}>
          {deviceTypes.length === 0 && (
            <Empty description="请先在后台创建角色、设备类型，并应用配置" />
          )}
          {deviceTypeId && !loadingConfig && !config && (
            <Alert type="warning" showIcon
              message="该设备类型还没有生效配置"
              description="请在设备类型页面保存能力草稿，再点击「应用配置」。" />
          )}
          <div style={{ height: 460, display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
              {history.length > 0 ? (
                <Bubble.List items={history} role={bubbleRoles} />
              ) : (
                <Empty style={{ paddingTop: 90 }}
                  description={config ? `可以开始和「${config.character.name}」对话` : '选择已应用配置的设备类型'} />
              )}
            </div>
            <Sender placeholder="对角色说点什么…" value={value}
              onChange={setValue} loading={sending} disabled={!config || loadingConfig}
              onSubmit={(text) => { void send(text); }}
              onCancel={() => pending.current?.abort()}
              autoSize={{ minRows: 2, maxRows: 4 }} />
          </div>
        </Spin>
      </Card>
    </PageContainer>
  );
};

export default AgentDemo;
