import { watchSileroSpeech } from './sileroVad';

export const preSpeechDelaySeconds = 0.3;

type HandsFreeCallbacks = {
  onReady: () => void;
  onStart: (captureStream: MediaStream) => void;
  onEnd: () => void;
};

// VAD observes the undelayed signal; MediaRecorder observes a short delayed
// stream, preserving speech while the detector confirms the start. No silent
// microphone audio is uploaded between turns.
export async function startHandsFreeMicrophone(
  signal: AbortSignal,
  callbacks: HandsFreeCallbacks,
): Promise<void> {
  if (signal.aborted) return;
  const context = new AudioContext();
  let microphone: MediaStream | undefined;
  let capture: MediaStream | undefined;
  let cleanupVAD: (() => void) | undefined;
  let closed = false;
  const close = () => {
    if (closed) return;
    closed = true;
    signal.removeEventListener('abort', close);
    cleanupVAD?.();
    microphone?.getTracks().forEach((track) => { track.stop(); });
    capture?.getTracks().forEach((track) => { track.stop(); });
    void context.close().catch(() => undefined);
  };
  signal.addEventListener('abort', close, { once: true });
  try {
    await context.resume();
    if (closed) return;
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
    });
    if (closed) {
      stream.getTracks().forEach((track) => { track.stop(); });
      return;
    }
    microphone = stream;
    const source = context.createMediaStreamSource(stream);
    const delay = context.createDelay(1);
    delay.delayTime.value = preSpeechDelaySeconds;
    const destination = context.createMediaStreamDestination();
    destination.channelCount = 1;
    source.connect(delay);
    delay.connect(destination);
    const delayed = destination.stream;
    capture = delayed;
    const cleanup = await watchSileroSpeech(stream, {
      onReady: () => { if (!closed) callbacks.onReady(); },
      onStart: () => { if (!closed) callbacks.onStart(delayed); },
      onEnd: () => { if (!closed) callbacks.onEnd(); },
    });
    if (closed) cleanup();
    else cleanupVAD = cleanup;
  } catch (error) {
    close();
    if (!signal.aborted) throw error;
  }
}
