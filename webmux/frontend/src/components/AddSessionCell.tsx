import './AddSessionCell.css';

interface AddSessionCellProps {
  row: number;
  col: number;
  isEmpty: boolean;
  onClick: () => void;
}

export function AddSessionCell({ row, col, isEmpty, onClick }: AddSessionCellProps) {
  return (
    <button type="button" className="add-session-cell" onClick={onClick}
      style={{ gridColumn: col + 1, gridRow: row + 1 }}
      aria-label={`Add session at row ${row + 1}, column ${col + 1}`}
      data-testid={`add-cell-${row}-${col}`}>
      <span aria-hidden="true" style={{ fontSize: isEmpty ? 64 : 36, lineHeight: 1 }}>+</span>
      {isEmpty && <span className="add-session-cell-hint">Click to add a session</span>}
    </button>
  );
}
