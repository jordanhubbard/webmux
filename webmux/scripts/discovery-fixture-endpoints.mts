import net from 'node:net';

export function discoveryFixtureFamily(choice: string | undefined, ...addresses: Array<string | undefined>): 4 | 6 {
  const family = choice === undefined || choice === 'ipv4' ? 4 : choice === 'ipv6' ? 6 : 0;
  if (!family || addresses.length !== 2) throw new Error('Select ipv4 or ipv6 and two matching native LAN addresses');
  const excluded = new net.BlockList();
  for (const [address, prefix] of [['::', 128], ['::1', 128], ['fe80::', 10], ['ff00::', 8], ['::ffff:0:0', 96]] as const) {
    excluded.addSubnet(address, prefix, 'ipv6');
  }
  for (const address of addresses) {
    if (!address || net.isIP(address) !== family || address.includes('%')
      || (family === 4 && (address.startsWith('127.') || address === '0.0.0.0'))
      || (family === 6 && excluded.check(address, 'ipv6'))) {
      throw new Error('Require matching native IPv4 or unscoped global/ULA IPv6 addresses; loopback, unspecified, mapped and link-local IPv6 are not qualification targets');
    }
  }
  return family;
}

export function fixtureHostPort(address: string, port: number): string {
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('Invalid fixture listener port');
  return `${net.isIP(address) === 6 ? `[${address}]` : address}:${port}`;
}
