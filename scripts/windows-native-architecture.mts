import { open, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

export type Machine = 'arm64' | 'amd64' | 'ia32';
export type NativeEnvironment = { platform: string; arch: string; go: { GOOS: string; GOARCH: string; GOHOSTARCH: string } };

export function requireNativeARM64(value: NativeEnvironment): void {
  if (value.platform !== 'win32' || value.arch !== 'arm64' || value.go.GOOS !== 'windows'
      || value.go.GOARCH !== 'arm64' || value.go.GOHOSTARCH !== 'arm64') {
    throw new Error('Qualification requires native Windows ARM64 Node and Go host/target; cross-compilation is insufficient');
  }
}

export async function inspectPE(file: string): Promise<{ machine: Machine; sha256: string }> {
  const handle = await open(file, 'r');
  try {
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size < 64) throw new Error('Truncated PE image');
    const dos = Buffer.alloc(64);
    if ((await handle.read(dos, 0, 64, 0)).bytesRead !== 64 || dos.readUInt16LE(0) !== 0x5a4d) throw new Error('Missing DOS signature');
    const offset = dos.readUInt32LE(0x3c);
    if (offset < 64 || offset > stat.size - 6) throw new Error('Invalid PE header offset');
    const pe = Buffer.alloc(6);
    if ((await handle.read(pe, 0, 6, offset)).bytesRead !== 6 || pe.readUInt32LE(0) !== 0x4550) throw new Error('Missing PE signature');
    const machines: Record<number, Machine> = { 0xaa64: 'arm64', 0x8664: 'amd64', 0x14c: 'ia32' };
    const machine = machines[pe.readUInt16LE(4)];
    if (!machine) throw new Error('Unsupported PE machine');
    const hash = createHash('sha256');
    for await (const chunk of handle.createReadStream({ start: 0, autoClose: false })) hash.update(chunk);
    return { machine, sha256: hash.digest('hex') };
  } finally { await handle.close(); }
}

export async function requireARM64Image(file: string): Promise<{ machine: Machine; sha256: string }> {
  const result = await inspectPE(file);
  if (result.machine !== 'arm64') throw new Error(`Expected ARM64 image, observed ${result.machine}`);
  return result;
}

async function main(): Promise<void> {
  const args = process.argv.slice(2);
  const values = new Map<string, string>();
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index], value = args[index + 1];
    if (!['--output', '--capture', '--application', '--browser'].includes(key) || !value || value.startsWith('--') || values.has(key)) throw new Error('Expected --output FILE [--capture EXE] [--application EXE] [--browser EXE]');
    values.set(key, value);
  }
  const output = values.get('--output');
  if (!output) throw new Error('--output is required');
  const go = JSON.parse(execFileSync('go', ['env', '-json', 'GOOS', 'GOARCH', 'GOHOSTARCH', 'GOROOT'], { encoding: 'utf8' })) as NativeEnvironment['go'] & { GOROOT: string };
  const environment = { platform: process.platform, arch: process.arch, go };
  requireNativeARM64(environment);
  const images: Record<string, { machine: Machine; sha256: string }> = {
    node: await requireARM64Image(process.execPath),
    go: await requireARM64Image(path.join(go.GOROOT, 'bin', 'go.exe')),
  };
  const capture = values.get('--capture'), application = values.get('--application'), browser = values.get('--browser');
  if (capture) images.capture = await requireARM64Image(capture);
  if (application) images.application = await requireARM64Image(application);
  if (browser) images.browser = await inspectPE(browser);
  await writeFile(output, JSON.stringify({ schema: 1, platform: environment.platform, nodeArchitecture: environment.arch,
    go: { GOOS: go.GOOS, GOARCH: go.GOARCH, GOHOSTARCH: go.GOHOSTARCH }, images,
    rendererNativeARM64: browser ? images.browser.machine === 'arm64' : null,
    scope: 'Architecture admission only; actual native tests/captures and renderer results are separate evidence. No authenticated conversation acceptance.' }, null, 2) + '\n');
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch(error => { console.error(error instanceof Error ? error.message : String(error)); process.exitCode = 1; });
}
