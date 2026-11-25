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

describe('live Opus recording', () => {
  afterEach(() => { vi.unstubAllGlobals(); });

  it('sends chunks before stop without collecting a complete recording', async () => {
    let emit!: (data: Blob) => void;
    const stopTrack = vi.fn();
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia: async () => ({
      getTracks: () => [{ stop: stopTrack }],
    }) } });
    class FakeMediaRecorder {
      static isTypeSupported = () => true;
      state: RecordingState = 'inactive';
      ondataavailable: ((event: { data: Blob }) => void) | null = null;
      onstop: (() => void) | null = null;
      start() {
        this.state = 'recording';
        emit = (data) => this.ondataavailable?.({ data });
      }
      stop() {
        emit(new Blob(['last']));
        this.state = 'inactive';
        this.onstop?.();
      }
    }
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    const send = vi.fn();
    const recording = await startOpusRecording(undefined, async () => send);
    emit(new Blob(['first']));
    expect(send).toHaveBeenCalledTimes(1);
    expect(await recording.stop()).toBeNull();
    expect(send).toHaveBeenCalledTimes(2);
    expect(stopTrack).toHaveBeenCalledTimes(1);
  });
});

describe('shared microphone ownership', () => {
  afterEach(() => { vi.unstubAllGlobals(); });

  it('keeps a borrowed microphone alive across completed and canceled turns', async () => {
    const stopTrack = vi.fn();
    const stream = { getTracks: () => [{ stop: stopTrack }] } as unknown as MediaStream;
    const getUserMedia = vi.fn();
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });
    class FakeMediaRecorder {
      static isTypeSupported = () => true;
      state: RecordingState = 'inactive';
      ondataavailable: ((event: { data: Blob }) => void) | null = null;
      onstop: (() => void) | null = null;
      start() { this.state = 'recording'; }
      stop() {
        this.ondataavailable?.({ data: new Blob(['audio']) });
        this.state = 'inactive';
        this.onstop?.();
      }
    }
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    const completed = await startOpusRecording(undefined, undefined, undefined, stream);
    expect(await completed.stop()).toBeInstanceOf(Blob);
    const controller = new AbortController();
    await startOpusRecording(controller.signal, undefined, undefined, stream);
    controller.abort();
    expect(getUserMedia).not.toHaveBeenCalled();
    expect(stopTrack).not.toHaveBeenCalled();
  });

  it('does not stop a borrowed stream when ASR setup fails', async () => {
    const stopTrack = vi.fn();
    const stream = { getTracks: () => [{ stop: stopTrack }] } as unknown as MediaStream;
    class FakeMediaRecorder {
      static isTypeSupported = () => true;
    }
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    await expect(startOpusRecording(undefined, async () => {
      throw new Error('ASR setup failed');
    }, undefined, stream)).rejects.toThrow('ASR setup failed');
    expect(stopTrack).not.toHaveBeenCalled();
  });
});
