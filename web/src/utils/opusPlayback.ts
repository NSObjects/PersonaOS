import { OggOpusPackets, opusDurationUs } from './oggOpus';

// TypeScript's standard DOM declarations do not yet include audio WebCodecs.
// Declare only the browser API subset used here; no decoder implementation.
type DecodedAudio = {
  numberOfFrames: number;
  numberOfChannels: number;
  sampleRate: number;
  copyTo(destination: Float32Array, options: { planeIndex: number; format: 'f32-planar' }): void;
  close(): void;
};
type Decoder = {
  state: string;
  configure(config: { codec: string; sampleRate: number; numberOfChannels: number; description: Uint8Array<ArrayBuffer> }): void;
  decode(chunk: unknown): void;
  flush(): Promise<void>;
  close(): void;
};
type AudioCodecs = {
  AudioDecoder?: new (init: { output: (data: DecodedAudio) => void; error: (error: DOMException) => void }) => Decoder;
  EncodedAudioChunk?: new (init: { type: 'key'; timestamp: number; duration: number; data: Uint8Array<ArrayBuffer> }) => unknown;
};

export type OpusPlayback = {
  append(chunk: Uint8Array): void;
  finish(): Promise<void>;
  cancel(): void;
};

export async function startOpusPlayback(onPlaying: (playing: boolean) => void): Promise<OpusPlayback> {
  const { AudioDecoder: DecoderClass, EncodedAudioChunk: ChunkClass } = globalThis as AudioCodecs;
  if (!DecoderClass || !ChunkClass) throw new Error('WebCodecs audio decoding unavailable');
  const context = new AudioContext();
  const sources = new Set<AudioBufferSourceNode>();
  let nextTime = 0;
  let timestamp = 0;
  let closed = false;
  let finished = false;
  let failure: Error | undefined;
  const closeContext = () => {
    if (!closed) {
      closed = true;
      onPlaying(false);
      void context.close().catch(() => undefined);
    }
  };
  const decoder = new DecoderClass({
    error(error) { failure = error; },
    output(data) {
      try {
        if (closed || failure || !data.numberOfFrames) return;
        const buffer = context.createBuffer(data.numberOfChannels, data.numberOfFrames, data.sampleRate);
        for (let i = 0; i < data.numberOfChannels; i++) {
          data.copyTo(buffer.getChannelData(i), { planeIndex: i, format: 'f32-planar' });
        }
        const source = context.createBufferSource();
        source.buffer = buffer;
        source.connect(context.destination);
        if (sources.size === 0) onPlaying(true);
        sources.add(source);
        source.onended = () => {
          sources.delete(source);
          source.disconnect();
          if (finished && sources.size === 0) closeContext();
        };
        nextTime = Math.max(nextTime, context.currentTime + 0.04);
        source.start(nextTime);
        nextTime += buffer.duration;
      } catch (error) {
        failure = error instanceof Error ? error : new Error('Audio playback failed');
      } finally {
        data.close();
      }
    },
  });
  const cancel = () => {
    for (const source of sources) {
      source.onended = null;
      source.stop();
      source.disconnect();
    }
    sources.clear();
    if (decoder.state !== 'closed') decoder.close();
    closeContext();
  };
  const packets = new OggOpusPackets(
    (head) => decoder.configure({ codec: 'opus', sampleRate: 48000, numberOfChannels: 1, description: head }),
    (packet) => {
      const duration = opusDurationUs(packet);
      decoder.decode(new ChunkClass({ type: 'key', timestamp, duration, data: packet }));
      timestamp += duration;
    },
  );
  try {
    // Called from the recording button's user gesture, before network I/O.
    await context.resume();
  } catch (error) {
    cancel();
    throw error;
  }
  return {
    append(chunk) {
      if (failure) throw failure;
      if (closed || finished) throw new Error('Playback closed');
      packets.push(chunk);
    },
    async finish() {
      try {
        packets.finish();
        await decoder.flush();
        if (failure) throw failure;
        if (decoder.state !== 'closed') decoder.close();
        finished = true;
        if (!sources.size) closeContext();
      } catch (error) {
        cancel();
        throw error;
      }
    },
    cancel,
  };
}
