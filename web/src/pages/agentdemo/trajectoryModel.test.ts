import { describe, expect, it } from 'vitest';
import { applyExecutionEvent, duration, firstOutputLatency, observeBrowserStage, settleExecutionTurn } from './trajectoryModel';

describe('trajectory projection', () => {
  it('joins start and end by turn and invocation, not name', () => {
    const start = { id: 1, kind: 'model', name: 'LLM', phase: 'start' as const, time_ms: 100 };
    const first = applyExecutionEvent([], 1, start);
    const second = applyExecutionEvent(first, 2, { ...start, time_ms: 120 });
    const done = applyExecutionEvent(second, 1, { ...start, phase: 'end', time_ms: 150 });
    expect(done).toHaveLength(2);
    expect(duration(done[0])).toBe(50);
    expect(duration(done[1])).toBeUndefined();
    expect(settleExecutionTurn(done, 2)[1].interrupted).toBe(true);
  });

  it('updates a live invocation with its first output without ending it', () => {
    const start = { id: 2, kind: 'model', name: 'LLM', phase: 'start' as const, time_ms: 100 };
    const rows = applyExecutionEvent([], 1, start);
    const partial = applyExecutionEvent(rows, 1, { ...start, phase: 'first_output', time_ms: 125 });
    expect(partial).toHaveLength(1);
    expect(firstOutputLatency(partial[0])).toBe(25);
    expect(duration(partial[0])).toBeUndefined();
    expect(firstOutputLatency(applyExecutionEvent(partial, 1, { ...start, phase: 'end', time_ms: 160 })[0])).toBe(25);
  });

  it('keeps browser measurement separate and marks failure', async () => {
    let records: Parameters<typeof applyExecutionEvent>[0] = [];
    await expect(observeBrowserStage(-1, 'ASR', (event) => {
      records = applyExecutionEvent(records, 1, event, 'browser');
    }, async () => { throw new Error('provider unavailable'); })).rejects.toThrow();
    expect(records).toHaveLength(1);
    expect(records[0].clock).toBe('browser');
    expect(records[0].error).toBe('请求未完成');
    expect(duration(records[0])).toBeGreaterThanOrEqual(0);
  });

  it('does not invent timing if a start frame is missing', () => {
    const rows = applyExecutionEvent([], 1, { id: 1, kind: 'tool', name: 'query', phase: 'end', time_ms: 200 });
    expect(duration(rows[0])).toBeUndefined();
  });
});
