import { Bubble, Sender } from '@ant-design/x';
import { PageContainer } from '@ant-design/pro-components';
import { useLocation } from '@umijs/max';
import { Alert, Button, Card, Empty, Select, Space, Spin, Switch, Tag, Tabs, Typography, message } from 'antd';
import React, { useEffect, useRef, useState } from 'react';

import {
  type ActiveDeviceConfig, type DeviceType, type DemoASRSession, type DemoExecutionEvent, type DemoRuntimeStatus,
  getActiveDeviceConfig, getDemoRuntimeStatus, listDeviceTypes,
  openDemoASR, recognizeDemoSpeech, streamCharacterSpeech, synthesizeDemoSpeech,
} from '@/services/admin';
import { type OpusPlayback, startOpusPlayback } from '@/utils/opusPlayback';
import { type RecordingSession, startOpusRecording } from '@/utils/voiceRecorder';
import MemoryPanel from './agentdemo/MemoryPanel';
import SessionObserver from './agentdemo/SessionObserver';
import { markReplyIncomplete, type SessionMessage, upsertReply } from './agentdemo/sessionProjection';
import Trajectory from './agentdemo/Trajectory';
import { applyExecutionEvent, type ExecutionRecord, observeBrowserStage, settleExecutionTurn } from './agentdemo/trajectoryModel';

const bubbleRoles = {
  user: { placement: 'end' as const },
  assistant: { placement: 'start' as const },
};

