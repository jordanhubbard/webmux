import { test } from 'node:test';
import assert from 'node:assert/strict';
import { federationFrameFacts, federationRouteFacts } from '../tests/e2e/federation-diagnostic-facts.mts';

test('federation frame diagnostics retain protocol facts without credential, path or content fields', () => {
  const secret = 'FAKE_PRIVATE_SENTINEL_478';
  const payload = JSON.stringify({ type: 'status', state: 'connected', data: secret,
    viewer_id: secret, snapshot_token: secret, snapshot_profile: secret, message: secret,
    url: 'https://private.invalid/?token=' + secret, transcript_file: '/private/' + secret, cols: 80, rows: 24 });
  const value = federationFrameFacts(payload);
  assert.deepEqual(value, { bytes: Buffer.byteLength(payload), type: 'status', state: 'connected',
    viewer: true, profile: true, token: true, dataBytes: Buffer.byteLength(secret), cols: 80, rows: 24 });
  assert(!JSON.stringify(value).includes(secret));
  for (const raw of [JSON.stringify({ type: secret, state: secret, data: secret }),
    JSON.stringify({ type: { toString: null }, state: { toString: null } }), secret,
    JSON.stringify({ type: 'output', data: secret.repeat(4000) })]) {
    assert(!JSON.stringify(federationFrameFacts(raw)).includes(secret));
  }
  assert.equal(federationFrameFacts('a'.repeat(65537)).type, 'oversize');
});

test('federation URL diagnostics project only recognized roles and discard origin, credentials, IDs and queries', () => {
  const secret = 'FAKE_PRIVATE_SENTINEL_478';
  assert.equal(federationRouteFacts(`wss://user:${secret}@private.invalid/api/term/${secret}?token=${secret}`), 'direct');
  assert.equal(federationRouteFacts(`wss://private.invalid/api/servers/local/term/${secret}?token=${secret}`), 'federated-local');
  assert.equal(federationRouteFacts(`wss://private.invalid/api/servers/${secret}/term/${secret}?token=${secret}`), 'federated-peer');
  for (const address of ['not a URL', 'https://private.invalid/api/keys', 'https://private.invalid/api/term/a/b']) {
    assert.equal(federationRouteFacts(address), undefined);
  }
});
