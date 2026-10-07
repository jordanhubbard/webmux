import { describe, it, expect } from 'vitest';
import { handoffLink } from '@frontend/utils/authHandoff';

describe('authentication link routing', () => {
  it('recognizes device links by exact HTTPS origin/path', () => {
    expect(handoffLink('https://github.com/login/device')?.device).toBe(true);
    expect(handoffLink('https://github.com.evil/login/device')?.device).toBe(false);
    expect(handoffLink('http://github.com/login/device')?.device).toBe(false);
  });
  it('offers relay only for state-bound explicit loopback redirects', () => {
    const link = 'https://provider.example/authorize?redirect_uri=' + encodeURIComponent('http://localhost:1455/callback');
    expect(handoffLink(link)?.relay).toBe(false);
    expect(handoffLink(link + '&state=secret')?.relay).toBe(true);
    expect(handoffLink(link + '&state=secret&state=second')?.relay).toBe(false);
    expect(handoffLink(link.replace('1455', '22') + '&state=secret')?.relay).toBe(false);
    expect(handoffLink('http://127.0.0.1:1234/')?.loopback).toBe(true);
  });
  it('rejects active schemes and embedded credentials', () => {
    for (const url of ['javascript:alert(1)', 'data:text/html,hello', 'file:///etc/passwd', 'https://user:pass@example.com', 'https://exam\nple.com']) {
      expect(handoffLink(url)).toBeNull();
    }
  });
});
