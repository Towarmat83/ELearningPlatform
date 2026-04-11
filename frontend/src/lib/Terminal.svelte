<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { Terminal } from '@xterm/xterm';
  import type { FitAddon } from '@xterm/addon-fit';

  export let courseId: string;
  export let labId: string;
  export let token: string;

  /** Send a command to the running terminal (adds a newline automatically). */
  export function sendCommand(cmd: string) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const data = new TextEncoder().encode(cmd + '\n');
    ws.send(data);
  }

  let container: HTMLElement;
  let term: Terminal | null = null;
  let fitAddon: FitAddon | null = null;
  let ws: WebSocket | null = null;
  let connected = false;
  let error = '';
  let resizeCleanup: (() => void) | null = null;

  // Derive API host for direct WebSocket connection (bypasses SvelteKit proxy)
  function getWsUrl(): string {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.hostname;
    const port = '8080';
    return `${proto}//${host}:${port}/ws/courses/${courseId}/labs/${labId}/terminal?token=${encodeURIComponent(token)}`;
  }

  onMount(async () => {
    // Dynamic imports to avoid SSR issues
    const [{ Terminal: XTerm }, { FitAddon: XFitAddon }, { AttachAddon }] = await Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/addon-attach'),
    ]);

    // Import xterm CSS
    await import('@xterm/xterm/css/xterm.css');

    term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", monospace',
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        selectionBackground: '#264f78',
        black: '#0d1117',
        red: '#ff7b72',
        green: '#3fb950',
        yellow: '#d29922',
        blue: '#58a6ff',
        magenta: '#bc8cff',
        cyan: '#39c5cf',
        white: '#b1bac4',
        brightBlack: '#6e7681',
        brightRed: '#ffa198',
        brightGreen: '#56d364',
        brightYellow: '#e3b341',
        brightBlue: '#79c0ff',
        brightMagenta: '#d2a8ff',
        brightCyan: '#56d4dd',
        brightWhite: '#f0f6fc',
      },
      scrollback: 1000,
    });

    fitAddon = new XFitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    fitAddon.fit();

    // Connect WebSocket
    const url = getWsUrl();
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      connected = true;
      error = '';
      // Send initial resize
      sendResize();
      // Attach addon bridges WS<->terminal automatically
      const attachAddon = new AttachAddon(ws!);
      term!.loadAddon(attachAddon);
    };

    ws.onerror = () => {
      error = 'Connection failed';
      connected = false;
    };

    ws.onclose = () => {
      connected = false;
      if (!error) {
        term?.writeln('\r\n\x1b[33m[Connection closed]\x1b[0m');
      }
    };

    // Resize observer — debounced to break the fit→resize→fit feedback loop
    let resizeTimer: ReturnType<typeof setTimeout>;
    const resizeObserver = new ResizeObserver(() => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        fitAddon?.fit();
        sendResize();
      }, 50);
    });
    resizeObserver.observe(container);
    resizeCleanup = () => { resizeObserver.disconnect(); clearTimeout(resizeTimer); };
  });

  function sendResize() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !term) return;
    ws.send(JSON.stringify({ type: 'resize', rows: term.rows, cols: term.cols }));
  }

  onDestroy(() => {
    resizeCleanup?.();
    ws?.close();
    term?.dispose();
  });
</script>

<div class="terminal-wrapper">
  {#if error}
    <div class="terminal-error">{error}</div>
  {/if}
  <div class="terminal-status">
    <span class="status-dot" class:connected></span>
    {connected ? 'Connected' : 'Connecting...'}
  </div>
  <div bind:this={container} class="terminal-container"></div>
</div>

<style>
  .terminal-wrapper {
    display: flex;
    flex-direction: column;
    background: #0d1117;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid #30363d;
  }

  .terminal-status {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    background: #161b22;
    border-bottom: 1px solid #30363d;
    font-size: 12px;
    color: #8b949e;
    font-family: monospace;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #6e7681;
    transition: background 0.3s;
  }

  .status-dot.connected {
    background: #3fb950;
  }

  .terminal-container {
    padding: 8px;
    height: 480px;
    overflow: hidden;
  }

  .terminal-error {
    padding: 8px 12px;
    background: #3d1515;
    color: #ff7b72;
    font-size: 12px;
    font-family: monospace;
  }
</style>
