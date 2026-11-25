import { describe, expect, it } from 'vitest';
import { rmsLevel, SpeechEndpoint } from './speechEndpoint';

describe('SpeechEndpoint', () => {
  it('does not end an initially silent recording', () => {
    const detector = new SpeechEndpoint();
    expect(detector.observe(0, 0)).toBe(false);
    expect(detector.observe(0, 1000)).toBe(false);
  });

  it('ignores brief noise before real speech', () => {
    const detector = new SpeechEndpoint();
    detector.observe(0.1, 0);
    detector.observe(0, 50);
    expect(detector.observe(0, 1000)).toBe(false);
  });

  it('ends exactly once after sustained voice and 700 ms silence', () => {
    const detector = new SpeechEndpoint();
    detector.observe(0.1, 0);
    detector.observe(0.1, 150);
    expect(detector.observe(0, 849)).toBe(false);
    expect(detector.observe(0, 850)).toBe(true);
    expect(detector.observe(0, 900)).toBe(false);
  });

  it('resets the silence interval when speech resumes', () => {
    const detector = new SpeechEndpoint();
    detector.observe(0.1, 0);
    detector.observe(0.1, 150);
    detector.observe(0, 650);
    detector.observe(0.1, 700);
    expect(detector.observe(0, 1399)).toBe(false);
    expect(detector.observe(0, 1400)).toBe(true);
  });
});

describe('rmsLevel', () => {
  it('calculates the signal level independently of sample polarity', () => {
    expect(rmsLevel(new Float32Array([0.5, -0.5]))).toBe(0.5);
    expect(rmsLevel(new Float32Array())).toBe(0);
  });
});
