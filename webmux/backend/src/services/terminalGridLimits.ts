import { AppConfig } from '../types';
import { GridItem, nextPositionFor } from './gridLayout';
import { persistence } from './persistenceManager';

export interface TerminalGridLimits {
  maxCols?: number;
  maxRows?: number;
}

export class TerminalGridLimitError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'TerminalGridLimitError';
  }
}

type RawGridLimit = number | string | null | undefined;

const MAX_COLS_ENV = 'WEBMUX_TERMINAL_GRID_MAX_COLS';
const MAX_ROWS_ENV = 'WEBMUX_TERMINAL_GRID_MAX_ROWS';

function normalizeGridLimit(value: RawGridLimit, label: string): number | undefined {
  if (value === undefined || value === null) return undefined;

  if (typeof value === 'string') {
    const trimmed = value.trim().toLowerCase();
    if (!trimmed || trimmed === '0' || trimmed === 'none' || trimmed === 'unlimited' || trimmed === 'infinite') {
      return undefined;
    }
    const parsed = Number(trimmed);
    if (Number.isInteger(parsed) && parsed > 0) return parsed;
  } else if (Number.isInteger(value)) {
    if (value === 0) return undefined;
    if (value > 0) return value;
  }

  throw new TerminalGridLimitError(`${label} must be a positive integer, null, 0, or unlimited`);
}

function envGridLimit(name: string, label: string): { present: boolean; value?: number } {
  if (!(name in process.env)) return { present: false };
  return { present: true, value: normalizeGridLimit(process.env[name], label) };
}

export function terminalGridLimitsFromApp(config: AppConfig): TerminalGridLimits {
  return {
    maxCols: normalizeGridLimit(config.app.terminal_grid?.max_cols, 'app.terminal_grid.max_cols'),
    maxRows: normalizeGridLimit(config.app.terminal_grid?.max_rows, 'app.terminal_grid.max_rows'),
  };
}

export function effectiveTerminalGridLimits(config: AppConfig): TerminalGridLimits {
  const configured = terminalGridLimitsFromApp(config);
  const envMaxCols = envGridLimit(MAX_COLS_ENV, MAX_COLS_ENV);
  const envMaxRows = envGridLimit(MAX_ROWS_ENV, MAX_ROWS_ENV);

  return {
    maxCols: envMaxCols.present ? envMaxCols.value : configured.maxCols,
    maxRows: envMaxRows.present ? envMaxRows.value : configured.maxRows,
  };
}

export function appConfigWithEffectiveTerminalGridLimits(config: AppConfig): AppConfig {
  const limits = effectiveTerminalGridLimits(config);
  return {
    app: {
      ...config.app,
      terminal_grid: {
        ...config.app.terminal_grid,
        max_cols: limits.maxCols ?? null,
        max_rows: limits.maxRows ?? null,
      },
    },
  };
}

export function loadTerminalGridLimits(): TerminalGridLimits {
  try {
    return effectiveTerminalGridLimits(persistence.loadApp());
  } catch (err) {
    if (err instanceof TerminalGridLimitError) throw err;
    const envMaxCols = envGridLimit(MAX_COLS_ENV, MAX_COLS_ENV);
    const envMaxRows = envGridLimit(MAX_ROWS_ENV, MAX_ROWS_ENV);
    return {
      maxCols: envMaxCols.present ? envMaxCols.value : undefined,
      maxRows: envMaxRows.present ? envMaxRows.value : undefined,
    };
  }
}

export function isTerminalGridLimitError(err: unknown): err is TerminalGridLimitError {
  return err instanceof TerminalGridLimitError;
}

export function assertTerminalGridPosition(
  row: number,
  col: number,
  limits: TerminalGridLimits = loadTerminalGridLimits(),
): void {
  if (!Number.isInteger(row) || !Number.isInteger(col) || row < 0 || col < 0) {
    throw new TerminalGridLimitError('Terminal grid row and col must be non-negative integers');
  }

  if (limits.maxRows !== undefined && row >= limits.maxRows) {
    throw new TerminalGridLimitError(`Terminal grid row ${row} exceeds max_rows ${limits.maxRows}`);
  }

  if (limits.maxCols !== undefined && col >= limits.maxCols) {
    throw new TerminalGridLimitError(`Terminal grid column ${col} exceeds max_cols ${limits.maxCols}`);
  }
}

export function nextTerminalGridPosition(
  items: GridItem[],
  requestedRow?: number,
  requestedCol?: number,
  limits: TerminalGridLimits = loadTerminalGridLimits(),
): { row: number; col: number } {
  if (requestedRow !== undefined && requestedCol !== undefined) {
    assertTerminalGridPosition(requestedRow, requestedCol, limits);
    return { row: requestedRow, col: requestedCol };
  }

  if (limits.maxCols === undefined && limits.maxRows === undefined) {
    return nextPositionFor(items);
  }

  if (items.length === 0) {
    assertTerminalGridPosition(0, 0, limits);
    return { row: 0, col: 0 };
  }

  const occupied = new Set(items.map(item => `${item.row},${item.col}`));

  if (limits.maxCols !== undefined) {
    const maxExistingRow = Math.max(...items.map(item => item.row));
    const rowSearchLimit = limits.maxRows ?? maxExistingRow + 2;

    for (let row = 0; row < rowSearchLimit; row++) {
      for (let col = 0; col < limits.maxCols; col++) {
        if (!occupied.has(`${row},${col}`)) return { row, col };
      }
    }

    throw new TerminalGridLimitError('Terminal grid is full');
  }

  const inBoundsItems = items.filter(item => limits.maxRows === undefined || item.row < limits.maxRows);
  if (inBoundsItems.length === 0) {
    assertTerminalGridPosition(0, 0, limits);
    return { row: 0, col: 0 };
  }

  const maxRow = Math.max(...inBoundsItems.map(item => item.row));
  const rowItems = inBoundsItems.filter(item => item.row === maxRow);
  const maxCol = Math.max(...rowItems.map(item => item.col));
  const candidate = { row: maxRow, col: maxCol + 1 };
  assertTerminalGridPosition(candidate.row, candidate.col, limits);
  return candidate;
}
