// In-memory credential store for password-based logins (never persisted to disk)
const store = new Map<string, { username: string; password: string; created_at: number }>();

const TTL_MS = 5 * 60 * 1000; // 5 minutes

export class CredentialHandler {
  store(sessionId: string, username: string, password: string): void {
    store.set(sessionId, { username, password, created_at: Date.now() });
    // Auto-clear after TTL; unref so the timer doesn't block process exit
    const t = setTimeout(() => this.clear(sessionId), TTL_MS);
    if (t.unref) t.unref();
  }

  get(sessionId: string): { username: string; password: string } | undefined {
    const entry = store.get(sessionId);
    if (!entry) return undefined;
    if (Date.now() - entry.created_at > TTL_MS) {
      store.delete(sessionId);
      return undefined;
    }
    return { username: entry.username, password: entry.password };
  }

  clear(sessionId: string): void {
    const entry = store.get(sessionId);
    if (entry) {
      // Zero out sensitive data
      entry.password = '';
      entry.username = '';
      store.delete(sessionId);
    }
  }

  clearAll(): void {
    store.forEach((entry) => {
      entry.password = '';
      entry.username = '';
    });
    store.clear();
  }
}

export const credentialHandler = new CredentialHandler();
