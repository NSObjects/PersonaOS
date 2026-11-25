import { Alert, Button, Descriptions, Input, Modal, Space, Typography } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { type CapabilityResource, previewVoiceResource } from '@/services/admin';

const defaultText = '你好呀！很高兴见到你，今天我们一起聊点有趣的事情吧。';

export default function VoicePreview({ resource, onClose }: { resource: CapabilityResource; onClose: () => void }) {
  const [text, setText] = useState(defaultText);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [audioURL, setAudioURL] = useState('');
  const request = useRef<AbortController | null>(null);
  const url = useRef('');

  useEffect(() => () => {
    request.current?.abort();
    if (url.current) URL.revokeObjectURL(url.current);
  }, []);

  const preview = async () => {
    if (request.current || !text.trim() || new TextEncoder().encode(text.trim()).length > 600) return;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError('');
    if (url.current) URL.revokeObjectURL(url.current);
    url.current = '';
    setAudioURL('');
    try {
      const audio = await previewVoiceResource(resource.id, text.trim(), controller.signal);
      if (controller.signal.aborted) return;
      url.current = URL.createObjectURL(audio);
      setAudioURL(url.current);
    } catch {
      if (!controller.signal.aborted) setError('试听失败，请检查音色 ID、厂商凭据及服务连通性。');
    } finally {
      if (request.current === controller) request.current = null;
      if (!controller.signal.aborted) setBusy(false);
    }
  };

  const cancel = () => {
    request.current?.abort();
    request.current = null;
    setBusy(false);
  };

  const byteCount = new TextEncoder().encode(text.trim()).length;
  return (
    <Modal title={`音色试听 · ${resource.name}`} open onCancel={onClose} footer={null} width={620}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="语音厂商">{resource.provider}</Descriptions.Item>
          <Descriptions.Item label="音色 ID">{resource.reference}</Descriptions.Item>
        </Descriptions>
        <Typography.Text type="secondary">直接使用当前已保存音色调用 TTS，不需要绑定设备类型或应用配置。试听会产生厂商合成费用。</Typography.Text>
        <Input.TextArea aria-label="试听文本" value={text} rows={4} maxLength={600} onChange={(event) => setText(event.target.value)} />
        <Typography.Text type={byteCount > 600 ? 'danger' : 'secondary'}>{byteCount} / 600 字节</Typography.Text>
        <Space>
          <Button type="primary" loading={busy} disabled={!text.trim() || byteCount > 600} onClick={() => { void preview(); }}>生成试听</Button>
          {busy && <Button onClick={cancel}>取消生成</Button>}
        </Space>
        {error && <Alert type="error" showIcon message={error} />}
        {/* biome-ignore lint/a11y/useMediaCaption: The exact synthesis text is shown in the input above. */}
        {audioURL && <audio src={audioURL} controls autoPlay aria-label="音色试听结果" style={{ width: '100%' }} />}
      </Space>
    </Modal>
  );
}
