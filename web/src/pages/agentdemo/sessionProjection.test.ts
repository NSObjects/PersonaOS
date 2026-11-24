import { describe, expect, it } from 'vitest';
import type { DemoSessionEvent } from '@/services/admin';
import { emptySessionProjection, projectSessionEvents } from './sessionProjection';
import { duration } from './trajectoryModel';

const start: DemoSessionEvent = { sequence: 1, turn: 1, type: 'turn_start', time_ms: 10, message: '你好' };
const trace: DemoSessionEvent = {
  sequence: 2, turn: 1, type: 'trace', time_ms: 20,
  trace: { id: 1, kind: 'model', name: 'LLM', phase: 'start', time_ms: 20 },
};
const end: DemoSessionEvent = {
  ...trace, sequence: 3, time_ms: 60,
  trace: { id: 1, kind: 'model', name: 'LLM', phase: 'end', time_ms: 60, output: '你好呀' },
};
const done: DemoSessionEvent = {
  sequence: 4, turn: 1, type: 'turn_done', time_ms: 61,
  reply: { device_type_id: 1, character_name: 'Demo', reply: '你好呀', memory_queued: false },
};

describe('session event projection', () => {
  it('shows live start and merges replay without duplicate messages or records', () => {
    const live = projectSessionEvents(emptySessionProjection(), { events: [start, trace], gap: false });
    expect(live.activeTurn).toBe(1);
    expect(live.messages).toHaveLength(1);
    expect(live.records[0].end).toBeUndefined();
    const finished = projectSessionEvents(live, { events: [start, trace, end, done], gap: false });
    expect(finished.cursor).toBe(4);
    expect(finished.activeTurn).toBeUndefined();
    expect(finished.messages.map((item) => item.content)).toEqual(['你好', '你好呀']);
    expect(finished.records).toHaveLength(1);
    expect(duration(finished.records[0])).toBe(40);
  });

  it('marks failed turns without fabricating a reply or completion time', () => {
    const failed = projectSessionEvents(emptySessionProjection(), {
      events: [start, trace, { sequence: 3, turn: 1, type: 'turn_error', time_ms: 30 }], gap: false,
    });
    expect(failed.failedTurn).toBe(1);
    expect(failed.messages).toHaveLength(1);
    expect(failed.records[0].interrupted).toBe(true);
    expect(duration(failed.records[0])).toBeUndefined();
  });

  it('flags missing history and keeps turn-local invocation IDs distinct', () => {
    const nextTrace: DemoSessionEvent = { ...trace, turn: 2, sequence: 8 };
    const snapshot = projectSessionEvents(emptySessionProjection(), { events: [trace, nextTrace], gap: true });
    expect(snapshot.incomplete).toBe(true);
    expect(snapshot.cursor).toBe(8);
    expect(snapshot.records.map((item) => item.key)).toEqual(['1:1', '2:1']);
    expect(snapshot.records[0].interrupted).toBe(true);
    expect(snapshot.activeTurn).toBe(2);
  });
});
