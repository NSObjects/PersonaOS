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

});
