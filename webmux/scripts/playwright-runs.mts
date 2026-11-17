import path from 'node:path';

/** Turn Playwright's own discovery into complete, isolated file invocations. */
export function isolatedPlaywrightRuns(report: unknown, outputRoot = 'test-results'): string[][] {
  if (!report || typeof report !== 'object') throw new Error('Invalid Playwright discovery');
  const { config, errors, suites } = report as {
    config?: { rootDir?: unknown }; errors?: unknown; suites?: unknown;
  };
  if (errors !== undefined && (!Array.isArray(errors) || errors.length)) throw new Error('Playwright discovery reported errors');
  if (typeof config?.rootDir !== 'string' || !config.rootDir || !Array.isArray(suites) || !suites.length) throw new Error('Playwright discovery has no test files');
  const files = new Set<string>();
  for (const suite of suites) {
    if (!suite || typeof suite !== 'object' || !('file' in suite) || typeof suite.file !== 'string' || !suite.file) {
      throw new Error('Playwright discovery contains a suite without a file');
    }
    files.add(path.resolve(config.rootDir, suite.file));
  }
  return [...files].map((file, index) => [
    `^${file.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`,
    `--output=${path.join(outputRoot, `spec-${index + 1}`)}`,
  ]);
}

/** Playwright clears its output directory at startup. Never implicitly reuse
 * the parent that contains another invocation's diagnostics. */
export function withPlaywrightOutput(args: string[], outputRoot: string): string[] {
  const separator = args.indexOf('--');
  const options = separator < 0 ? args : args.slice(0, separator);
  if (options.some(arg => arg === '--output' || arg.startsWith('--output='))) return args;
  return [`--output=${outputRoot}`, ...args];
}
