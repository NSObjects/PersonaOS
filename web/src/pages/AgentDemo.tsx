import { Bubble, Sender } from '@ant-design/x';
import { PageContainer } from '@ant-design/pro-components';
import { Alert, Button, Card, Empty, Select, Space, Spin, Switch, Tag, Typography, message } from 'antd';
import React, { useEffect, useRef, useState } from 'react';

import {
  type ActiveDeviceConfig, type DemoChatTurn, type DeviceType,
  chatWithCharacter, getActiveDeviceConfig, listDeviceTypes,
  recognizeDemoSpeech, streamCharacterSpeech, synthesizeDemoSpeech,
} from '@/services/admin';
import { type OpusPlayback, startOpusPlayback } from '@/utils/opusPlayback';
import { type RecordingSession, startOpusRecording } from '@/utils/voiceRecorder';

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
  const [recording, setRecording] = useState(false);
  const [audioURL, setAudioURL] = useState('');
  const recorder = useRef<RecordingSession | null>(null);
  const captureRequest = useRef<AbortController | null>(null);
  const recordingTimer = useRef<number | null>(null);
  const lastAudioURL = useRef('');
  const playback = useRef<OpusPlayback | null>(null);
  const [playing, setPlaying] = useState(false);

  useEffect(() => () => {
    captureRequest.current?.abort();
    playback.current?.cancel();
    recorder.current?.cancel();
    pending.current?.abort();
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    if (lastAudioURL.current) URL.revokeObjectURL(lastAudioURL.current);
  }, []);

  useEffect(() => {
    void listDeviceTypes().then((items) => {
      setDeviceTypes(items);
      if (items.length > 0) setDeviceTypeId(items[0].id);
    }).catch(() => message.error('获取设备类型失败'));
  }, []);

  useEffect(() => {
    captureRequest.current?.abort();
    captureRequest.current = null;
    playback.current?.cancel();
    playback.current = null;
    setPlaying(false);
    pending.current?.abort();
    pending.current = null;
    recorder.current?.cancel();
    recorder.current = null;
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    recordingTimer.current = null;
    setRecording(false);
    if (lastAudioURL.current) URL.revokeObjectURL(lastAudioURL.current);
    lastAudioURL.current = '';
    setAudioURL('');
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

  const completeTurn = async (input: string, speak: boolean, controller: AbortController) => {
    const content = input.trim();
    if (!content || !deviceTypeId || !config || controller.signal.aborted) return;
    const previous: DemoChatTurn[] = history.slice(-20).map((turn) => ({
      role: turn.role, content: turn.content,
    }));
    nextMessageID.current += 1;
    setHistory((current) => [...current, {
      key: `u-${nextMessageID.current}`, role: 'user', content,
    }]);
    setValue('');
    const player = speak ? playback.current : null;
    const response = player
      ? await streamCharacterSpeech(deviceTypeId, content, previous, (chunk) => {
        player.append(chunk);
        setPlaying(true);
      }, controller.signal, remember, memorySession.current)
      : await chatWithCharacter(
        deviceTypeId, content, previous, controller.signal, remember, memorySession.current,
      );
    if (player) await player.finish();
    if (controller.signal.aborted) return;
    if (remember && !response.memory_queued) {
      message.warning('当前记忆队列繁忙，本轮对话未进入长期记忆');
    }
    nextMessageID.current += 1;
    setHistory((current) => [...current, {
      key: `a-${nextMessageID.current}`, role: 'assistant', content: response.reply,
    }]);
    if (speak && config.voice && !player) {
      // Voice comes from the server-side applied configuration, never the browser.
      const spokenAudio = await synthesizeDemoSpeech(deviceTypeId, response.reply, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(spokenAudio);
      if (lastAudioURL.current) URL.revokeObjectURL(lastAudioURL.current);
      lastAudioURL.current = url;
      setAudioURL(url);
    }
  };

  const runBusy = async (work: (controller: AbortController) => Promise<void>) => {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setSending(true);
    try {
      await work(controller);
    } catch {
      playback.current?.cancel();
      playback.current = null;
      setPlaying(false);
      if (!controller.signal.aborted) message.error('语音或对话请求失败，请检查服务端配置');
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        setSending(false);
      }
    }
  };

  const send = async (text: string) => {
    if (!text.trim() || text.length > 2000 || !config || recording) return;
    await runBusy((controller) => completeTurn(text, false, controller));
  };

  const stopRecording = async () => {
    const session = recorder.current;
    if (!session) return;
    recorder.current = null;
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    recordingTimer.current = null;
    setRecording(false);
    await runBusy(async (controller) => {
      const opus = await session.stop();
      const transcript = await recognizeDemoSpeech(opus, controller.signal);
      await completeTurn(transcript, true, controller);
    });
  };

  const startRecording = async () => {
    if (!config || pending.current || recorder.current || captureRequest.current) return;
    const controller = new AbortController();
    captureRequest.current = controller;
    try {
      playback.current?.cancel();
      playback.current = null;
      setPlaying(false);
      if (lastAudioURL.current) URL.revokeObjectURL(lastAudioURL.current);
      lastAudioURL.current = '';
      setAudioURL('');
      if (config.voice?.provider === 'aliyun' || config.voice?.provider === 'volcengine') {
        const player = await startOpusPlayback();
        if (controller.signal.aborted) {
          player.cancel();
          return;
        }
        playback.current = player;
      }
      const session = await startOpusRecording(controller.signal);
      if (controller.signal.aborted || captureRequest.current !== controller || pending.current) {
        session.cancel();
        return;
      }
      recorder.current = session;
      setRecording(true);
      recordingTimer.current = window.setTimeout(() => { void stopRecording(); }, 14500);
    } catch {
      if (captureRequest.current === controller) {
        playback.current?.cancel();
        playback.current = null;
      }
      if (!controller.signal.aborted) {
        message.error('Opus 录音或播放不可用，请使用支持 WebCodecs 的浏览器并检查麦克风授权');
      }
    } finally {
      if (captureRequest.current === controller) captureRequest.current = null;
    }
  };

  return (
    <PageContainer title="AI 角色对话" subTitle="Ant Design X · Eino Agent">
      <Card style={{ maxWidth: 980, margin: '0 auto' }}>
        <Space wrap style={{ width: '100%', marginBottom: 16 }}>
          <Typography.Text strong>设备类型</Typography.Text>
          <Select<number> placeholder="选择后台配置的设备类型"
            style={{ width: 260 }} value={deviceTypeId}
            onChange={(id) => { captureRequest.current?.abort(); setDeviceTypeId(id); }}
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
            <Space wrap>
              <Button type={recording ? 'primary' : 'default'} danger={recording}
                disabled={!config || sending}
                onClick={() => { void (recording ? stopRecording() : startRecording()); }}>
                {recording ? '结束录音并发送' : '按键录音'}
              </Button>
              {playing && <Button onClick={() => {
                pending.current?.abort();
                playback.current?.cancel();
                playback.current = null;
                setPlaying(false);
              }}>停止语音</Button>}
              {recording && <Typography.Text type="secondary">录音中（最多 15 秒）</Typography.Text>}
              {/* biome-ignore lint/a11y/useMediaCaption: The exact spoken response is already visible in the conversation bubble. */}
              {audioURL && <audio src={audioURL} controls autoPlay aria-label="角色语音回复" />}
            </Space>
            <Sender placeholder="对角色说点什么…" value={value}
              onChange={setValue} loading={sending}
              disabled={!config || loadingConfig || recording}
              onSubmit={(text) => { void send(text); }}
              onCancel={() => { pending.current?.abort(); playback.current?.cancel(); }}
              autoSize={{ minRows: 2, maxRows: 4 }} />
          </div>
        </Spin>
      </Card>
    </PageContainer>
  );
};

export default AgentDemo;
