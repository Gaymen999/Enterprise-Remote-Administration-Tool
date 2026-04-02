import { useEffect, useRef, useState, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';
import { SearchAddon } from 'xterm-addon-search';
import DOMPurify from 'dompurify';
import 'xterm/css/xterm.css';
import { getWebSocketUrl } from '../services/api';

const escapeXterm = (str: string): string => {
  return DOMPurify.sanitize(str, { 
    ALLOWED_TAGS: [],
    ALLOWED_ATTR: []
  });
};

const writeEscaped = (term: XTerm, data: string) => {
  term.write(escapeXterm(data));
};

interface PtyMessage {
  type: string;
  payload?: {
    session_id?: string;
    data?: string;
    error?: string;
    cols?: number;
    rows?: number;
  };
}

export function Terminal() {
  const { id: agentId } = useParams<{ id: string }>();
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [sessionActive, setSessionActive] = useState(false);

  const sendPtyMessage = useCallback((ptyType: string, data?: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: 'pty',
        payload: { pty_type: ptyType, ...data }
      }));
    }
  }, []);

  const startPtySession = useCallback(() => {
    if (!xtermRef.current || !agentId) return;

    const cols = xtermRef.current.cols;
    const rows = xtermRef.current.rows;

    xtermRef.current.write('\x1b[36m[PTY]\x1b[0m Starting PTY session...\r\n');

    sendPtyMessage('start', {
      session_id: sessionIdRef.current || undefined,
      cols,
      rows,
      shell: ''
    });
  }, [agentId, sendPtyMessage]);

  useEffect(() => {
    if (!termRef.current || !agentId) return;

    const term = new XTerm({
      theme: {
        background: '#1a1a2e',
        foreground: '#eaeaea',
        cursor: '#00ff00',
        cursorAccent: '#1a1a2e',
        selectionBackground: 'rgba(0, 255, 0, 0.3)',
        black: '#1a1a2e',
        red: '#ff5555',
        green: '#50fa7b',
        yellow: '#f1fa8c',
        blue: '#6272a4',
        magenta: '#ff79c6',
        cyan: '#8be9fd',
        white: '#eaeaea',
      },
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 14,
      fontFamily: '"Fira Code", "Cascadia Code", Consolas, Monaco, "Courier New", monospace',
      fontWeight: '400',
      fontWeightBold: '700',
      lineHeight: 1.2,
      scrollback: 5000,
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    const searchAddon = new SearchAddon();
    const webLinksAddon = new WebLinksAddon();

    term.loadAddon(fitAddon);
    term.loadAddon(searchAddon);
    term.loadAddon(webLinksAddon);

    term.open(termRef.current);

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    setTimeout(() => {
      fitAddon.fit();
      term.focus();
    }, 100);

    term.write('\x1b[36m╔════════════════════════════════════════════╗\x1b[0m\r\n');
    term.write('\x1b[36m║     Enterprise RAT - Secure Terminal       ║\x1b[0m\r\n');
    term.write('\x1b[36m╚════════════════════════════════════════════╝\x1b[0m\r\n');
    term.write(`\x1b[33m[INFO]\x1b[0m Agent ID: ${agentId}\r\n`);
    term.write(`\x1b[33m[INFO]\x1b[0m Connecting to server...\r\n`);

    const wsUrl = getWebSocketUrl();
    const protocol = wsUrl.startsWith('wss') ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${wsUrl.replace(/^https?:\/\//, '')}/api/v1/ws`);

    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      term.write('\x1b[32m[CONNECTED]\x1b[0m WebSocket established.\r\n');
      term.write('\x1b[33m[INFO]\x1b[0m Starting PTY session...\r\n');
      setConnected(true);
      startPtySession();
    };

    ws.onclose = (event) => {
      term.write(`\r\n\x1b[31m[DISCONNECTED]\x1b[0m Connection closed (${event.code})\r\n`);
      setConnected(false);
      setSessionActive(false);
    };

    ws.onerror = () => {
      term.write('\r\n\x1b[31m[ERROR]\x1b[0m WebSocket error - ensure backend is running.\r\n');
    };

    ws.onmessage = (event) => {
      try {
        let data: string;
        if (event.data instanceof ArrayBuffer) {
          data = new TextDecoder().decode(event.data);
        } else {
          data = event.data;
        }

        const msg: PtyMessage = JSON.parse(data);

        switch (msg.type) {
          case 'pty_started':
            sessionIdRef.current = msg.payload?.session_id || null;
            setSessionActive(true);
            writeEscaped(term, '\x1b[32m[PTY]\x1b[0m PTY session started.\r\n\r\n');
            break;

          case 'pty_output':
            if (msg.payload?.data) {
              writeEscaped(term, msg.payload.data);
            }
            break;

          case 'pty_error':
            writeEscaped(term, `\r\n\x1b[31m[PTY ERROR]\x1b[0m ${msg.payload?.error || 'Unknown error'}\r\n`);
            break;

          case 'pty_stopped':
            writeEscaped(term, '\r\n\x1b[33m[PTY]\x1b[0m PTY session ended.\r\n');
            setSessionActive(false);
            break;

          case 'command_result':
            if (msg.payload?.data) {
              writeEscaped(term, `\r\n${msg.payload.data}\r\n`);
            }
            break;

          default:
            if (msg.type === 'error') {
              writeEscaped(term, `\r\n\x1b[31m[ERROR]\x1b[0m ${msg.payload?.error || 'Unknown error'}\r\n`);
            }
        }
      } catch (err) {
        term.write(`\r\n${event.data}`);
      }
    };

    wsRef.current = ws;

    term.onData((data: string) => {
      if (sessionActive && ws.readyState === WebSocket.OPEN) {
        sendPtyMessage('input', {
          session_id: sessionIdRef.current,
          data: data
        });
      }
    });

    term.onResize(({ cols, rows }) => {
      if (sessionActive && ws.readyState === WebSocket.OPEN) {
        sendPtyMessage('resize', {
          session_id: sessionIdRef.current,
          cols,
          rows
        });
      }
    });

    const handleResize = () => {
      setTimeout(() => {
        fitAddon.fit();
      }, 100);
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'c' && !sessionActive) {
        e.preventDefault();
        term.write('^C');
        sendPtyMessage('input', {
          session_id: sessionIdRef.current,
          data: '\x03'
        });
      }

      if (e.ctrlKey && e.key === 'l') {
        e.preventDefault();
        term.clear();
        term.write('\x1b[2J\x1b[H');
      }

      if (e.key === 'Tab') {
        e.preventDefault();
        sendPtyMessage('input', {
          session_id: sessionIdRef.current,
          data: '\t'
        });
      }
    };

    window.addEventListener('resize', handleResize);
    window.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('resize', handleResize);
      window.removeEventListener('keydown', handleKeyDown);

      if (sessionIdRef.current) {
        sendPtyMessage('stop', { session_id: sessionIdRef.current });
      }

      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }

      if (xtermRef.current) {
        xtermRef.current.dispose();
        xtermRef.current = null;
      }
    };
  }, [agentId, sendPtyMessage, startPtySession, sessionActive]);

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 65px)' }}>
      <div className="px-4 py-2 bg-gray-800 border-b border-gray-700 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <span className="text-white font-medium">Terminal</span>
          <span className="text-gray-400 text-sm">Agent: {agentId}</span>
          {sessionIdRef.current && (
            <span className="text-gray-500 text-xs">Session: {sessionIdRef.current.slice(0, 8)}...</span>
          )}
        </div>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full ${sessionActive ? 'bg-green-500 animate-pulse' : 'bg-red-500'}`} />
            <span className="text-sm text-gray-400">
              {sessionActive ? 'PTY Active' : connected ? 'Connecting...' : 'Disconnected'}
            </span>
          </div>
          {xtermRef.current && (
            <div className="flex items-center gap-2 text-xs text-gray-500">
              <span>{xtermRef.current.cols}x{xtermRef.current.rows}</span>
            </div>
          )}
        </div>
      </div>
      <div 
        ref={termRef} 
        className="flex-1 bg-[#1a1a2e] overflow-hidden" 
        style={{ minHeight: '400px', padding: '8px' }} 
      />
    </div>
  );
}
