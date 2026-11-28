import test from 'node:test';
import assert from 'node:assert/strict';
import { discoveryFixtureFamily, fixtureHostPort } from './discovery-fixture-endpoints.mts';

test('fixture family retains IPv4 default and requires a matching explicit IPv6 pair', () => {
  assert.equal(discoveryFixtureFamily(undefined, '192.0.2.1', '192.0.2.2'), 4);
  assert.equal(discoveryFixtureFamily('ipv4', '192.0.2.1', '192.0.2.2'), 4);
  assert.equal(discoveryFixtureFamily('ipv6', 'fd00::1', '2001:db8::2'), 6);
  for (const [choice, first, second] of [
    ['invalid', '192.0.2.1', '192.0.2.2'], ['ipv4', 'fd00::1', 'fd00::2'],
    ['ipv6', 'fd00::1', '192.0.2.2'], ['ipv6', undefined, 'fd00::2'],
  ]) assert.throws(() => discoveryFixtureFamily(choice, first, second));
});

test('IPv6 qualification excludes loopback, unspecified, mapped, multicast and scoped/link-local targets', () => {
  for (const address of ['::', '::1', '0:0:0:0:0:0:0:1', 'fe80::1', 'fe80::1%eth0', 'ff02::fb', '::ffff:192.0.2.1', '[fd00::1]']) {
    assert.throws(() => discoveryFixtureFamily('ipv6', address, 'fd00::2'), address);
  }
});

test('socket and SSH forwarding endpoint syntax keeps IPv6 brackets and IPv4 unchanged', () => {
  assert.equal(fixtureHostPort('192.0.2.1', 443), '192.0.2.1:443');
  assert.equal(fixtureHostPort('fd00::1', 443), '[fd00::1]:443');
  assert.equal(`127.0.0.1:1234:${fixtureHostPort('fd00::1', 443)}`, '127.0.0.1:1234:[fd00::1]:443');
  for (const port of [0, -1, 65536, 1.5]) assert.throws(() => fixtureHostPort('fd00::1', port));
});
