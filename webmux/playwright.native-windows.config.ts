import { defineConfig } from '@playwright/test';
import base from './playwright.config';

if (process.platform !== 'win32') {
  throw new Error('Native Windows provider qualification requires a Windows host');
}

// This gate deliberately fails on restrictive hosted jobs. Run it on the
// selected native target; a skipped fixture test cannot establish acceptance.
export default defineConfig({
  ...base,
  testMatch: 'windows-local-provider.native.spec.ts',
  testIgnore: [],
});
