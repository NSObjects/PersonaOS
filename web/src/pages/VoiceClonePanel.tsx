import { Alert, Button, Checkbox, Descriptions, Input, Modal, Space, Tag, Typography, Upload } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { type CapabilityResource, type VoiceCloneStatus, createVoiceClone, getVoiceCloneStatus, trainVoiceClone } from '@/services/admin';

const maxBytes = 10_000_000;
const states: Record<number, string> = { 0: '未训练', 2: '训练成功', 4: '已激活' };

export default function VoiceClonePanel({ initial, canTrain, onClose, onChanged }: {
  initial?: CapabilityResource; canTrain: boolean; onClose: () => void; onChanged: () => void;
}) {
  const [resource, setResource] = useState(initial);
  const [name, setName] = useState('');
  const [status, setStatus] = useState<VoiceCloneStatus>();
  const [file, setFile] = useState<File>();
  const [transcript, setTranscript] = useState('');
  const [authorized, setAuthorized] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const request = useRef<AbortController | null>(null);

  useEffect(() => () => request.current?.abort(), []);

  const perform = async (action: (signal: AbortSignal) => Promise<void>) => {
    if (request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError('');
    try {
      await action(controller.signal);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '请求失败');
    } finally {
      if (request.current === controller) request.current = null;
      if (!controller.signal.aborted) setBusy(false);
    }
  };

  const refresh = () => {
    if (!resource) return;
    void perform(async (signal) => {
      const next = await getVoiceCloneStatus(resource.id, signal);
      if (!signal.aborted) setStatus(next);
    });
  };

  const create = () => {
    void perform(async (signal) => {
      const next = await createVoiceClone(name.trim());
      if (!signal.aborted) { setResource(next); onChanged(); }
    });
  };

  const train = () => {
    if (!resource || !file || !authorized) return;
    void perform(async (signal) => {
      setStatus(undefined);
      const next = await trainVoiceClone(resource.id, file, transcript.trim(), authorized, signal);
      if (!signal.aborted) {
        setStatus(next);
        setFile(undefined);
        setAuthorized(false);
        onChanged();
      }
    });
  };

  return (
    <Modal title={resource ? `克隆音色 · ${resource.name}` : '新建克隆音色'} open
      onCancel={onClose} footer={null} width={720} maskClosable={!busy}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Alert type="info" showIcon message="Seed-TTS 3.0 · 中文复刻"
          description="当前使用后付费自定义音色槽位。请先开通对应服务；试听使用训练返回的音频，不会调用正式合成。" />
        {!resource ? (
          <>
            <Input aria-label="音色名称" placeholder="输入专属音色名称" value={name}
              maxLength={50} onChange={(event) => setName(event.target.value)} />
            <Button type="primary" loading={busy} disabled={!name.trim()} onClick={create}>创建克隆音色</Button>
          </>
        ) : (
          <>
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="自定义音色 ID">{resource.reference}</Descriptions.Item>
              <Descriptions.Item label="训练状态">
                {status ? <Tag color={status.model_ready ? 'success' : 'default'}>{states[status.status] ?? `未知状态 ${status.status}`}</Tag> : '尚未查询'}
              </Descriptions.Item>
              <Descriptions.Item label="剩余训练次数">{status?.available_training_times ?? '—'}</Descriptions.Item>
            </Descriptions>
            <Button disabled={busy} onClick={refresh}>刷新训练状态</Button>
            {status?.demo_audio && (
              <>
                <Typography.Text>厂商训练试听（有效期约 1 小时，过期请刷新）</Typography.Text>
                {/* biome-ignore lint/a11y/useMediaCaption: This is the provider's reference audition, with no transcript supplied. */}
                <audio controls src={status.demo_audio} aria-label="克隆音色试听" style={{ width: '100%' }} />
              </>
            )}
            {canTrain && !resource.enabled && status?.status !== 4 && (
              <>
                <Upload accept=".wav,.mp3" maxCount={1} disabled={busy}
                  beforeUpload={(audio) => {
                    if (audio.size > maxBytes || audio.size === 0 || !/\.(wav|mp3)$/i.test(audio.name)) {
                      setError('请选择 10 MB 以内的 WAV 或 MP3'); return Upload.LIST_IGNORE;
                    }
                    setFile(audio); setError(''); setAuthorized(false); return false;
                  }}
                  fileList={file ? [{ uid: 'clone-sample', name: file.name }] : []}
                  onRemove={() => { setFile(undefined); setAuthorized(false); }}>
                  <Button disabled={busy}>上传授权音频</Button>
                </Upload>
                <Typography.Text type="secondary">建议单人清晰录音，10–60 秒，无背景音乐。实际音频格式和时长由厂商验证。</Typography.Text>
                <Input.TextArea aria-label="录音原文" placeholder="录音原文（可选，须与实际录音一致）" rows={3}
                  value={transcript} maxLength={1000} disabled={busy} onChange={(event) => setTranscript(event.target.value)} />
                <Checkbox checked={authorized} disabled={busy} onChange={(event) => setAuthorized(event.target.checked)}>
                  我已获得声音本人或相关权利方授权，允许将此录音提交给火山引擎进行声音复刻。
                </Checkbox>
                <Button type="primary" loading={busy} disabled={!file || !authorized} onClick={train}>提交训练</Button>
              </>
            )}
            <Alert type="warning" message="正式启用暂未开放，待 Seed-TTS 3.0 合成适配完成。训练成功不会自动激活音色或绑定设备类型。" />
          </>
        )}
        {error && <Alert type="error" showIcon message={error}
          description="若训练请求中断，请先刷新状态，确认结果后再决定是否重新训练。" />}
      </Space>
    </Modal>
  );
}
