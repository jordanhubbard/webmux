import { useEffect, useRef, useState, useCallback, RefObject } from 'react';
import type { Session } from '../types';

interface WorkspaceMinimapProps {
  scrollRef: RefObject<HTMLDivElement | null>;
  sessions: Session[];
  numCols: number;
  numRows: number;
  // Tile pixel dimensions INCLUDING neither gap nor padding — intrinsic cell size.
  tileWidth: number;
  tileHeight: number;
  gap: number;
}

const MINIMAP_W = 180;
const MINIMAP_H_MAX = 140;
const PAD = 12;

export function WorkspaceMinimap({ scrollRef, sessions, numCols, numRows, tileWidth, tileHeight, gap }: WorkspaceMinimapProps) {
  const [viewport, setViewport] = useState({ x: 0, y: 0, w: 0, h: 0 });
  const [visible, setVisible] = useState(false);
  const svgRef = useRef<SVGSVGElement>(null);

  const gridW = numCols * tileWidth + (numCols + 1) * gap;
  const gridH = numRows * tileHeight + (numRows + 1) * gap;

  // Derive minimap pixel size: preserve grid aspect ratio, cap dimensions.
  const aspect = gridH / Math.max(1, gridW);
  const mmW = MINIMAP_W;
  const mmH = Math.max(50, Math.min(MINIMAP_H_MAX, Math.round(MINIMAP_W * aspect)));
  const scaleX = mmW / Math.max(1, gridW);
  const scaleY = mmH / Math.max(1, gridH);

  // Track scroll + size changes on the outer scroll container.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const sync = () => {
      setViewport({ x: el.scrollLeft, y: el.scrollTop, w: el.clientWidth, h: el.clientHeight });
      setVisible(el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1);
    };
    sync();
    el.addEventListener('scroll', sync, { passive: true });
    const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(sync) : null;
    ro?.observe(el);
    window.addEventListener('resize', sync);
    return () => {
      el.removeEventListener('scroll', sync);
      window.removeEventListener('resize', sync);
      ro?.disconnect();
    };
  }, [scrollRef, sessions.length, numCols, numRows]);

  const jumpTo = useCallback((clientX: number, clientY: number) => {
    const el = scrollRef.current;
    const svg = svgRef.current;
    if (!el || !svg) return;
    const rect = svg.getBoundingClientRect();
    // Map click in minimap-pixels back to grid-pixels, then center viewport.
    const mx = (clientX - rect.left) / scaleX;
    const my = (clientY - rect.top) / scaleY;
    const targetLeft = Math.max(0, Math.min(el.scrollWidth - el.clientWidth, mx - el.clientWidth / 2));
    const targetTop = Math.max(0, Math.min(el.scrollHeight - el.clientHeight, my - el.clientHeight / 2));
    el.scrollTo({ left: targetLeft, top: targetTop, behavior: 'smooth' });
  }, [scrollRef, scaleX, scaleY]);

  const handleMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    jumpTo(e.clientX, e.clientY);
    const onMove = (me: MouseEvent) => jumpTo(me.clientX, me.clientY);
    const onUp = () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  };

  if (!visible || sessions.length === 0) return null;

  // Viewport rect clamped so it stays visible at the edges of the minimap.
  const vpX = viewport.x * scaleX;
  const vpY = viewport.y * scaleY;
  const vpW = Math.max(4, Math.min(mmW - 1, viewport.w * scaleX));
  const vpH = Math.max(4, Math.min(mmH - 1, viewport.h * scaleY));

  return (
    <div
      style={{
        position: 'absolute',
        bottom: PAD,
        right: PAD,
        zIndex: 50,
        background: 'rgba(13, 13, 26, 0.92)',
        border: '1px solid #333366',
        borderRadius: 6,
        padding: 6,
        pointerEvents: 'auto',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
      }}
      title="Workspace overview — click or drag to jump"
    >
      <svg
        ref={svgRef}
        width={mmW}
        height={mmH}
        onMouseDown={handleMouseDown}
        style={{ display: 'block', cursor: 'pointer', background: '#0a0a18', borderRadius: 3 }}
      >
        {sessions.map(s => {
          const x = (s.col * tileWidth + (s.col + 1) * gap) * scaleX;
          const y = (s.row * tileHeight + (s.row + 1) * gap) * scaleY;
          const w = Math.max(2, tileWidth * scaleX);
          const h = Math.max(2, tileHeight * scaleY);
          const fill = s.state === 'connected' ? '#4aaa6a'
            : s.state === 'connecting' ? '#caaa4a'
            : s.state === 'error' ? '#ff5555'
            : '#555';
          return (
            <rect key={s.id} x={x} y={y} width={w} height={h} fill={fill} opacity={0.65} rx={1} />
          );
        })}
        <rect
          x={vpX}
          y={vpY}
          width={vpW}
          height={vpH}
          fill="rgba(124, 106, 247, 0.15)"
          stroke="#7c6af7"
          strokeWidth={1.5}
          pointerEvents="none"
        />
      </svg>
    </div>
  );
}
