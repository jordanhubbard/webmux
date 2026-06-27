export const TERMINAL_CELL_WIDTH_RATIO = 0.602;
export const TERMINAL_CELL_HEIGHT_RATIO = 1.2;

const MIN_COLS = 40;
const MIN_ROWS = 10;

export interface TerminalSize {
  cols: number;
  rows: number;
}

export function fitTerminalSizeToPixels(
  configuredCols: number,
  configuredRows: number,
  fontSize: number,
  width: number,
  height: number,
): TerminalSize {
  if (width <= 0 || height <= 0 || fontSize <= 0) {
    return { cols: configuredCols, rows: configuredRows };
  }

  const fittedCols = Math.max(MIN_COLS, Math.floor(width / (fontSize * TERMINAL_CELL_WIDTH_RATIO)));
  const fittedRows = Math.max(MIN_ROWS, Math.floor(height / (fontSize * TERMINAL_CELL_HEIGHT_RATIO)));
  return {
    cols: Math.min(configuredCols, fittedCols),
    rows: Math.min(configuredRows, fittedRows),
  };
}
