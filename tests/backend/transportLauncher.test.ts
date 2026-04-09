import { TransportLauncher } from '@backend/services/transportLauncher';
import { Session } from '@backend/types';

// node-pty is mocked via jest.config.js moduleNameMapper

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 'sess-001',
    owner: 'testuser',
    transport: 'ssh',
    host_id: '',
    hostname: 'example.com',
    port: 22,
    username: 'testuser',
    key_id: '',
    cols: 80,
    rows: 24,
    row: 0,
    col: 0,
    state: 'connecting',
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    title: 'testuser@example.com',
    persistent: true,
    minimized: false,
    ...overrides,
  };
}

describe('TransportLauncher', () => {
  let launcher: TransportLauncher;

  beforeEach(() => {
    launcher = new TransportLauncher();
  });

  afterEach(() => {
    // Clean up all handles
    launcher.kill('sess-001');
    launcher.kill('sess-002');
  });

  it('launches SSH session and stores handle', () => {
    const session = makeSession();
    const pty = launcher.launch(session);
    expect(pty).toBeDefined();
    expect(launcher.isAlive('sess-001')).toBe(true);
  });

  it('getHandle returns the pty for a known session', () => {
    const session = makeSession();
    const pty = launcher.launch(session);
    expect(launcher.getHandle('sess-001')).toBe(pty);
  });

  it('getHandle returns undefined for unknown session', () => {
    expect(launcher.getHandle('nonexistent')).toBeUndefined();
  });

  it('kill removes the handle', () => {
    launcher.launch(makeSession());
    launcher.kill('sess-001');
    expect(launcher.isAlive('sess-001')).toBe(false);
    expect(launcher.getHandle('sess-001')).toBeUndefined();
  });

  it('kill is safe for unknown session', () => {
    expect(() => launcher.kill('nonexistent')).not.toThrow();
  });

  it('resize is safe for unknown session', () => {
    expect(() => launcher.resize('nonexistent', 120, 40)).not.toThrow();
  });

  it('resize calls resize on the pty', () => {
    const session = makeSession();
    const pty = launcher.launch(session);
    const spy = jest.spyOn(pty, 'resize');
    launcher.resize('sess-001', 120, 40);
    expect(spy).toHaveBeenCalledWith(120, 40);
  });

  it('launches mosh transport when binary is available', () => {
    const session = makeSession({ transport: 'mosh' });
    // This will either succeed (mosh installed) or throw (mosh not installed)
    // We test both paths
    try {
      const pty = launcher.launch(session);
      expect(pty).toBeDefined();
    } catch (err) {
      expect((err as Error).message).toContain('mosh is not installed');
    }
  });

  it('launches multiple sessions independently', () => {
    launcher.launch(makeSession({ id: 'sess-001' }));
    launcher.launch(makeSession({ id: 'sess-002' }));
    expect(launcher.isAlive('sess-001')).toBe(true);
    expect(launcher.isAlive('sess-002')).toBe(true);
    launcher.kill('sess-001');
    expect(launcher.isAlive('sess-001')).toBe(false);
    expect(launcher.isAlive('sess-002')).toBe(true);
  });
});
