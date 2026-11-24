import type { DemoExecutionEvent } from '@/services/admin';

export type ExecutionRecord = {
  key: string;
  turn: number;
  kind: string;
  name: string;
  start?: number;
  end?: number;
  firstOutput?: number;
  clock?: 'browser';
  input?: string;
  output?: string;
  error?: string;
  usage?: DemoExecutionEvent['usage'];
  interrupted?: boolean;
};

export function applyExecutionEvent(
  records: ExecutionRecord[], turn: number, event: DemoExecutionEvent, clock?: ExecutionRecord['clock'],
): ExecutionRecord[] {
  const key = `${turn}:${event.id}`;
  const index = records.findIndex((record) => record.key === key);
  const previous = records[index];
  const record: ExecutionRecord = {
    ...previous, key, turn, kind: event.kind, name: event.name, clock,
    ...(event.phase === 'start'
      ? { start: event.time_ms, input: event.input }
      : event.phase === 'first_output'
        ? { firstOutput: event.time_ms }
        : { end: event.time_ms, output: event.output, error: event.error, usage: event.usage }),
  };
  if (index < 0) return [...records, record];
  return records.map((item, i) => i === index ? record : item);
}

export function settleExecutionTurn(records: ExecutionRecord[], turn: number): ExecutionRecord[] {
  return records.map((record) => record.turn === turn && record.end === undefined
    ? { ...record, interrupted: true } : record);
}

export function duration(record: ExecutionRecord): number | undefined {
  return record.start !== undefined && record.end !== undefined && record.end >= record.start
    ? record.end - record.start : undefined;
}

export function firstOutputLatency(record: ExecutionRecord): number | undefined {
  return record.start !== undefined && record.firstOutput !== undefined && record.firstOutput >= record.start
    ? record.firstOutput - record.start : undefined;
}

// Browser stages use a monotonic local clock. They must not be placed on the
// server wall-clock overview, or described as provider-only latency.
export async function observeBrowserStage<T>(
  id: number, name: string, emit: (event: DemoExecutionEvent) => void, run: () => Promise<T>,
): Promise<T> {
  const base = { id, kind: 'audio', name };
  emit({ ...base, phase: 'start', time_ms: Math.round(performance.now()) });
  try {
    const result = await run();
    emit({ ...base, phase: 'end', time_ms: Math.round(performance.now()) });
    return result;
  } catch (error) {
    emit({ ...base, phase: 'end', time_ms: Math.round(performance.now()), error: '请求未完成' });
    throw error;
  }
}
