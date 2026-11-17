import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rdpPixelMatches } from './rdp-pixels.ts';

test('RDP frame admission distinguishes JPEG rounding from a stale or absent frame', () => {
  const blue = [0, 0, 255, 255], green = [0, 255, 0, 255];
  assert.equal(rdpPixelMatches([0, 0, 253, 255], blue, false), false);
  assert.equal(rdpPixelMatches([0, 0, 253, 255], blue, true), true);
  assert.equal(rdpPixelMatches([0, 0, 251, 255], blue, true), false);
  assert.equal(rdpPixelMatches([0, 254, 1, 255], green, true), true);
  for (const invalid of [[], [0, 0, 0, 255], blue, [0, 255, 0, 0], [0, 255, 0, 254]]) {
    assert.equal(rdpPixelMatches(invalid, green, true), false);
  }
  assert.equal(rdpPixelMatches(green, green, false), true);
});