// The Demo chat deliberately uses the existing admin login and confirmed
// device-type configuration. It is not an unauthenticated device interface.
const AgentChat: React.FC<{ initialDeviceId?: number }> = ({ initialDeviceId }) => {
  const [deviceTypes, setDeviceTypes] = useState<DeviceType[]>([]);
  const [deviceTypeId, setDeviceTypeId] = useState<number>();
  const [config, setConfig] = useState<ActiveDeviceConfig>();
  const [loadingConfig, setLoadingConfig] = useState(false);
  const [configError, setConfigError] = useState(false);
  const [sessionGeneration, setSessionGeneration] = useState(0);
  const [runtimeStatus, setRuntimeStatus] = useState<DemoRuntimeStatus>();
  const [statusError, setStatusError] = useState(false);
  const [history, setHistory] = useState<SessionMessage[]>([]);
  const [records, setRecords] = useState<ExecutionRecord[]>([]);
  const [incomplete, setIncomplete] = useState(false);
  const [view, setView] = useState('chat');
  const [value, setValue] = useState('');
  const [sending, setSending] = useState(false);
  const [remember, setRemember] = useState(false);
  const [memoryOpen, setMemoryOpen] = useState(false);
  const conversationSession = useRef('');
  const pending = useRef<AbortController | null>(null);
  const nextTurnID = useRef(0);
  const [recording, setRecording] = useState(false);
  const [audioURL, setAudioURL] = useState('');
  const recorder = useRef<RecordingSession | null>(null);
  const liveASR = useRef<DemoASRSession | null>(null);
  const captureRequest = useRef<AbortController | null>(null);
  const recordingTimer = useRef<number | null>(null);
  const lastAudioURL = useRef('');
  const playback = useRef<OpusPlayback | null>(null);
  const [playing, setPlaying] = useState(false);

  useEffect(() => () => {
    captureRequest.current?.abort();
    liveASR.current?.cancel();
    playback.current?.cancel();
    recorder.current?.cancel();
    pending.current?.abort();
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    if (lastAudioURL.current) URL.revokeObjectURL(lastAudioURL.current);
  }, []);

  useEffect(() => {
    void listDeviceTypes().then((items) => {
      setDeviceTypes(items);
      if (initialDeviceId !== undefined) {
        if (items.some((item) => item.id === initialDeviceId)) setDeviceTypeId(initialDeviceId);
        else message.error('指定设备类型不存在或无访问权限');
      } else if (items.length > 0) setDeviceTypeId(items[0].id);
    }).catch(() => message.error('获取设备类型失败'));
  }, [initialDeviceId]);

  const refreshStatus = () => {
    setStatusError(false);
    void getDemoRuntimeStatus().then(setRuntimeStatus).catch(() => { setRuntimeStatus(undefined); setStatusError(true); });
  };

  useEffect(() => { refreshStatus(); }, []);
  useEffect(() => { setRemember(false); }, [deviceTypeId]);

  useEffect(() => {
    captureRequest.current?.abort();
    captureRequest.current = null;
    liveASR.current?.cancel();
    liveASR.current = null;
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
    setRecords([]);
    setIncomplete(false);
    setValue('');
    setConfigError(false);
    setMemoryOpen(false);
    nextTurnID.current = 0;
    setView('chat');
    conversationSession.current = crypto.randomUUID();
    if (!deviceTypeId) return;
    let active = true;
    setLoadingConfig(true);
    void getActiveDeviceConfig(deviceTypeId)
      .then((current) => { if (active) setConfig(current); })
      .catch((error) => {
        if (active) {
          setConfig(undefined);
          setConfigError(error?.response?.status !== 404);
        }
      })
      .finally(() => { if (active) setLoadingConfig(false); });
    return () => { active = false; };
  }, [deviceTypeId, sessionGeneration]);

  const recordBrowserEvent = (turn: number, controller: AbortController, event: DemoExecutionEvent) => {
    if (pending.current === controller) {
      setRecords((current) => applyExecutionEvent(current, turn, event, 'browser'));
    }
  };

  const completeTurn = async (input: string, speak: boolean, controller: AbortController, turn: number) => {
    const content = input.trim();
    if (!content || !deviceTypeId || !config || controller.signal.aborted) return;
    setHistory((current) => [...current, {
      key: `u-${turn}`, role: 'user', content,
    }]);
    setValue('');
    const player = speak ? playback.current : null;
    const response = await streamCharacterSpeech(deviceTypeId, content, [],
      (chunk) => player?.append(chunk), controller.signal, remember, conversationSession.current, {
        voice: Boolean(player),
        onEvent: (event) => {
          if (!controller.signal.aborted) setRecords((current) => applyExecutionEvent(current, turn, event));
        },
        onPreview: (text) => {
          if (!controller.signal.aborted) setHistory((current) => upsertReply(current, turn, text));
        },
        onGap: () => { if (!controller.signal.aborted) setIncomplete(true); },
      }).catch((error) => {
        if (pending.current === controller) setHistory((current) => markReplyIncomplete(current, turn));
        throw error;
      }).finally(() => {
        if (!controller.signal.aborted || pending.current === controller) {
          setRecords((current) => settleExecutionTurn(current, turn));
        }
      });
    if (controller.signal.aborted) return;
    setHistory((current) => upsertReply(current, turn, response.reply));
    if (player) await player.finish();
    if (controller.signal.aborted) return;
    if (remember && !response.memory_queued) {
      message.warning('当前记忆队列繁忙，本轮对话未进入长期记忆');
    }
    if (speak && config.voice && !player) {
      // Voice comes from the server-side applied configuration, never the browser.
      const spokenAudio = await observeBrowserStage(-2, '批量 TTS 请求（浏览器）',
        (event) => recordBrowserEvent(turn, controller, event),
        () => synthesizeDemoSpeech(deviceTypeId, response.reply, controller.signal));
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
      if (pending.current === controller) {
        playback.current?.cancel();
        playback.current = null;
        setPlaying(false);
      }
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
    await runBusy((controller) => completeTurn(text, false, controller, ++nextTurnID.current));
  };

  const stopRecording = async () => {
    const session = recorder.current;
    if (!session) return;
    recorder.current = null;
    if (recordingTimer.current !== null) window.clearTimeout(recordingTimer.current);
    recordingTimer.current = null;
    setRecording(false);
    await runBusy(async (controller) => {
      const turn = ++nextTurnID.current;
      const live = liveASR.current;
      const cancel = () => live?.cancel();
      controller.signal.addEventListener('abort', cancel, { once: true });
      try {
        const transcript = await observeBrowserStage(-1, 'ASR 结束等待（浏览器）',
          (event) => recordBrowserEvent(turn, controller, event), async () => {
            const opus = await session.stop();
            const text = live ? await live.finish()
              : opus ? await recognizeDemoSpeech(opus, controller.signal) : '';
            if (!text) throw new Error('Missing transcript');
            return text;
          });
        await completeTurn(transcript, true, controller, turn);
      } finally {
        controller.signal.removeEventListener('abort', cancel);
        live?.cancel();
        if (liveASR.current === live) liveASR.current = null;
      }
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
      if (config.voice?.provider === 'aliyun' || config.voice?.provider === 'volcengine' || config.voice?.provider === 'volcengine_seed3_clone') {
        const player = await startOpusPlayback((active) => {
          if (playback.current === player) setPlaying(active);
        });
        if (controller.signal.aborted) {
          player.cancel();
          return;
        }
        playback.current = player;
      }
      const session = await startOpusRecording(controller.signal, async (mime) => {
        const live = await openDemoASR(mime, controller.signal);
        if (controller.signal.aborted) {
          live?.cancel();
          throw new Error('Recording canceled');
        }
        liveASR.current = live;
        return live ? (chunk) => live.send(chunk) : undefined;
      });
      if (controller.signal.aborted || captureRequest.current !== controller || pending.current) {
        session.cancel();
        if (captureRequest.current === controller) {
          liveASR.current?.cancel();
          liveASR.current = null;
        }
        return;
      }
      recorder.current = session;
      setRecording(true);
      recordingTimer.current = window.setTimeout(() => { void stopRecording(); }, 14500);
    } catch {
      if (captureRequest.current === controller) {
        liveASR.current?.cancel();
        liveASR.current = null;
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
    <PageContainer title="AI 角色对话">
      <Card style={{ maxWidth: 1440, margin: '0 auto' }}>
        <Space wrap style={{ width: '100%', marginBottom: 16 }}>
          <Typography.Text strong>设备类型</Typography.Text>
          <Select<number> placeholder="选择后台配置的设备类型"
            style={{ width: 260 }} value={deviceTypeId}
            onChange={(id) => { captureRequest.current?.abort(); setDeviceTypeId(id); }}
            options={deviceTypes.map((item) => ({
              value: item.id, label: `${item.name} (${item.code})`,
            }))} />
          {config && <Tag color="success">已应用 · {config.character.name} · V{config.version}</Tag>}
          <Button disabled={!config || sending || recording || playing} onClick={() => setSessionGeneration((current) => current + 1)}>新建会话</Button>
          {history.length > 0 && <Button href={`/agent-demo?observe=${encodeURIComponent(conversationSession.current)}`} target="_blank" rel="noopener noreferrer">打开观测窗口</Button>}
          <Space size="small">
            <Switch checked={remember} disabled={sending || recording || !config}
              onChange={setRemember} aria-label="启用偏好记忆" />
            <Typography.Text type="secondary">偏好记忆（主动开启，仅供 Demo）</Typography.Text>
            <Button size="small" disabled={!config || sending || recording || playing} onClick={() => setMemoryOpen(true)}>查看记忆</Button>
          </Space>
        </Space>
        <Space wrap size="small" style={{ marginBottom: 16 }}>
          <Typography.Text type="secondary">服务配置</Typography.Text>
          {runtimeStatus ? (
            <>
              {[
                { name: 'LLM', configured: runtimeStatus.llm },
                { name: 'ASR', configured: runtimeStatus.asr },
                { name: 'TTS', configured: Boolean(config?.voice && runtimeStatus.tts[config.voice.provider]) },
                { name: 'RAG（Redis / Embedding）', configured: runtimeStatus.knowledge },
              ].map((item) => <Tag key={item.name} color={item.configured ? 'success' : 'default'}>{item.name} · {item.configured ? '已配置' : '未配置'}</Tag>)}
              <Typography.Text type="secondary">配置状态不代表云端连通性</Typography.Text>
            </>
          ) : <Typography.Text type={statusError ? 'warning' : 'secondary'}>{statusError ? '状态读取失败' : '读取中'}</Typography.Text>}
          <Button size="small" type="link" onClick={refreshStatus}>刷新</Button>
        </Space>
        <Spin spinning={loadingConfig}>
          {deviceTypes.length === 0 && (
            <Empty description="请先在后台创建角色、设备类型，并应用配置" />
          )}
          {deviceTypeId && !loadingConfig && !config && (
            <Alert type="warning" showIcon
              message={configError ? "生效配置读取失败" : "该设备类型还没有生效配置"}
              description={configError ? "请检查服务端与当前角色权限。" : "请在设备类型页面保存能力草稿，再点击「应用配置」。"}
              action={<Button size="small" href="/device-types">前往设备类型</Button>} />
          )}
          <Tabs activeKey={view} onChange={setView} items={[{ key: 'chat', label: '对话' }, { key: 'trajectory', label: '执行轨迹' }]} />
          <div style={{ minHeight: 460, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {view === 'trajectory' && <Trajectory records={records} incomplete={incomplete} />}
            <div style={{ flex: 1, minHeight: 0, maxHeight: 460, overflow: 'auto', display: view === 'chat' ? undefined : 'none' }}>
              {history.length > 0 ? (
                <Bubble.List items={history} role={bubbleRoles} />
              ) : (
                <Empty style={{ paddingTop: 90 }}
                  description={config ? `可以开始和「${config.character.name}」对话` : '选择已应用配置的设备类型'} />
              )}
            </div>
            <Space wrap>
              <Button type={recording ? 'primary' : 'default'} danger={recording}
                disabled={!config || sending || (!recording && (runtimeStatus?.asr === false || runtimeStatus?.llm === false))}
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
              disabled={!config || loadingConfig || recording || runtimeStatus?.llm === false}
              onSubmit={(text) => { void send(text); }}
              onCancel={() => { pending.current?.abort(); playback.current?.cancel(); setPlaying(false); }}
              autoSize={{ minRows: 2, maxRows: 4 }} />
          </div>
        </Spin>
      </Card>
      {memoryOpen && deviceTypeId && <MemoryPanel key={deviceTypeId} deviceId={deviceTypeId} onClose={() => setMemoryOpen(false)} />}
    </PageContainer>
  );
};

const AgentDemo: React.FC = () => {
  const { search } = useLocation();
  const params = new URLSearchParams(search);
  const observe = params.get('observe');
  const requestedDevice = Number(params.get('device_type_id'));
  const initialDeviceId = Number.isSafeInteger(requestedDevice) && requestedDevice > 0 ? requestedDevice : undefined;
  if (!observe) return <AgentChat key={initialDeviceId ?? 'default'} initialDeviceId={initialDeviceId} />;
  return (
    <PageContainer title="AI 角色对话">
      <Card style={{ maxWidth: 1440, margin: '0 auto' }}>
        <SessionObserver key={observe} sessionId={observe} />
      </Card>
    </PageContainer>
  );
};

export default AgentDemo;
