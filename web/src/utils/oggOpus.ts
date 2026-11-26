// Ogg pages and Opus packets can straddle HTTP chunk boundaries. Decode only
// complete packets, retaining the small unfinished page/packet between pushes.
const magic = (data: Uint8Array, text: string) =>
  data.length >= text.length && [...text].every((char, i) => data[i] === char.charCodeAt(0));

const concat = (a: Uint8Array, b: Uint8Array): Uint8Array<ArrayBuffer> => {
  const out = new Uint8Array(a.length + b.length);
  out.set(a);
  out.set(b, a.length);
  return out;
};

export class OggOpusPackets {
  private pending: Uint8Array<ArrayBuffer> = new Uint8Array(0);
  private packet: Uint8Array<ArrayBuffer> = new Uint8Array(0);
  private headers = 0;
  private serial?: number;
  private ended = false;

  constructor(
    private readonly onHead: (head: Uint8Array<ArrayBuffer>) => void,
    private readonly onPacket: (packet: Uint8Array<ArrayBuffer>) => void,
  ) {}

  push(chunk: Uint8Array) {
    this.pending = concat(this.pending, chunk);
    let offset = 0;
    while (this.pending.length - offset >= 27) {
      const page = this.pending.subarray(offset);
      if (!magic(page, 'OggS') || page[4] !== 0) throw new Error('Invalid Ogg page');
      const count = page[26];
      if (page.length < 27 + count) break;
      const sizes = page.subarray(27, 27 + count);
      const end = 27 + count + sizes.reduce((sum, n) => sum + n, 0);
      if (page.length < end) break;
      const serial = new DataView(page.buffer, page.byteOffset, page.byteLength).getUint32(14, true);
      if (this.serial !== undefined && this.serial !== serial) {
        // Flush can delimit independent Ogg streams in one speech response.
        // Accept only a completed stream followed by a fresh BOS page.
        if (!this.ended || !(page[5] & 2) || this.packet.length || this.headers < 3) {
          throw new Error('Unexpected Ogg stream');
        }
        this.headers = 0;
        this.ended = false;
      } else if (this.ended) {
        throw new Error('Ogg page after end of stream');
      }
      this.serial = serial;
      if (Boolean(page[5] & 1) !== Boolean(this.packet.length)) throw new Error('Invalid Ogg continuation');
      let body = 27 + count;
      for (const size of sizes) {
        this.packet = concat(this.packet, page.subarray(body, body + size));
        body += size;
        if (this.packet.length > 65536) throw new Error('Oversized Opus packet');
        if (size < 255) {
          this.accept(this.packet);
          this.packet = new Uint8Array(0);
        }
      }
      if (page[5] & 4) {
        if (this.packet.length || this.headers < 3) throw new Error('Incomplete Ogg stream at EOS');
        this.ended = true;
      }
      offset += end;
    }
    this.pending = this.pending.slice(offset);
  }

  private accept(packet: Uint8Array<ArrayBuffer>) {
    if (this.headers === 0) {
      if (!magic(packet, 'OpusHead') || packet.length < 19 || packet[9] !== 1) {
        throw new Error('Expected mono OpusHead');
      }
      this.onHead(packet);
    } else if (this.headers === 1) {
      if (!magic(packet, 'OpusTags')) throw new Error('Missing OpusTags');
    } else {
      if (packet.length === 0) throw new Error('Empty Opus packet');
      this.onPacket(packet);
    }
    this.headers += 1;
  }

  finish() {
    if (this.pending.length || this.packet.length || this.headers < 3) {
      throw new Error('Incomplete Opus stream');
    }
  }
}

// RFC 6716 TOC: frame duration depends on config and frame count on code.
export function opusDurationUs(packet: Uint8Array): number {
  if (!packet.length) throw new Error('Empty Opus packet');
  const config = packet[0] >> 3;
  const code = packet[0] & 3;
  const frameMs = config < 12
    ? [10, 20, 40, 60][config % 4]
    : config < 16 ? [10, 20][config % 2] : [2.5, 5, 10, 20][config % 4];
  const count = code === 0 ? 1 : code === 3 ? (packet[1] ?? 0) & 63 : 2;
  const duration = frameMs * count * 1000;
  if (!duration || duration > 120000) throw new Error('Invalid Opus packet duration');
  return duration;
}
