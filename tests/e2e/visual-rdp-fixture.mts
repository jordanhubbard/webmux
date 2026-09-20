// Loopback Guacamole protocol fixture; drawing follows Apache's protocol reference.
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { createServer, type Socket } from 'node:net';

function instruction(...values: string[]): string {
  return values.map(value => `${[...value].length}.${value}`).join(',') + ';';
}

export async function startVisualRdp() {
  const sockets = new Set<Socket>();
  const keys: string[][] = [], pointers: string[][] = [], errors: string[] = [];
  const server = createServer(socket => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
    socket.on('error', error => errors.push(error.message));
    socket.setEncoding('utf8');
    let buffer = '', selected = false, connected = false;
    socket.on('data', chunk => {
      try {
        buffer += chunk;
        assert(buffer.length <= 1024 * 1024, 'Fixture instruction exceeds limit');
        while (buffer) {
          const chars = [...buffer], values: string[] = [];
          let offset = 0;
          for (;;) {
            const dot = chars.indexOf('.', offset);
            if (dot === -1) return;
            const length = chars.slice(offset, dot).join('');
            assert(/^\d+$/.test(length));
            const end = dot + 1 + Number(length);
            assert(end <= 1024 * 1024);
            if (chars.length <= end) return;
            values.push(chars.slice(dot + 1, end).join(''));
            const delimiter = chars[end];
            assert(delimiter === ',' || delimiter === ';');
            offset = end + 1;
            if (delimiter === ';') break;
          }
          buffer = chars.slice(offset).join('');
          const [opcode, ...args] = values;
          if (!selected) {
            assert.deepEqual(values, ['select', 'rdp']); selected = true;
            socket.write(instruction('args', 'hostname', 'port', 'username', 'password', 'domain'));
          } else if (!connected) {
            if (opcode !== 'connect') continue; // size/audio/video/image capabilities
            assert.deepEqual(args, ['127.0.0.1', '3389', 'visual-user', '', '']);
            connected = true;
            socket.write(instruction('ready', 'visual-desktop') + instruction('size', '0', '640', '400')
              + instruction('rect', '0', '0', '0', '640', '400') + instruction('cfill', '12', '0', '48', '80', '112', '255')
              + instruction('rect', '0', '80', '60', '480', '280') + instruction('cfill', '14', '0', '176', '96', '208', '255')
              + instruction('sync', '1'));
          } else if (opcode === 'key') keys.push(args);
          else if (opcode === 'mouse') pointers.push(args);
          else if (opcode === 'disconnect') socket.end();
        }
      } catch (error) { errors.push(String(error)); socket.destroy(); }
    });
  });
  server.listen(14822, '127.0.0.1'); await once(server, 'listening');
  return { keys, pointers, errors, close: async () => {
    for (const socket of sockets) socket.destroy();
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  } };
}
