/** Lossy RDP/Guacamole codecs may round a saturated fixture colour by a few 8-bit channel levels.
 * Lossless transport and alpha remain exact; stale colours and empty canvases never match. */
export function rdpPixelMatches(rgba: number[], expected: number[], codec?: 'jpeg' | 'remotefx'): boolean {
  return rgba.length === 4 && expected.length === 4 && rgba[3] === expected[3]
    && rgba.slice(0, 3).every((value, index) => Number.isInteger(value)
      && value >= 0 && value <= 255 && Math.abs(value - expected[index]) <= (codec ? 3 : 0));
}
