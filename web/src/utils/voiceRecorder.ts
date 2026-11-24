// Capture Opus natively. The server accepts Ogg Opus or WebM/Opus,
// remuxing WebM without re-encoding before sending to go-speech.
const opusTypes = ['audio/ogg;codecs=opus', 'audio/webm;codecs=opus'];
const maxRecordingBytes = 1 << 20;

export function selectOpusMimeType(supports: (type: string) => boolean): string {
  return opusTypes.find((type) => supports(type)) ?? '';
}

export type RecordingSession = {
  stop(): Promise<Blob | null>;
  cancel(): void;
};

export async function startOpusRecording(
  signal?: AbortSignal,
  beforeStart?: (mime: string) => Promise<((chunk: Blob) => void) | undefined>,
): Promise<RecordingSession> {
  if (signal?.aborted) throw new Error('Opus recording canceled');
  if (typeof MediaRecorder === 'undefined') {
    throw new Error('This browser does not support Opus recording');
  }
  const mimeType = selectOpusMimeType((type) => MediaRecorder.isTypeSupported(type));
  if (!mimeType) {
    throw new Error('This browser does not support Ogg Opus or WebM Opus recording');
  }
  const microphone = await navigator.mediaDevices.getUserMedia({
    audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
  });
  if (signal?.aborted) {
    microphone.getTracks().forEach((track) => { track.stop(); });
    throw new Error('Opus recording canceled');
  }
  try {
    const media = new MediaRecorder(microphone, {
      mimeType, audioBitsPerSecond: 24000,
    });
    const send = await beforeStart?.(mimeType);
    if (signal?.aborted) throw new Error('Opus recording canceled');
    const chunks: Blob[] = [];
    let total = 0;
    let canceled = false;
    let stopping = false;
    let failed = false;
    let released = false;
    const release = () => {
      if (released) return;
      released = true;
      signal?.removeEventListener('abort', abortCapture);
      microphone.getTracks().forEach((track) => { track.stop(); });
    };
    const abortCapture = () => {
      canceled = true;
      if (media.state !== 'inactive') media.stop();
      release();
    };
    media.ondataavailable = (event) => {
      if (canceled || failed || !event.data.size) return;
      total += event.data.size;
      if (total > maxRecordingBytes) { failed = true; return; }
      try {
        if (send) send(event.data);
        else chunks.push(event.data);
      } catch {
        failed = true;
      }
    };
    media.onerror = () => { failed = true; };
    media.onstop = release;
    media.start(100);
    signal?.addEventListener('abort', abortCapture, { once: true });
    if (signal?.aborted) {
      abortCapture();
      throw new Error('Opus recording canceled');
    }

    return {
      cancel() {
        abortCapture();
      },
      async stop() {
        if (stopping || canceled || media.state === 'inactive') {
          throw new Error('Opus recording is no longer active');
        }
        stopping = true;
        return new Promise<Blob | null>((resolve, reject) => {
          media.onstop = () => {
            release();
            if (canceled || failed) {
              reject(new Error('Opus recording failed'));
              return;
            }
            if (total === 0 || total > maxRecordingBytes) {
              reject(new Error('Opus recording is empty or too long'));
              return;
            }
            resolve(send ? null : new Blob(chunks, { type: mimeType }));
          };
          media.stop();
        });
      },
    };
  } catch (error) {
    microphone.getTracks().forEach((track) => { track.stop(); });
    throw error;
  }
}
