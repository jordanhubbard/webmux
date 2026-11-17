import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rdpPixelMatches } from './rdp-pixels.ts';

test('RDP frame admission distinguishes lossy-codec rounding from a stale or absent frame', () => {
  const blue = [0, 0, 255, 255], green = [0, 255, 0, 255];
  assert.equal(rdpPixelMatches([0, 0, 253, 255], blue), false);
  for (const codec of ['jpeg', 'remotefx'] as const) {
    assert.equal(rdpPixelMatches([0, 0, 253, 255], blue, codec), true);
    assert.equal(rdpPixelMatches([0, 0, 251, 255], blue, codec), false);
    assert.equal(rdpPixelMatches([0, 254, 1, 255], green, codec), true);
    for (const invalid of [[], [0, 0, 0, 255], blue, [0, 255, 0, 0], [0, 255, 0, 254]]) {
      assert.equal(rdpPixelMatches(invalid, green, codec), false);
    }
  }
  assert.equal(rdpPixelMatches(green, green), true);
});
