import { useState, useRef, useEffect, useCallback } from 'react';
import { api } from '../utils/api';

interface AiMessage {
  role: 'user' | 'assistant';
  content: string;
}

interface AiPanelProps {
  pendingContext: string | null;
  onContextConsumed: () => void;
}

export function AiPanel({ pendingContext, onContextConsumed }: AiPanelProps) {
  const [messages, setMessages] = useState<AiMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [contextReady, setContextReady] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (pendingContext) {
      setContextReady(pendingContext);
      onContextConsumed();
    }
  }, [pendingContext, onContextConsumed]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = useCallback(async () => {
    const question = input.trim();
    const context = contextReady || '';
    if (!question && !context) return;

    const userContent = question
      ? (context ? `[Terminal context provided]\n${question}` : question)
      : '[Explain terminal output]';

    setMessages(prev => [...prev, { role: 'user', content: userContent }]);
    setInput('');
    setContextReady(null);
    setLoading(true);

    try {
      const result = await api.explainTerminal(context, question || undefined);
      setMessages(prev => [...prev, { role: 'assistant', content: result.response }]);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      setMessages(prev => [...prev, { role: 'assistant', content: `Error: ${msg}` }]);
    } finally {
      setLoading(false);
    }
  }, [input, contextReady]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div style={styles.panel}>
      <div style={styles.header}>AI Assistant</div>

      {contextReady && (
        <div style={styles.contextBanner}>
          Terminal context ready ({contextReady.split('\n').length} lines) — press Enter to explain
        </div>
      )}

      <div style={styles.messages}>
        {messages.length === 0 && !contextReady && (
          <div style={styles.empty}>
            Ask a question about terminal output, or click "Explain" on a terminal tile.
          </div>
        )}
        {messages.map((msg, i) => (
          <div key={i} style={msg.role === 'user' ? styles.userMsg : styles.assistantMsg}>
            <div style={styles.msgRole}>{msg.role === 'user' ? 'You' : 'AI'}</div>
            <div style={styles.msgContent}>{msg.content}</div>
          </div>
        ))}
        {loading && (
          <div style={styles.assistantMsg}>
            <div style={styles.msgRole}>AI</div>
            <div style={{ ...styles.msgContent, color: '#666' }}>Thinking…</div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <div style={styles.inputArea}>
        <textarea
          style={styles.input}
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={contextReady ? 'Ask a question (or Enter to explain)…' : 'Ask a question…'}
          rows={2}
          disabled={loading}
        />
        <button
          style={{ ...styles.sendBtn, opacity: loading ? 0.5 : 1 }}
          onClick={handleSend}
          disabled={loading}
        >
          Send
        </button>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  panel: {
    display: 'flex',
    flexDirection: 'column',
    width: 320,
    minWidth: 320,
    height: '100%',
    background: '#0f0f20',
    borderLeft: '1px solid #333366',
    flexShrink: 0,
  },
  header: {
    padding: '10px 14px',
    fontSize: 13,
    fontWeight: 700,
    color: '#7c6af7',
    borderBottom: '1px solid #222244',
    flexShrink: 0,
    letterSpacing: 0.5,
  },
  contextBanner: {
    padding: '6px 14px',
    fontSize: 11,
    color: '#caaa4a',
    background: '#1a1a0a',
    borderBottom: '1px solid #333310',
    flexShrink: 0,
  },
  messages: {
    flex: 1,
    overflowY: 'auto',
    padding: '12px 12px 4px',
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
  },
  empty: {
    color: '#444',
    fontSize: 12,
    textAlign: 'center',
    marginTop: 32,
    lineHeight: 1.6,
  },
  userMsg: {
    background: '#1a1a3a',
    borderRadius: 6,
    padding: '8px 10px',
    border: '1px solid #2a2a5a',
  },
  assistantMsg: {
    background: '#0d1a0d',
    borderRadius: 6,
    padding: '8px 10px',
    border: '1px solid #1a2a1a',
  },
  msgRole: {
    fontSize: 10,
    fontWeight: 700,
    color: '#666',
    marginBottom: 4,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  msgContent: {
    fontSize: 12,
    color: '#ccc',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    lineHeight: 1.5,
  },
  inputArea: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
    padding: '10px 12px',
    borderTop: '1px solid #222244',
    flexShrink: 0,
  },
  input: {
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: 4,
    color: '#e0e0e0',
    fontSize: 12,
    fontFamily: 'inherit',
    padding: '6px 8px',
    resize: 'none',
    outline: 'none',
    width: '100%',
    boxSizing: 'border-box',
  },
  sendBtn: {
    background: '#7c6af7',
    border: 'none',
    borderRadius: 4,
    color: '#fff',
    fontSize: 12,
    fontWeight: 600,
    padding: '6px 12px',
    cursor: 'pointer',
    alignSelf: 'flex-end',
  },
};
