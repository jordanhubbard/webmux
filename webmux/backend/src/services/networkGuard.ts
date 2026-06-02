import { lookup } from 'dns/promises';

const ALLOW_LOCAL = process.env.WEBMUX_ALLOW_LOCAL_TARGETS === '1';

function ipv4Octets(ip: string): number[] | null {
  const m = ip.match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)$/);
  if (!m) return null;
  const octs = m.slice(1, 5).map(Number);
  if (octs.some(o => o < 0 || o > 255)) return null;
  return octs;
}

function isBlockedIpv4(ip: string): boolean {
  const octs = ipv4Octets(ip);
  if (!octs) return false;
  const [a, b] = octs;
  if (a === 127) return true;               // loopback 127.0.0.0/8
  if (a === 0) return true;                  // 0.0.0.0/8 reserved "this network"
  if (a === 169 && b === 254) return true;   // link-local 169.254.0.0/16
  if (a >= 224) return true;                 // multicast 224/4 + reserved 240/4 + broadcast
  return false;
}

function isBlockedIpv6(ip: string): boolean {
  const lower = ip.toLowerCase();
  if (lower === '::1') return true;          // loopback
  if (lower === '::') return true;           // unspecified
  if (lower.startsWith('fe80:') || lower.startsWith('fe80::')) return true;  // link-local
  if (lower.startsWith('ff')) return true;   // multicast ff00::/8
  // IPv4-mapped IPv6 (::ffff:a.b.c.d) — re-check as IPv4 to catch ::ffff:127.0.0.1 etc.
  const mapped = lower.match(/^::ffff:([0-9.]+)$/);
  if (mapped) return isBlockedIpv4(mapped[1]);
  return false;
}

export function isBlockedAddress(ip: string): boolean {
  if (ALLOW_LOCAL) return false;
  if (ip.includes(':')) return isBlockedIpv6(ip);
  return isBlockedIpv4(ip);
}

// Resolves hostname to an IP and rejects if the IP is in a blocked range.
// Returns the resolved IP so callers can connect by IP and avoid DNS rebinding
// (the hostname could resolve to a different IP on a second lookup).
export async function resolveAndValidateTarget(hostname: string): Promise<string> {
  let host = hostname;
  if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1);
  const { address } = await lookup(host);
  if (isBlockedAddress(address)) {
    throw new Error(`Target blocked (loopback/link-local/reserved): ${address}`);
  }
  return address;
}
