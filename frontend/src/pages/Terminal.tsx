import { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';
import { getWebSocketUrl } from '../services/api';

interface CommandResponse {
  command_id: string;
  stdout: string;
  stderr: string;
  exit_code: number;
  error_msg?: string;
}

interface WsMessage {
  type: string;
  payload?: CommandResponse | Record<string, unknown>;
}

export function Terminal() {
  const { id: agentId } = useParams<{ id: string }>();
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const inputBufferRef = useRef<string>('');
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!termRef.current || !agentId) return;

    const term = new XTerm({
      theme: {
        background: '#1f2937',
        foreground: '#e5e7eb',
        cursor: '#3b82f6',
        cursorAccent: '#1f2937',
        selection: 'rgba(59, 130, 246, 0.3)',
        black: '#000000',
        red: '#ef4444',
        green: '#22c55e',
        yellow: '#eab308',
        blue: '#3b82f6',
        magenta: '#a855f7',
        cyan: '#06b6d4',
        white: '#e5e7eb',
      },
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Consolas, Monaco, "Courier New", monospace',
      scrollback: 10000,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    term.open(termRef.current);
    setTimeout(() => {
      fitAddon.fit();
    }, 100);

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    term.write(`\x1b[36m[Enterprise RAT]\x1b[0m Connecting to agent ${agentId}...\r\n`);

    const wsUrl = getWebSocketUrl();
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      term.write(`\x1b[32m[Connected]\x1b[0m WebSocket established.\r\n`);
      term.write(`\x1b[33m[Ready]\x1b[0m Type commands below.\r\n\r\n$ `);
      setConnected(true);
    };

    ws.onclose = () => {
      term.write(`\r\n\x1b[31m[Disconnected]\x1b[0m Connection closed.\r\n`);
      setConnected(false);
    };

    ws.onerror = () => {
      term.write(`\r\n\x1b[31m[Error]\x1b[0m WebSocket error - ensure backend is running.\r\n`);
    };

    ws.onmessage = (event) => {
      try {
        const msg: WsMessage = JSON.parse(event.data);
        if (msg.type === 'command_result' && msg.payload) {
          const resp = msg.payload as CommandResponse;
          if (resp.stdout) {
            term.write(`\r\n${resp.stdout}`);
          }
          if (resp.stderr) {
            term.write(`\r\n\x1b[31m${resp.stderr}\x1b[0m`);
          }
          term.write(`\r\n\x1b[90m[Exit: ${resp.exit_code}]\x1b[0m\r\n$ `);
        }
      } catch {
        term.write(`\r\n${event.data}`);
      }
    };

    wsRef.current = ws;

    term.onData((data: string) => {
      const code = data.charCodeAt(0);

      if (code === 13) {
        term.write('\r\n');
        const cmd = inputBufferRef.current.trim();

        if (cmd) {
          const sanitizedCmd = cmd.replace(/[;&|`$()]/g, '').trim();
          if (!sanitizedCmd) {
            term.write('\x1b[33m[Warning] Command contains disallowed characters\x1b[0m\r\n$ ');
            inputBufferRef.current = '';
            return;
          }

          const parts = sanitizedCmd.split(/\s+/);
          const executable = parts[0];
          const args = parts.slice(1);

          const commandReq = {
            type: 'command',
            payload: {
              command_id: crypto.randomUUID(),
              executable,
              args,
              timeout_seconds: 300,
            },
          };

          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(commandReq));
          } else {
            term.write('\x1b[31m[Error] Not connected\x1b[0m\r\n$ ');
          }
        } else {
          term.write('$ ');
        }
        inputBufferRef.current = '';
      } else if (code === 127) {
        if (inputBufferRef.current.length > 0) {
          inputBufferRef.current = inputBufferRef.current.slice(0, -1);
          term.write('\b \b');
        }
      } else if (code >= 32) {
        inputBufferRef.current += data;
        term.write(data);
      }
    });

    const handleResize = () => {
      setTimeout(() => {
        if (fitAddonRef.current) {
          fitAddonRef.current.fit();
        }
      }, 100);
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
      }
    };
  }, [agentId]);

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 65px)' }}>
      <div className="px-4 py-2 bg-gray-800 border-b border-gray-700 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-white font-medium">Terminal</span>
          <span className="text-gray-400 text-sm">Agent: {agentId}</span>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`}
          />
          <span className="text-sm text-gray-400">
            {connected ? 'Connected' : 'Disconnected'}
          </span>
        </div>
      </div>
      <div ref={termRef} className="flex-1 bg-gray-900 overflow-hidden" style={{ minHeight: '400px' }} />
    </div>
  );
}
