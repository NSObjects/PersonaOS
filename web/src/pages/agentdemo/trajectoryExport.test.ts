import { describe, expect, it } from 'vitest';
import type { SessionMessage } from './sessionProjection';
import { buildTurnExport } from './trajectoryExport';
import type { ExecutionRecord } from './trajectoryModel';

describe('turn trace export', () => {
  it('exports only the chosen turn, including messages and all invocation details', () => {
    const records: ExecutionRecord[] = [
      { key: '1:1', turn: 1, kind: 'model', name: 'old model', start: 100, end: 200 },
      { key: '2:-1', turn: 2, kind: 'audio', name: 'ASR', clock: 'browser', start: 0, end: 50 },
      { key: '2:1', turn: 2, kind: 'tool', name: 'search', start: 1000, end: 1300, input: '{"query":"news"}', output: 'result' },
      { key: '2:2', turn: 2, kind: 'model', name: 'LLM', start: 1300, firstOutput: 1500, interrupted: true, input: 'Tools: get_weather', finishReason: 'tool_calls', usage: { prompt_tokens: 200, completion_tokens: 20, total_tokens: 220, completion_token_details: { reasoning_tokens: 8 } } },
      { key: '2:3', turn: 2, kind: 'audio', name: 'TTS', error: 'failed' },
    ];
    const messages: SessionMessage[] = [
      { key: 'u-1', role: 'user', content: 'old question' },
      { key: 'u-2', role: 'user', content: 'news?' },
      { key: 'a-2', role: 'assistant', content: 'partial reply' },
    ];
    const data = buildTurnExport(records, messages, 2, true);
    expect(data.turn).toBe(2);
    expect(data.session_trace_incomplete).toBe(true);
    expect(data.messages).toEqual(messages.slice(1));
    expect(data.records.map((record) => record.key)).toEqual(['2:-1', '2:1', '2:2', '2:3']);
    expect(data.records[0]).toMatchObject({ clock: 'browser_monotonic_ms', start_ms: 0, duration_ms: 50, status: 'completed' });
    expect(data.records[1]).toMatchObject({ clock: 'server_epoch_ms', input: '{"query":"news"}', output: 'result', duration_ms: 300 });
    expect(data.records[2]).toMatchObject({ status: 'incomplete', first_output_latency_ms: 200, end_ms: null, duration_ms: null, input: 'Tools: get_weather', finish_reason: 'tool_calls', usage: { total_tokens: 220, completion_token_details: { reasoning_tokens: 8 } } });
    expect(data.records[3]).toMatchObject({ status: 'error', start_ms: null, error: 'failed' });
  });

  it('does not invent missing messages or timings', () => {
    const data = buildTurnExport([{ key: '3:1', turn: 3, kind: 'tool', name: 'search', end: 100 }], [], 3, true);
    expect(data.messages).toEqual([]);
    expect(data.records[0]).toMatchObject({ start_ms: null, end_ms: 100, duration_ms: null, first_output_latency_ms: null, finish_reason: null });
  });
});
