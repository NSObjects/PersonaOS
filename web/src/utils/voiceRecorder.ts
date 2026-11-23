// Capture Opus natively. The server accepts Ogg Opus or WebM/Opus,
// remuxing WebM without re-encoding before sending to go-speech.
const opusTypes = ['audio/ogg;codecs=opus', 'audio/webm;codecs=opus'];
const maxRecordingBytes = 1 << 20;

export function selectOpusMimeType(supports: (type: string) => boolean): string {
  return opusTypes.find((type) => supports(type)) ?? '';
}

export type RecordingSession = {
  stop(): Promise<Blob>;
  cancel(): void;
};

export async function startOpusRecording(): Promise<RecordingSession> {
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
  try {
    const media = new MediaRecorder(microphone, {
      mimeType, audioBitsPerSecond: 24000,
    });
    const chunks: Blob[] = [];
    let canceled = false;
    let stopping = false;
    let failed = false;
    const release = () => {
      microphone.getTracks().forEach((track) => { track.stop(); });
    };
    media.ondataavailable = (event) => {
      if (event.data.size > 0) chunks.push(event.data);
    };
    media.onerror = () => { failed = true; };
    media.onstop = release;
    media.start(250);

    return {
      cancel() {
        canceled = true;
        if (media.state !== 'inactive') media.stop();
        release();
      },
      async stop() {
        if (stopping || canceled || media.state === 'inactive') {
          throw new Error('Opus recording is no longer active');
        }
        stopping = true;
        return new Promise<Blob>((resolve, reject) => {
          media.onstop = () => {
            release();
            if (canceled || failed) {
              reject(new Error('Opus recording failed'));
              return;
            }
            const opus = new Blob(chunks, { type: mimeType });
            if (opus.size === 0 || opus.size > maxRecordingBytes) {
              reject(new Error('Opus recording is empty or too long'));
              return;
            }
            resolve(opus);
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
