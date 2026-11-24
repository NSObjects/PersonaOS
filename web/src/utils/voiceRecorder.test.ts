import { afterEach, describe, expect, it, vi } from 'vitest';

import { selectOpusMimeType, startOpusRecording } from './voiceRecorder';

describe('selectOpusMimeType', () => {
  it('prefers native Ogg Opus capture where supported', () => {
    expect(selectOpusMimeType(() => true)).toBe('audio/ogg;codecs=opus');
  });

  it('uses native WebM Opus capture when Ogg recording is unavailable', () => {
    expect(selectOpusMimeType((type) => type === 'audio/webm;codecs=opus'))
      .toBe('audio/webm;codecs=opus');
  });

  it('does not silently send non-Opus microphone formats', () => {
    expect(selectOpusMimeType(() => false)).toBe('');
  });
});

describe('startOpusRecording cancellation', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('releases the microphone if canceled during the permission prompt', async () => {
    let grantMicrophone: (stream: MediaStream) => void = () => {};
    const getUserMedia = vi.fn(() => new Promise<MediaStream>((resolve) => {
      grantMicrophone = resolve;
    }));
    const stopTrack = vi.fn();
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });
    vi.stubGlobal('MediaRecorder', { isTypeSupported: () => true });

    const controller = new AbortController();
    const pending = startOpusRecording(controller.signal);
    controller.abort();
    grantMicrophone({ getTracks: () => [{ stop: stopTrack }] } as unknown as MediaStream);

    await expect(pending).rejects.toThrow('Opus recording canceled');
    expect(stopTrack).toHaveBeenCalledTimes(1);
  });

  it('stops an active recording and releases its tracks on cancellation', async () => {
    const stopTrack = vi.fn();
    const getUserMedia = vi.fn(async () => ({
      getTracks: () => [{ stop: stopTrack }],
    } as unknown as MediaStream));
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

    class FakeMediaRecorder {
      static isTypeSupported = () => true;
      state: RecordingState = 'inactive';
      onstop: (() => void) | null = null;

      start() {
        this.state = 'recording';
      }

      stop() {
        this.state = 'inactive';
        this.onstop?.();
      }
    }
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);

    const controller = new AbortController();
    const session = await startOpusRecording(controller.signal);
    controller.abort();

    expect(stopTrack).toHaveBeenCalledTimes(1);
    await expect(session.stop()).rejects.toThrow('Opus recording is no longer active');
  });
});
