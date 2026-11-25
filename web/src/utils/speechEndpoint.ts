// A lightweight browser endpoint detector, not a speech recognition model.
// Only a sustained voiced interval arms the silence timer.
export class SpeechEndpoint {
  private voicedSince: number | undefined;
  private lastVoice: number | undefined;
  private armed = false;
  private ended = false;

  constructor(
    private readonly silenceMs = 700,
    private readonly minVoiceMs = 150,
    private readonly threshold = 0.025,
  ) {}

  observe(rms: number, now: number): boolean {
    if (this.ended) return false;
    if (rms >= this.threshold) {
      this.voicedSince ??= now;
      if (now - this.voicedSince >= this.minVoiceMs) this.armed = true;
      this.lastVoice = now;
      return false;
    }
    this.voicedSince = undefined;
    if (this.armed && this.lastVoice !== undefined && now - this.lastVoice >= this.silenceMs) {
      this.ended = true;
      return true;
    }
    return false;
  }
}

export function rmsLevel(samples: Float32Array): number {
  if (samples.length === 0) return 0;
  let sum = 0;
  for (const sample of samples) sum += sample * sample;
  return Math.sqrt(sum / samples.length);
}

// Observe the existing microphone stream without connecting it to speakers.
export async function watchSpeechEndpoint(stream: MediaStream, onEnd: () => void): Promise<() => void> {
  const context = new AudioContext();
  try {
    const source = context.createMediaStreamSource(stream);
    const analyser = context.createAnalyser();
    analyser.fftSize = 1024;
    source.connect(analyser);
    await context.resume();
    const samples = new Float32Array(analyser.fftSize);
    const detector = new SpeechEndpoint();
    let closed = false;
    const timer = window.setInterval(() => {
      analyser.getFloatTimeDomainData(samples);
      if (detector.observe(rmsLevel(samples), performance.now())) onEnd();
    }, 50);
    return () => {
      if (closed) return;
      closed = true;
      window.clearInterval(timer);
      source.disconnect();
      analyser.disconnect();
      void context.close().catch(() => {});
    };
  } catch (error) {
    await context.close().catch(() => {});
    throw error;
  }
}
