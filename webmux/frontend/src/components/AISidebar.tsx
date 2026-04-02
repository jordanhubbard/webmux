/**
 * AISidebar — collapsible AI assistant panel for webmux terminal sessions.
 *
 * Shows a chat interface that lets the user ask questions about their terminal
 * output. The last N lines of terminal buffer are automatically sent as
 * context with each message.
 *
 * Usage:
 *   <AISidebar
 *     getTerminalContext={() => terminalRef.current?.getScrollback(50) || ''}
 *     isOpen={aiOpen}
 *     onClose={() => setAiOpen(false)}
 *   />
 *
 * The panel is resizable via a drag handle on the left edge.
 */

import React, { useState, useRef, useEffect, useCallback } from 'react';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  ts: number;
  model?: string;
  error?: boolean;
}

interface AISidebarProps {
  /** Returns the last N lines of terminal output for context. */
  getTerminalContext: () => string;
  /** Whether the sidebar is visible. */
  isOpen: boolean;
  /** Called when the user closes the sidebar. */
  onClose: () => void;
  /** Sidebar width in px (default 380). */
  initialWidth?: number;
}

const API_BASE = import.meta.env.VITE_API_BASE ?? '';

async function sendMessage(
  message: string,
  context: string,
  history: Message[],
): Promise<{ reply: string; model: string }> {
  const resp = await fetch(`${API_BASE}/api/ai/chat`, {
    method:  'POST',
    headers: { 'Content-Type': 'application/json' },
    body:    JSON.stringify({
      message,
      context,
      history: history.slice(-10).map(m => ({ role: m.role, content: m.content })),
    }),
    credentials: 'include',
  });

  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: `HTTP ${resp.status}` }));
    throw new Error(err.error || `HTTP ${resp.status}`);
  }

  return resp.json();
}

