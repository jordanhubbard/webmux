export interface GridItem {
  row: number;
  col: number;
}

/**
 * Given existing items in the grid and optional requested position,
 * returns the next available (row, col) position.
 */
export function nextPositionFor(
  items: GridItem[],
  requestedRow?: number,
  requestedCol?: number
): { row: number; col: number } {
  if (requestedRow !== undefined && requestedCol !== undefined) {
    return { row: requestedRow, col: requestedCol };
  }

  if (items.length === 0) return { row: 0, col: 0 };

  const maxRow = Math.max(...items.map(s => s.row));
  const rowItems = items.filter(s => s.row === maxRow);
  const maxCol = Math.max(...rowItems.map(s => s.col));
  return { row: maxRow, col: maxCol + 1 };
}

/**
 * After removing an item, compact positions so there are no gaps.
 * Returns updated items with renumbered positions (mutates in place).
 */
export function compactPositions<T extends GridItem>(items: T[]): T[] {
  // Group by row, sort within each row by col
  const rowMap = new Map<number, T[]>();
  for (const item of items) {
    const row = rowMap.get(item.row) || [];
    row.push(item);
    rowMap.set(item.row, row);
  }
  const sortedRows = Array.from(rowMap.keys()).sort((a, b) => a - b);
  let newRow = 0;
  for (const oldRow of sortedRows) {
    const rowItems = rowMap.get(oldRow)!.sort((a, b) => a.col - b.col);
    rowItems.forEach((item, newCol) => {
      item.row = newRow;
      item.col = newCol;
    });
    newRow++;
  }
  return items;
}
