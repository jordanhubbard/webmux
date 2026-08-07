export default function globalTeardown() {
  // Test-home cleanup happens in global setup, before the next web server is
  // started. Removing watched files here crashes Node's Windows FSWatcher.
}
