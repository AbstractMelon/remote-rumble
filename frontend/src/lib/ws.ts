type EventHandler = (payload: any) => void;

class AppSocket {
  private socket: WebSocket | null = null;
  private handlers = new Map<string, Set<EventHandler>>();
  private currentToken = '';
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  connect(token: string) {
    const nextToken = token.trim();
    if (!nextToken) return;

    const tokenChanged = this.currentToken !== '' && this.currentToken !== nextToken;
    this.currentToken = nextToken;

    if (this.socket && this.socket.readyState <= WebSocket.OPEN) {
      if (!tokenChanged) return;
      this.socket.close(1000, 'token changed');
      this.socket = null;
    }

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${window.location.host}/ws?token=${encodeURIComponent(this.currentToken)}`;
    this.socket = new WebSocket(url);

    this.socket.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      const type = msg.type;
      const set = this.handlers.get(type);
      if (!set) return;
      for (const cb of set) cb(msg);
    };

    this.socket.onclose = () => {
      this.socket = null;
      if (!this.currentToken) return;
      this.reconnectTimer = setTimeout(() => {
        this.reconnectTimer = null;
        this.connect(this.currentToken);
      }, 1200);
    };
  }

  disconnect() {
    this.currentToken = '';
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.socket) {
      this.socket.close(1000, 'client disconnect');
      this.socket = null;
    }
  }

  on(type: string, handler: EventHandler) {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set());
    this.handlers.get(type)!.add(handler);
    return () => this.handlers.get(type)?.delete(handler);
  }

  send(payload: Record<string, any>) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
    this.socket.send(JSON.stringify(payload));
  }
}

export const appSocket = new AppSocket();
