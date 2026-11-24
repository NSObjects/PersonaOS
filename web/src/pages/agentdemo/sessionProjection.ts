import type { DemoSessionEvents } from '@/services/admin';
import { applyExecutionEvent, type ExecutionRecord, settleExecutionTurn } from './trajectoryModel';

export type SessionMessage = {
  key: string;
  role: 'user' | 'assistant';
  content: string;
};

export type SessionProjection = {
  cursor: number;
  messages: SessionMessage[];
  records: ExecutionRecord[];
  incomplete: boolean;
  activeTurn?: number;
  failedTurn?: number;
};

export function upsertReply(messages: SessionMessage[], turn: number, content: string): SessionMessage[] {
  const key = `a-${turn}`;
  if (messages.some((item) => item.key === key)) {
    return messages.map((item) => item.key === key ? { ...item, content } : item);
  }
  return [...messages, { key, role: 'assistant', content }];
}

export function emptySessionProjection(): SessionProjection {
  return { cursor: 0, messages: [], records: [], incomplete: false };
}

// Replay and live delivery use the same reducer. Reconnect can safely repeat
// already received sequences without duplicating a bubble or invocation.
export function projectSessionEvents(
  previous: SessionProjection, batch: DemoSessionEvents,
): SessionProjection {
  let { cursor, messages, records, activeTurn, failedTurn } = previous;
  let incomplete = previous.incomplete || batch.gap;
  for (const event of batch.events) {
    if (event.sequence <= cursor) continue;
    if (event.sequence !== cursor + 1) incomplete = true;
    cursor = event.sequence;
    if (activeTurn !== undefined && event.turn > activeTurn) {
      records = settleExecutionTurn(records, activeTurn);
      activeTurn = undefined;
    }
    switch (event.type) {
      case 'turn_start':
        activeTurn = event.turn;
        failedTurn = undefined;
        if (event.message !== undefined) messages = [...messages, { key: `u-${event.turn}`, role: 'user', content: event.message }];
        break;
      case 'trace':
        activeTurn = event.turn;
        if (event.trace) records = applyExecutionEvent(records, event.turn, event.trace);
        break;
      case 'reply_preview':
        activeTurn = event.turn;
        if (event.preview !== undefined) messages = upsertReply(messages, event.turn, event.preview);
        break;
      case 'turn_done':
        if (event.reply) messages = upsertReply(messages, event.turn, event.reply.reply);
        records = settleExecutionTurn(records, event.turn);
        activeTurn = undefined;
        failedTurn = undefined;
        break;
      case 'turn_error':
        records = settleExecutionTurn(records, event.turn);
        activeTurn = undefined;
        failedTurn = event.turn;
        break;
    }
  }
  return { cursor, messages: messages.slice(-128), records: records.slice(-256), incomplete, activeTurn, failedTurn };
}