export function AISidebar({ getTerminalContext, isOpen, onClose, initialWidth = 380 }: AISidebarProps) {
  const [messages, setMessages]   = useState<Message[]>([]);
  const [input, setInput]         = useState('');
  const [loading, setLoading]     = useState(false);
  const [width, setWidth]         = useState(initialWidth);
  const [useContext, setUseCtx]   = useState(true);
  const messagesEndRef             = useRef<HTMLDivElement>(null);
  const inputRef                   = useRef<HTMLTextAreaElement>(null);
  const dragRef                    = useRef<{ startX: number; startW: number } | null>(null);

  // Auto-scroll to bottom on new message
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Focus input when sidebar opens
  useEffect(() => {
    if (isOpen) setTimeout(() => inputRef.current?.focus(), 100);
  }, [isOpen]);

  const handleSend = useCallback(async () => {
    const msg = input.trim();
    if (!msg || loading) return;
    setInput('');
    const ctx = useContext ? getTerminalContext() : '';
    const userMsg: Message = { role: 'user', content: msg, ts: Date.now() };
    setMessages(prev => [...prev, userMsg]);
    setLoading(true);

    try {
      const { reply, model } = await sendMessage(msg, ctx, messages);
      setMessages(prev => [...prev, { role: 'assistant', content: reply, model, ts: Date.now() }]);
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : String(err);
      setMessages(prev => [...prev, {
        role: 'assistant', content: `⚠ ${errMsg}`, ts: Date.now(), error: true,
      }]);
    } finally {
      setLoading(false);
    }
  }, [input, loading, messages, getTerminalContext, useContext]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  // Drag-to-resize handle (left edge of sidebar)
  const onDragStart = (e: React.MouseEvent) => {
    dragRef.current = { startX: e.clientX, startW: width };
    const onMove = (me: MouseEvent) => {
      if (!dragRef.current) return;
      const dx  = dragRef.current.startX - me.clientX;
      const nw  = Math.min(700, Math.max(260, dragRef.current.startW + dx));
      setWidth(nw);
    };
    const onUp = () => {
      dragRef.current = null;
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  };

  if (!isOpen) return null;

  return (
    <div style={{
      position:   'relative',
      width:      `${width}px`,
      minWidth:   '260px',
      maxWidth:   '700px',
      display:    'flex',
      flexDirection: 'column',
      background: '#1a1d21',
      borderLeft: '1px solid #2f3338',
      overflow:   'hidden',
      flexShrink: 0,
    }}>
      {/* Drag handle */}
      <div onMouseDown={onDragStart} style={{
        position: 'absolute', left: 0, top: 0, bottom: 0, width: '4px',
        cursor: 'ew-resize', zIndex: 10,
        background: 'transparent',
      }} />

      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '8px 12px', borderBottom: '1px solid #2f3338',
        background: '#22262c',
      }}>
        <span style={{ fontWeight: 600, fontSize: '14px', color: '#d1d2d3' }}>
          🤖 AI Assistant
        </span>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <label style={{ fontSize: '11px', color: '#8f939b', display: 'flex', gap: '4px', alignItems: 'center', cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={useContext}
              onChange={e => setUseCtx(e.target.checked)}
              style={{ width: 12, height: 12 }}
            />
            Terminal context
          </label>
          <button onClick={() => setMessages([])} title="Clear conversation"
            style={{ background: 'none', border: 'none', color: '#8f939b', cursor: 'pointer', fontSize: '16px', padding: '0 4px', lineHeight: 1 }}>
            🗑
          </button>
          <button onClick={onClose} title="Close AI panel"
            style={{ background: 'none', border: 'none', color: '#8f939b', cursor: 'pointer', fontSize: '18px', padding: '0 4px', lineHeight: 1 }}>
            ✕
          </button>
        </div>
      </div>

      {/* Messages */}
      <div style={{
        flex: 1, overflowY: 'auto', padding: '12px',
        display: 'flex', flexDirection: 'column', gap: '10px',
        fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
      }}>
        {messages.length === 0 && (
          <div style={{ color: '#8f939b', fontSize: '13px', textAlign: 'center', marginTop: '40px' }}>
            <div style={{ fontSize: '28px', marginBottom: '8px' }}>💬</div>
            <div>Ask about your terminal output,</div>
            <div>diagnose errors, or get command suggestions.</div>
            {useContext && (
              <div style={{ marginTop: '12px', fontSize: '11px', color: '#636b72' }}>
                Terminal context is included automatically.
              </div>
            )}
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} style={{
            display: 'flex',
            flexDirection: m.role === 'user' ? 'row-reverse' : 'row',
            gap: '8px',
          }}>
            <div style={{ fontSize: '18px', lineHeight: 1, marginTop: '2px', flexShrink: 0 }}>
              {m.role === 'user' ? '👤' : '🤖'}
            </div>
            <div style={{
              background:   m.role === 'user' ? '#1a3d5c' : (m.error ? '#3d1a1a' : '#22262c'),
              color:        m.error ? '#f85149' : '#d1d2d3',
              borderRadius: '8px',
              padding:      '8px 12px',
              fontSize:     '13px',
              lineHeight:   '1.5',
              maxWidth:     '85%',
              whiteSpace:   'pre-wrap',
              wordBreak:    'break-word',
              border:       `1px solid ${m.role === 'user' ? '#2a5a8f' : '#2f3338'}`,
            }}>
              {m.content}
              {m.model && m.role === 'assistant' && (
                <div style={{ marginTop: '4px', fontSize: '10px', color: '#636b72' }}>
                  via {m.model}
                </div>
              )}
            </div>
          </div>
        ))}
        {loading && (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <span style={{ fontSize: '18px' }}>🤖</span>
            <div style={{ background: '#22262c', border: '1px solid #2f3338', borderRadius: '8px', padding: '8px 14px', color: '#8f939b', fontSize: '13px' }}>
              <span style={{ animation: 'pulse 1s ease-in-out infinite' }}>Thinking…</span>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div style={{ padding: '10px 12px', borderTop: '1px solid #2f3338', background: '#22262c' }}>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'flex-end' }}>
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask about your terminal… (Enter to send, Shift+Enter for newline)"
            disabled={loading}
            rows={2}
            style={{
              flex: 1, resize: 'none', background: '#1a1d21', color: '#d1d2d3',
              border: '1px solid #3a4048', borderRadius: '6px', padding: '8px 10px',
              fontSize: '13px', fontFamily: 'inherit', outline: 'none',
              lineHeight: '1.4', maxHeight: '120px',
            }}
          />
          <button
            onClick={handleSend}
            disabled={!input.trim() || loading}
            style={{
              background: input.trim() && !loading ? '#4a9eff' : '#2a5a8f',
              border: 'none', borderRadius: '6px', color: '#fff',
              padding: '8px 12px', cursor: input.trim() && !loading ? 'pointer' : 'not-allowed',
              fontSize: '16px', lineHeight: 1, alignSelf: 'flex-end',
              opacity: input.trim() && !loading ? 1 : 0.5,
            }}
          >
            ➤
          </button>
        </div>
        <div style={{ fontSize: '11px', color: '#636b72', marginTop: '4px' }}>
          Enter ↵ send · Shift+Enter newline
        </div>
      </div>
    </div>
  );
}

export default AISidebar;
