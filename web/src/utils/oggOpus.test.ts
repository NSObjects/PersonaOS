import { describe, expect, it } from 'vitest';

import { OggOpusPackets, opusDurationUs } from './oggOpus';

const bytes = (text: string) => new TextEncoder().encode(text);
function page(lacing: number[], body: Uint8Array, flags = 0, serial = 0): Uint8Array {
  const data = new Uint8Array(27 + lacing.length + body.length);
  data.set(bytes('OggS'));
  data[5] = flags;
  new DataView(data.buffer).setUint32(14, serial, true);
  data[26] = lacing.length;
  data.set(lacing, 27);
  data.set(body, 27 + lacing.length);
  return data;
}

describe('incremental Ogg Opus packets', () => {
  it('delivers a complete audio packet before the stream finishes', () => {
    const head = new Uint8Array(19);
    head.set(bytes('OpusHead'));
    head[9] = 1;
    const received: Uint8Array[] = [];
    const parser = new OggOpusPackets((value) => received.push(value), (value) => received.push(value));
    const headers = page([19], head);
    const tags = page([8], bytes('OpusTags'));
    const audio = page([2], new Uint8Array([0xf8, 0xff]));
    for (const part of [headers, tags, audio.slice(0, 11), audio.slice(11)]) parser.push(part);
    expect(received).toEqual([head, new Uint8Array([0xf8, 0xff])]);
    parser.finish();
  });

  it('plays two chained logical Opus streams in order after a flush', () => {
    const head = new Uint8Array(19);
    head.set(bytes('OpusHead'));
    head[9] = 1;
    const received: string[] = [];
    const parser = new OggOpusPackets(
      () => received.push('head'),
      (packet) => received.push(String(packet[0])),
    );
    for (const serial of [1, 2]) {
      parser.push(page([19], head, 2, serial));
      parser.push(page([8], bytes('OpusTags'), 0, serial));
      const audio = page([2], new Uint8Array([serial, 0xff]), 4, serial);
      parser.push(audio.slice(0, 13));
      parser.push(audio.slice(13));
    }
    parser.finish();
    expect(received).toEqual(['head', '1', 'head', '2']);
  });

  it('rejects a new logical stream before the previous one ends', () => {
    const head = new Uint8Array(19);
    head.set(bytes('OpusHead'));
    head[9] = 1;
    const parser = new OggOpusPackets(() => {}, () => {});
    parser.push(page([19], head, 2, 1));
    parser.push(page([8], bytes('OpusTags'), 0, 1));
    parser.push(page([2], new Uint8Array([0xf8, 0xff]), 0, 1));
    expect(() => parser.push(page([19], head, 2, 2))).toThrow('Unexpected Ogg stream');
  });

  it('assembles a packet continued on the next page', () => {
    const head = new Uint8Array(19);
    head.set(bytes('OpusHead'));
    head[9] = 1;
    const received: Uint8Array[] = [];
    const parser = new OggOpusPackets(() => {}, (packet) => received.push(packet));
    parser.push(page([19], head));
    parser.push(page([8], bytes('OpusTags')));
    parser.push(page([255], new Uint8Array(255)));
    expect(received).toHaveLength(0);
    parser.push(page([1], new Uint8Array([7]), 1));
    expect(received[0].length).toBe(256);
    expect(received[0][255]).toBe(7);
    parser.finish();
  });

  it('rejects an incomplete stream', () => {
    const parser = new OggOpusPackets(() => {}, () => {});
    parser.push(bytes('OggS'));
    expect(() => parser.finish()).toThrow('Incomplete');
  });

  it('calculates the packet duration from the Opus TOC', () => {
    expect(opusDurationUs(new Uint8Array([0xf8]))).toBe(20000);
    expect(opusDurationUs(new Uint8Array([0xf9]))).toBe(40000);
    expect(() => opusDurationUs(new Uint8Array([0xfb, 63]))).toThrow('Invalid');
  });
});
