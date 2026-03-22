import { useState, useCallback, useRef, useEffect } from 'react';
import { api } from '../utils/api';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';

interface AiSidebarProps {
  onClose: () => void;
}

interface Message {
  role: 'user' | 'assistant';
  text: string;
}

// Split response text into text segments and code blocks
function parseSegments(text: string): Array<{ type: 'text' | 'code'; content: string; lang?: string }> {
  const segments: Array<{ type: 'text' | 'code'; content: string; lang?: string }> = [];
  const codeBlockRe = /```(\w*)\n?([\s\S]*?)```/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = codeBlockRe.exec(text)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ type: 'text', content: text.slice(lastIndex, match.index) });
    }
    segments.push({ type: 'code', content: match[2].trimEnd(), lang: match[1] || undefined });
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    segments.push({ type: 'text', content: text.slice(lastIndex) });
  }

  return segments;
}

export function AiSidebar({ onClose }: AiSidebarProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [prompt, setPrompt] = useState('');
  const [loading, setLoading] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const { focusedSessionId, getScrollbackForSession, sendToSession } = useInputBroadcast();

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  const handleSubmit = useCallback(async () => {
    const text = prompt.trim();
    if (!text || loading) return;

    setPrompt('');
    setMessages(prev => [...prev, { role: 'user', text }]);
    setLoading(true);

    try {
      const context = focusedSessionId ? getScrollbackForSession(focusedSessionId, 50) : '';
      const result = await api.askAi(text, context);
      const responseText = result.response ?? JSON.stringify(result, null, 2);
      setMessages(prev => [...prev, { role: 'assistant', text: responseText }]);
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', text: `Error: ${(err as Error).message}` }]);
    } finally {
      setLoading(false);
    }
  }, [prompt, loading, focusedSessionId, getScrollbackForSession]);

  const handleApply = useCallback((code: string) => {
    if (!focusedSessionId) return;
    sendToSession(focusedSessionId, code + '\n');
  }, [focusedSessionId, sendToSession]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }, [handleSubmit]);

  return (
    <div style={styles.sidebar}>
      <div style={styles.header}>
        <span style={styles.headerTitle}>🤖 AI Assistant</span>
        <button style={styles.closeBtn} onClick={onClose} title="Close AI sidebar">✕</button>
      </div>

      <div ref={scrollRef} style={styles.messages}>
        {messages.length === 0 && (
          <div style={styles.empty}>
            Ask a question or type a command. The last 50 lines of the focused terminal will be included as context.
          </div>
        )}
        {messages.map((msg, i) => (
          <MessageBubble key={i} message={msg} onApply={handleApply} />
        ))}
        {loading && (
          <div style={styles.thinking}>Thinking…</div>
        )}
      </div>

      <div style={styles.inputArea}>
        <textarea
          ref={textareaRef}
          style={styles.textarea}
          placeholder="Ask AI… (Enter to send, Shift+Enter for newline)"
          value={prompt}
          onChange={e => setPrompt(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={3}
          disabled={loading}
        />
        <button
          style={{ ...styles.sendBtn, opacity: (!prompt.trim() || loading) ? 0.5 : 1 }}
          onClick={handleSubmit}
          disabled={!prompt.trim() || loading}
        >
          Send
        </button>
      </div>
    </div>
  );
}

interface MessageBubbleProps {
  message: Message;
  onApply: (code: string) => void;
}

function MessageBubble({ message, onApply }: MessageBubbleProps) {
  const isUser = message.role === 'user';
  if (isUser) {
    return (
      <div style={styles.userBubble}>
        <span style={styles.userText}>{message.text}</span>
      </div>
    );
  }

  const segments = parseSegments(message.text);
  return (
    <div style={styles.assistantBubble}>
      {segments.map((seg, i) => {
        if (seg.type === 'code') {
          return (
            <div key={i} style={styles.codeBlock}>
              {seg.lang && <span style={styles.codeLang}>{seg.lang}</span>}
              <pre style={styles.codePre}>{seg.content}</pre>
              <button
                style={styles.applyBtn}
                onClick={() => onApply(seg.content)}
                title="Type this into the focused terminal"
              >
                Apply
              </button>
            </div>
          );
        }
        return <span key={i} style={styles.assistantText}>{seg.content}</span>;
      })}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  sidebar: {
    width: 320,
    flexShrink: 0,
    display: 'flex',
    flexDirection: 'column',
    background: '#12122a',
    borderLeft: '1px solid #333366',
    height: '100%',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '8px 12px',
    borderBottom: '1px solid #333366',
    flexShrink: 0,
  },
  headerTitle: {
    fontSize: 13,
    fontWeight: 700,
    color: '#e0e0e0',
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#666',
    fontSize: 12,
    cursor: 'pointer',
    padding: '2px 4px',
    borderRadius: 2,
  },
  messages: {
    flex: 1,
    overflowY: 'auto',
    padding: 12,
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
  },
  empty: {
    color: '#555',
    fontSize: 12,
    fontStyle: 'italic',
    lineHeight: 1.5,
    textAlign: 'center',
    padding: '20px 0',
  },
  thinking: {
    color: '#888',
    fontSize: 12,
    fontStyle: 'italic',
    padding: '4px 0',
  },
  userBubble: {
    alignSelf: 'flex-end',
    background: '#2a2a5a',
    borderRadius: 8,
    padding: '7px 10px',
    maxWidth: '85%',
  },
  userText: {
    fontSize: 12,
    color: '#e0e0e0',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
  assistantBubble: {
    alignSelf: 'flex-start',
    maxWidth: '100%',
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  assistantText: {
    fontSize: 12,
    color: '#c0c0d0',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    lineHeight: 1.5,
  },
  codeBlock: {
    background: '#0d0d1a',
    border: '1px solid #333366',
    borderRadius: 4,
    overflow: 'hidden',
    position: 'relative',
  },
  codeLang: {
    display: 'block',
    fontSize: 10,
    color: '#7c6af7',
    padding: '3px 8px',
    background: '#1a1a3a',
    borderBottom: '1px solid #333366',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  codePre: {
    margin: 0,
    padding: '8px 10px',
    fontSize: 11,
    fontFamily: 'Consolas, Menlo, monospace',
    color: '#e0e0e0',
    overflowX: 'auto',
    whiteSpace: 'pre',
  },
  applyBtn: {
    display: 'block',
    width: '100%',
    background: '#1a3a2a',
    border: 'none',
    borderTop: '1px solid #2a6a4a',
    padding: '5px 10px',
    color: '#4aaa6a',
    fontSize: 11,
    fontWeight: 600,
    cursor: 'pointer',
    textAlign: 'left',
  },
  inputArea: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
    padding: 10,
    borderTop: '1px solid #333366',
    flexShrink: 0,
  },
  textarea: {
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '7px 10px',
    color: '#e0e0e0',
    fontSize: 12,
    resize: 'none',
    fontFamily: 'inherit',
    outline: 'none',
    lineHeight: 1.4,
  },
  sendBtn: {
    background: '#7c6af7',
    border: 'none',
    borderRadius: 4,
    padding: '7px 16px',
    color: '#fff',
    fontSize: 12,
    fontWeight: 600,
    cursor: 'pointer',
    alignSelf: 'flex-end',
  },
};
