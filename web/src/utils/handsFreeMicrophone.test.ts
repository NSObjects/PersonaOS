import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { startHandsFreeMicrophone } from './handsFreeMicrophone';
import { watchSileroSpeech } from './sileroVad';

vi.mock('./sileroVad', () => ({ watchSileroSpeech: vi.fn() }));

describe('startHandsFreeMicrophone', () => {
  const stopInput = vi.fn();
  const stopOutput = vi.fn();
  const cleanup = vi.fn();
  const close = vi.fn(async () => {});
  const resume = vi.fn(async () => {});
  const input = { getTracks: () => [{ stop: stopInput }] } as unknown as MediaStream;
  const output = { getTracks: () => [{ stop: stopOutput }] } as unknown as MediaStream;
  const getUserMedia = vi.fn(async () => input);
  const delay = { delayTime: { value: 0 }, connect: vi.fn() };
  const source = { connect: vi.fn() };
  const destination = { stream: output, channelCount: 2 };
  const mockVAD = vi.mocked(watchSileroSpeech);
  beforeEach(() => {
    vi.clearAllMocks();
    getUserMedia.mockResolvedValue(input);
    mockVAD.mockResolvedValue(cleanup);
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });
    vi.stubGlobal('AudioContext', class {
      resume = resume;
      close = close;
      createMediaStreamSource = () => source;
      createDelay = () => delay;
      createMediaStreamDestination = () => destination;
    });
  });
  afterEach(() => { vi.unstubAllGlobals(); });

  it('observes undelayed input and supplies delayed mono capture on confirmed speech', async () => {
    const controller = new AbortController();
    const callbacks = { onReady: vi.fn(), onStart: vi.fn(), onEnd: vi.fn() };
    await startHandsFreeMicrophone(controller.signal, callbacks);
    expect(getUserMedia).toHaveBeenCalledWith({
      audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
    });
    expect(delay.delayTime.value).toBe(0.3);
    expect(destination.channelCount).toBe(1);
    expect(source.connect).toHaveBeenCalledWith(delay);
    expect(delay.connect).toHaveBeenCalledWith(destination);
    const [stream, events] = mockVAD.mock.calls[0];
    expect(stream).toBe(input);
    events.onReady?.();
    events.onStart?.();
    events.onEnd?.(new Float32Array());
    expect(callbacks.onReady).toHaveBeenCalledTimes(1);
    expect(callbacks.onStart).toHaveBeenCalledWith(output);
    expect(callbacks.onEnd).toHaveBeenCalledTimes(1);
    controller.abort();
    events.onStart?.();
    events.onEnd?.(new Float32Array());
    expect(callbacks.onStart).toHaveBeenCalledTimes(1);
    expect(callbacks.onEnd).toHaveBeenCalledTimes(1);
    expect(cleanup).toHaveBeenCalledTimes(1);
    expect(stopInput).toHaveBeenCalledTimes(1);
    expect(stopOutput).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('releases the microphone if permission resolves after disable', async () => {
    let grant!: (stream: MediaStream) => void;
    getUserMedia.mockImplementationOnce(() => new Promise((resolve) => { grant = resolve; }));
    const controller = new AbortController();
    const pending = startHandsFreeMicrophone(controller.signal, {
      onReady: vi.fn(), onStart: vi.fn(), onEnd: vi.fn(),
    });
    await vi.waitFor(() => expect(getUserMedia).toHaveBeenCalled());
    controller.abort();
    grant(input);
    await pending;
    expect(stopInput).toHaveBeenCalledTimes(1);
    expect(mockVAD).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('cleans up a detector that initializes after disable', async () => {
    let loaded!: (dispose: () => void) => void;
    mockVAD.mockImplementationOnce(() => new Promise((resolve) => { loaded = resolve; }));
    const controller = new AbortController();
    const pending = startHandsFreeMicrophone(controller.signal, {
      onReady: vi.fn(), onStart: vi.fn(), onEnd: vi.fn(),
    });
    await vi.waitFor(() => expect(mockVAD).toHaveBeenCalled());
    controller.abort();
    loaded(cleanup);
    await pending;
    expect(cleanup).toHaveBeenCalledTimes(1);
    expect(stopInput).toHaveBeenCalledTimes(1);
    expect(stopOutput).toHaveBeenCalledTimes(1);
  });

  it('releases all owned resources if VAD fails to initialize', async () => {
    mockVAD.mockRejectedValueOnce(new Error('VAD failed'));
    await expect(startHandsFreeMicrophone(new AbortController().signal, {
      onReady: vi.fn(), onStart: vi.fn(), onEnd: vi.fn(),
    })).rejects.toThrow('VAD failed');
    expect(stopInput).toHaveBeenCalledTimes(1);
    expect(stopOutput).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
  });
});
