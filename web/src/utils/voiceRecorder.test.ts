import { describe, expect, it } from 'vitest';

import { selectOpusMimeType } from './voiceRecorder';

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
