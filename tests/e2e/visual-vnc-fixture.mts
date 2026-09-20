// Minimal loopback RFB 3.8 server for exercising the real noVNC renderer.
// Wire layouts follow RFC 6143; this fixture offers only no-auth/raw pixels.
import assert from 'node:assert/strict';
import { createServer, type Socket } from 'node:net';
import { once } from 'node:events';

export async function startVisualVnc() {
  const sockets = new Set<Socket>();
  const keys: number[] = [];
  const pointers: number[] = [];
  const errors: string[] = [];
  const width = 320, height = 200;
  const server = createServer(socket => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
    socket.on('error', error => { errors.push(error.message); });
    let pending = Buffer.alloc(0), stage = 0, sent = false;
    let redShift = 16, greenShift = 8, blueShift = 0, bigEndian = false;
    socket.write('RFB 003.008\n');
    function frame() {
      const result = Buffer.alloc(16 + width * height * 4);
      result.writeUInt16BE(1, 2);
      result.writeUInt16BE(width, 8); result.writeUInt16BE(height, 10);
      for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
        const red = x < 160 ? 48 : 192, green = y < 100 ? 80 : 176, blue = (x + y) % 64 < 32 ? 112 : 224;
        const pixel = ((red << redShift) | (green << greenShift) | (blue << blueShift)) >>> 0;
        const offset = 16 + (y * width + x) * 4;
        if (bigEndian) result.writeUInt32BE(pixel, offset); else result.writeUInt32LE(pixel, offset);
      }
      socket.write(result); sent = true;
    }
    socket.on('data', chunk => {
      try {
        pending = Buffer.concat([pending, chunk]);
        while (pending.length) {
          let length: number;
          if (stage === 0) length = 12;
          else if (stage < 3) length = 1;
          else {
            const type = pending[0];
            if (type === 0) length = 20;
            else if (type === 2) { if (pending.length < 4) return; length = 4 + pending.readUInt16BE(2) * 4; }
            else if (type === 3) length = 10;
            else if (type === 4) length = 8;
            else if (type === 5) length = 6;
            else if (type === 6) { if (pending.length < 8) return; length = 8 + pending.readUInt32BE(4); }
            else throw new Error(`Unexpected RFB client message ${type}`);
          }
          assert(length <= 1024 * 1024, 'Unexpected fixture message length');
          if (pending.length < length) return;
          const message = pending.subarray(0, length); pending = pending.subarray(length);
          if (stage === 0) {
            assert.equal(message.toString(), 'RFB 003.008\n'); socket.write(Buffer.from([1, 1])); stage++;
          } else if (stage === 1) {
            assert.equal(message[0], 1); socket.write(Buffer.alloc(4)); stage++;
          } else if (stage === 2) {
            const name = Buffer.from('Visual parity desktop');
            const init = Buffer.alloc(24);
            init.writeUInt16BE(width, 0); init.writeUInt16BE(height, 2);
            init[4] = 32; init[5] = 24; init[7] = 1;
            for (const offset of [8, 10, 12]) init.writeUInt16BE(255, offset);
            init[14] = redShift; init[15] = greenShift; init[16] = blueShift;
            init.writeUInt32BE(name.length, 20);
            socket.write(Buffer.concat([init, name])); stage++;
          } else if (message[0] === 0) {
            assert.equal(message[4], 32); assert.equal(message[7], 1);
            for (const offset of [8, 10, 12]) assert.equal(message.readUInt16BE(offset), 255);
            bigEndian = message[6] !== 0;
            redShift = message[14]; greenShift = message[15]; blueShift = message[16];
          } else if (message[0] === 3 && (!sent || message[1] === 0)) frame();
          else if (message[0] === 4 && message[1] === 1) keys.push(message.readUInt32BE(4));
          else if (message[0] === 5) pointers.push(message[1]);
        }
      } catch (error) { errors.push(String(error)); socket.destroy(); }
    });
  });
  // Fixed fixture port keeps the unmodified UI's host:port title deterministic.
  server.listen(15909, '127.0.0.1'); await once(server, 'listening');
  return { port: 15909, keys, pointers, errors, close: async () => {
    for (const socket of sockets) socket.destroy();
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  } };
}
