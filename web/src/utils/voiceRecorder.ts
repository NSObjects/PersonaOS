// Browser microphone capture is deliberately local to the Demo UI.
// The go-speech SDK requires raw PCM16LE; MediaRecorder/WebM is not that format.
const targetSampleRate = 16000;

export function encodePCM16(samples: Float32Array): ArrayBuffer {
  const pcm = new ArrayBuffer(samples.length * 2);
  const view = new DataView(pcm);
  samples.forEach((sample, index) => {
    const limited = Math.max(-1, Math.min(1, sample));
    view.setInt16(index * 2, limited < 0
      ? Math.round(limited * 32768)
      : Math.round(limited * 32767), true);
  });
  return pcm;
}

export type RecordingSession = {
  stop(): Promise<ArrayBuffer>;
  cancel(): void;
};

export async function startPCMRecording(): Promise<RecordingSession> {
  const microphone = await navigator.mediaDevices.getUserMedia({
    audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
  });
  let context: AudioContext | undefined;
  try {
    context = new AudioContext();
    const input = context.createMediaStreamSource(microphone);
    // ScriptProcessor is retained only for this short push-to-talk Demo.
    // A later low-latency WebSocket implementation can use AudioWorklet.
    const processor = context.createScriptProcessor(2048, 1, 1);
    const chunks: Float32Array[] = [];
    let size = 0;
    let closed = false;
    processor.onaudioprocess = (event) => {
      if (closed) return;
      const samples = new Float32Array(event.inputBuffer.getChannelData(0));
      chunks.push(samples);
      size += samples.length;
    };
    input.connect(processor);
    processor.connect(context.destination);

    const release = () => {
      if (closed) return;
      closed = true;
      processor.onaudioprocess = null;
      processor.disconnect();
      input.disconnect();
      microphone.getTracks().forEach((track) => track.stop());
      void context?.close();
    };

    return {
      cancel: release,
      async stop() {
        if (closed) throw new Error('Recording already stopped');
        release();
        if (!size) throw new Error('No microphone audio recorded');
        const samples = new Float32Array(size);
        let offset = 0;
        chunks.forEach((chunk) => {
          samples.set(chunk, offset);
          offset += chunk.length;
        });
        const frames = Math.max(1, Math.round(size * targetSampleRate / context!.sampleRate));
        const offline = new OfflineAudioContext(1, frames, targetSampleRate);
        const audio = offline.createBuffer(1, samples.length, context!.sampleRate);
        audio.copyToChannel(samples, 0);
        const source = offline.createBufferSource();
        source.buffer = audio;
        source.connect(offline.destination);
        source.start();
        const rendered = await offline.startRendering();
        return encodePCM16(rendered.getChannelData(0));
      },
    };
  } catch (error) {
    microphone.getTracks().forEach((track) => track.stop());
    if (context) void context.close();
    throw error;
  }
}
