import type { RealTimeVADOptions } from '@ricky0123/vad-web';

type SpeechCallbacks = {
  onReady?: () => void;
  onStart?: () => void;
  onEnd?: (audio: Float32Array) => void;
  onMisfire?: () => void;
};

// The caller owns the microphone. Silero only observes its PCM and never
// opens a second input or stops tracks when its detector is disposed.
export async function watchSileroSpeech(
  stream: MediaStream,
  callbacks: SpeechCallbacks,
): Promise<() => void> {
  const context = new AudioContext();
  let closed = false;
  let ready = false;
  let vad: { destroy(): Promise<void> } | undefined;
  const cleanup = () => {
    if (closed) return;
    closed = true;
    void (async () => {
      try {
        await vad?.destroy();
      } finally {
        await context.close().catch(() => undefined);
      }
    })().catch(() => undefined);
  };
  try {
    await context.resume();
    const { MicVAD } = await import('@ricky0123/vad-web');
    const assetPath = '/vad/';
    const options: Partial<RealTimeVADOptions> = {
      model: 'v5',
      baseAssetPath: assetPath,
      onnxWASMBasePath: assetPath,
      startOnLoad: false,
      audioContext: context,
      getStream: async () => stream,
      pauseStream: async () => {},
      resumeStream: async () => stream,
      redemptionMs: 700,
      minSpeechMs: 150,
      preSpeechPadMs: 300,
      submitUserSpeechOnPause: false,
      ortConfig: (ort) => {
        ort.env.wasm.numThreads = 1;
        ort.env.logLevel = 'error';
      },
      // Confirm that the model actually processed a frame, not just that
      // its assets were downloaded or the microphone was connected.
      onFrameProcessed: () => {
        if (closed || ready) return;
        ready = true;
        callbacks.onReady?.();
      },
      onSpeechRealStart: () => { if (!closed) callbacks.onStart?.(); },
      onSpeechEnd: (audio) => { if (!closed) callbacks.onEnd?.(audio); },
      onVADMisfire: () => { if (!closed) callbacks.onMisfire?.(); },
    };
    const detector = await MicVAD.new(options);
    vad = detector;
    await detector.start();
    if (!detector.listening) throw new Error('Silero VAD did not start');
    return cleanup;
  } catch (error) {
    cleanup();
    throw error;
  }
}
