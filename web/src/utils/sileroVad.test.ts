import type { RealTimeVADOptions } from '@ricky0123/vad-web';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { watchSileroSpeech } from './sileroVad';

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  start: vi.fn(),
  destroy: vi.fn(),
}));
vi.mock('@ricky0123/vad-web', () => ({ MicVAD: { new: mocks.create } }));

describe('watchSileroSpeech', () => {
  const resume = vi.fn(async () => {});
  const close = vi.fn(async () => {});
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.create.mockResolvedValue({ start: mocks.start, destroy: mocks.destroy, listening: true });
    mocks.start.mockResolvedValue(undefined);
    mocks.destroy.mockResolvedValue(undefined);
    vi.stubGlobal('AudioContext', class {
      resume = resume;
      close = close;
    });
  });
  afterEach(() => { vi.unstubAllGlobals(); });

  it('uses local assets and the supplied microphone, and ignores callbacks after disposal', async () => {
    const stop = vi.fn();
    const stream = { getTracks: () => [{ stop }] } as unknown as MediaStream;
    const onReady = vi.fn();
    const onStart = vi.fn();
    const onEnd = vi.fn();
    const cleanup = await watchSileroSpeech(stream, { onReady, onStart, onEnd });
    const options = mocks.create.mock.calls[0][0] as Partial<RealTimeVADOptions>;
    expect(options.model).toBe('v5');
    expect(options.processorType).toBe('AudioWorklet');
    expect(options.baseAssetPath).toBe('/vad/');
    expect(options.onnxWASMBasePath).toBe('/vad/');
    expect(options.redemptionMs).toBe(700);
    expect(options.preSpeechPadMs).toBe(300);
    expect(await options.getStream?.()).toBe(stream);
    await options.pauseStream?.(stream);
    expect(stop).not.toHaveBeenCalled();
    options.onSpeechRealStart?.();
    const audio = new Float32Array(512);
    expect(onReady).not.toHaveBeenCalled();
    const probabilities = { isSpeech: 0.1, notSpeech: 0.9 };
    options.onFrameProcessed?.(probabilities, audio);
    options.onFrameProcessed?.(probabilities, audio);
    expect(onReady).toHaveBeenCalledTimes(1);
    options.onSpeechEnd?.(audio);
    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onEnd).toHaveBeenCalledWith(audio);
    cleanup();
    cleanup();
    options.onFrameProcessed?.(probabilities, audio);
    expect(onReady).toHaveBeenCalledTimes(1);
    options.onSpeechRealStart?.();
    options.onSpeechEnd?.(audio);
    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onEnd).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(close).toHaveBeenCalledTimes(1));
    expect(mocks.destroy).toHaveBeenCalledTimes(1);
    expect(stop).not.toHaveBeenCalled();
  });

  it('closes its audio context when model loading fails', async () => {
    mocks.create.mockRejectedValueOnce(new Error('model unavailable'));
    await expect(watchSileroSpeech({} as MediaStream, {})).rejects.toThrow('model unavailable');
    await vi.waitFor(() => expect(close).toHaveBeenCalledTimes(1));
  });
});
