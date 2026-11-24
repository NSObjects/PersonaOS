import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@umijs/max', () => ({
  request: vi.fn(),
}));

describe('admin table adapters', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('pageParams', () => {
    it('maps ProTable request params to backend pagination', async () => {
      const { pageParams } = await import('./admin');
      expect(pageParams({ current: 3, pageSize: 50 })).toEqual({
        page: 3,
        page_size: 50,
      });
    });

    it('falls back to first page with 20 rows when params are missing', async () => {
      const { pageParams } = await import('./admin');
      expect(pageParams({})).toEqual({ page: 1, page_size: 20 });
    });
  });

  describe('toTableResult', () => {
    it('maps an envelope response to the ProTable result shape', async () => {
      const { toTableResult } = await import('./admin');
      const result = await toTableResult({
        code: 0,
        message: 'ok',
        data: [{ id: 1 }, { id: 2 }],
        page: { page: 1, page_size: 20, total: 2, has_next: false },
      });
      expect(result).toEqual({
        data: [{ id: 1 }, { id: 2 }],
        success: true,
        total: 2,
      });
    });

    it('reports total 0 when the envelope has no page meta', async () => {
      const { toTableResult } = await import('./admin');
      const result = await toTableResult({
        code: 0,
        message: 'ok',
        data: [],
      });
      expect(result).toEqual({ data: [], success: true, total: 0 });
    });
  });
  describe('capability resource API', () => {
    it('passes the selected resource kind to the API', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: [{ id: 9, kind: 'voice' }] });
      const { listCapabilityResources } = await import('./admin');
      const result = await listCapabilityResources('voice');
      expect(request).toHaveBeenCalledWith('/api/capability-resources', {
        params: { kind: 'voice' },
      });
      expect(result).toEqual([{ id: 9, kind: 'voice' }]);
    });

    it('writes edits through PATCH instead of replacing resource identity', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: { id: 9 } });
      const { updateCapabilityResource } = await import('./admin');
      await updateCapabilityResource(9, { name: 'Voice B', reference: 'voice-b', enabled: false });
      expect(request).toHaveBeenCalledWith('/api/capability-resources/9', {
        method: 'PATCH',
        data: { name: 'Voice B', reference: 'voice-b', enabled: false },
      });
    });
  });

  describe('device type capability draft API', () => {
    it('loads one device type draft without mixing other device types', async () => {
      const { request } = await import('@umijs/max');
      const draft = {
        device_type_id: 3, voice_id: 12, tool_ids: [31],
        skill_ids: [], knowledge_base_ids: [41],
      };
      vi.mocked(request).mockResolvedValueOnce({ data: draft });
      const { getDeviceTypeCapabilityDraft } = await import('./admin');
      expect(await getDeviceTypeCapabilityDraft(3)).toEqual(draft);
      expect(request).toHaveBeenCalledWith('/api/device-types/3/capability-draft');
    });

    it('saves the complete selection using PUT, never a partial PATCH', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: { device_type_id: 4 } });
      const { saveDeviceTypeCapabilityDraft } = await import('./admin');
      const selection = {
        voice_id: 0, tool_ids: [12, 13], skill_ids: [21], knowledge_base_ids: [],
      };
      await saveDeviceTypeCapabilityDraft(4, selection);
      expect(request).toHaveBeenCalledWith('/api/device-types/4/capability-draft', {
        method: 'PUT', data: selection,
      });
    });
  });

  describe('confirmed Demo device type configuration', () => {
    it('reads only the active snapshot for one dynamically created device type', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: { version: 2 } });
      const { getActiveDeviceConfig } = await import('./admin');
      expect(await getActiveDeviceConfig(17)).toEqual({ version: 2 });
      expect(request).toHaveBeenCalledWith('/api/device-types/17/active-config');
    });

    it('explicitly applies the current draft using PUT', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: { version: 1 } });
      const { applyDeviceTypeConfig } = await import('./admin');
      expect(await applyDeviceTypeConfig(23)).toEqual({ version: 1 });
      expect(request).toHaveBeenCalledWith('/api/device-types/23/active-config', {
        method: 'PUT',
      });
    });
  });

  describe('Agent Demo chat', () => {
    it('passes device type and bounded conversation history to Eino endpoint', async () => {
      const { request } = await import('@umijs/max');
      vi.mocked(request).mockResolvedValueOnce({ data: {
        device_type_id: 18, character_name: 'Hero', reply: 'Hello!',
      } });
      const { chatWithCharacter } = await import('./admin');
      const history = [{ role: 'user' as const, content: '你好' }];
      const reply = await chatWithCharacter(18, '继续', history);
      expect(reply.reply).toBe('Hello!');
      expect(request).toHaveBeenCalledWith('/api/demo/chat', {
        method: 'POST',
        data: { device_type_id: 18, message: '继续', history },
        signal: undefined,
      });
    });
  });

  it('sends memory consent and session scope only when explicitly enabled', async () => {
    const { request } = await import('@umijs/max');
    vi.mocked(request).mockResolvedValueOnce({ data: {
      device_type_id: 5, character_name: 'Hero', reply: 'Hi', memory_queued: true,
    } });
    const { chatWithCharacter } = await import('./admin');
    const session = '550e8400-e29b-41d4-a716-446655440000';
    await chatWithCharacter(5, '你好', [], undefined, true, session);
    expect(request).toHaveBeenCalledWith('/api/demo/chat', {
      method: 'POST',
      data: {
        device_type_id: 5, message: '你好', history: [],
        remember: true, session_id: session,
      },
      signal: undefined,
    });
  });

});

describe('streamed voice reply', () => {
  it('forwards an audio event before the done event arrives', async () => {
    const { streamCharacterSpeech } = await import('./admin');
    const encoder = new TextEncoder();
    let writer!: ReadableStreamDefaultController<Uint8Array>;
    const body = new ReadableStream<Uint8Array>({ start(controller) { writer = controller; } });
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(body, {
      headers: { 'Content-Type': 'application/x-ndjson' },
    }));
    const received = vi.fn();
    try {
      const pending = streamCharacterSpeech(7, 'hello', [], received, new AbortController().signal);
      writer.enqueue(encoder.encode('{"type":"audio","mime":"audio/ogg","audio":"T2dn'));
      writer.enqueue(encoder.encode('Uw=="}\n'));
      await vi.waitFor(() => expect(received).toHaveBeenCalledWith(new Uint8Array([79, 103, 103, 83])));
      writer.enqueue(encoder.encode('{"type":"done","reply":{"device_type_id":7,"character_name":"Demo","reply":"hello"}}\n'));
      writer.close();
      expect((await pending).reply).toBe('hello');
      expect(fetchMock).toHaveBeenCalledWith('/api/demo/chat', expect.objectContaining({
        method: 'POST', credentials: 'include',
      }));
    } finally {
      fetchMock.mockRestore();
    }
  });
});
