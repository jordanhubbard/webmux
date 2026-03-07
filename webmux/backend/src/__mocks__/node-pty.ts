import { EventEmitter } from 'events';

class MockPty extends EventEmitter {
  pid = 1234;
  cols = 80;
  rows = 24;

  write(_data: string): void {}
  resize(_cols: number, _rows: number): void {}
  kill(_signal?: string): void {}
  onData(callback: (data: string) => void): void {
    this.on('data', callback);
  }
  onExit(callback: (e: { exitCode: number; signal?: number }) => void): void {
    this.on('exit', callback);
  }
}

export function spawn(_cmd: string, _args: string[], _opts: Record<string, unknown>): MockPty {
  return new MockPty();
}
