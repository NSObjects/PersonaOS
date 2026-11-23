import { describe, expect, it } from 'vitest';

import { encodePCM16 } from './voiceRecorder';

describe('encodePCM16', () => {
  it('encodes raw mono samples as little-endian signed 16-bit PCM', () => {
    const bytes = encodePCM16(new Float32Array([-1, 0, 1]));
    const view = new DataView(bytes);
    expect(view.byteLength).toBe(6);
    expect(view.getInt16(0, true)).toBe(-32768);
    expect(view.getInt16(2, true)).toBe(0);
    expect(view.getInt16(4, true)).toBe(32767);
  });

  it('clamps out-of-range samples to the PCM limits', () => {
    const view = new DataView(encodePCM16(new Float32Array([-2, 2])));
    expect(view.getInt16(0, true)).toBe(-32768);
    expect(view.getInt16(2, true)).toBe(32767);
  });
});
