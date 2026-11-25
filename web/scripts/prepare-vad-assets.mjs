import { copyFileSync, mkdirSync, readdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const vad = join(root, 'node_modules/@ricky0123/vad-web');
const ort = join(root, 'node_modules/onnxruntime-web');
const target = join(root, 'public/vad');

const vadFiles = ['silero_vad_v5.onnx', 'vad.worklet.bundle.min.js'];
const ortFiles = readdirSync(join(ort, 'dist'))
  .filter((name) => /^ort-wasm.*\.(wasm|mjs)$/.test(name));
if (!ortFiles.some((name) => name.endsWith('.wasm')) ||
    !ortFiles.some((name) => name.endsWith('.mjs'))) {
  throw new Error('ONNX Runtime WASM binaries or module loaders missing');
}
rmSync(target, { recursive: true, force: true });
mkdirSync(target, { recursive: true });
for (const name of vadFiles) copyFileSync(join(vad, 'dist', name), join(target, name));
for (const name of ortFiles) copyFileSync(join(ort, 'dist', name), join(target, name));
console.log('Prepared local VAD assets:', [...vadFiles, ...ortFiles].join(', '));
