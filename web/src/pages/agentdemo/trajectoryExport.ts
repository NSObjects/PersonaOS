import type { SessionMessage } from './sessionProjection';
import { duration, type ExecutionRecord, firstOutputLatency } from './trajectoryModel';

// Export the observed snapshot only. Browser monotonic timestamps are not
// comparable to server epoch timestamps; missing events remain missing.
export function buildTurnExport(
  records: ExecutionRecord[], messages: SessionMessage[], turn: number, incomplete: boolean,
) {
  const selected = records.filter((record) => record.turn === turn);
  return {
    schema_version: 1,
    turn,
    exported_at: new Date().toISOString(),
    session_trace_incomplete: incomplete,
    messages: messages.filter((message) => message.key === `u-${turn}` || message.key === `a-${turn}`),
    records: selected.map((record) => ({
      key: record.key,
      kind: record.kind,
      name: record.name,
      status: record.error ? 'error' : record.interrupted ? 'incomplete' : record.end === undefined ? 'running' : 'completed',
      clock: record.clock === 'browser' ? 'browser_monotonic_ms' : 'server_epoch_ms',
      start_ms: record.start ?? null,
      first_output_ms: record.firstOutput ?? null,
      end_ms: record.end ?? null,
      first_output_latency_ms: firstOutputLatency(record) ?? null,
      duration_ms: duration(record) ?? null,
      input: record.input ?? null,
      output: record.output ?? null,
      usage: record.usage ?? null,
      finish_reason: record.finishReason ?? null,
      error: record.error ?? null,
    })),
  };
}

export function downloadTurnExport(records: ExecutionRecord[], messages: SessionMessage[], turn: number, incomplete: boolean) {
  const payload = buildTurnExport(records, messages, turn, incomplete);
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `personaos-trace-turn-${turn}-${Date.now()}.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  // Allow the browser to start the download before releasing its object URL.
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
