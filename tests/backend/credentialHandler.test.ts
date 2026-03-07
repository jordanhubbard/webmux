import { CredentialHandler } from '@backend/services/credentialHandler';

describe('CredentialHandler', () => {
  let handler: CredentialHandler;

  beforeEach(() => {
    handler = new CredentialHandler();
  });

  afterEach(() => {
    handler.clearAll();
  });

  it('stores and retrieves credentials', () => {
    handler.store('sess-001', 'admin', 'secret123');
    const creds = handler.get('sess-001');
    expect(creds).toBeDefined();
    expect(creds!.username).toBe('admin');
    expect(creds!.password).toBe('secret123');
  });

  it('returns undefined for unknown session', () => {
    expect(handler.get('nonexistent')).toBeUndefined();
  });

  it('clears credentials for a session', () => {
    handler.store('sess-002', 'user', 'pass');
    handler.clear('sess-002');
    expect(handler.get('sess-002')).toBeUndefined();
  });

  it('clearAll removes all stored credentials', () => {
    handler.store('sess-003', 'u1', 'p1');
    handler.store('sess-004', 'u2', 'p2');
    handler.clearAll();
    expect(handler.get('sess-003')).toBeUndefined();
    expect(handler.get('sess-004')).toBeUndefined();
  });
});
