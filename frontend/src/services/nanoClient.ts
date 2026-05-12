import {
  encodePackage,
  decodePackage,
  encodeMessage,
  decodeMessage,
  PackageType,
  MessageType,
} from './nanoProtocol';

interface NanoClientOptions {
  host: string;
  port?: string;
  path?: string;
  reconnect?: boolean;
  maxReconnectAttempts?: number;
  reconnectionDelay?: number;
}

interface PendingRequest {
  resolve: (value: unknown) => void;
  reject: (reason?: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
}

export class NanoClient {
  private socket: WebSocket | null = null;
  private reqId = 0;
  private callbacks = new Map<number, PendingRequest>();
  private routeMap = new Map<number, string>();
  private listeners = new Map<string, Set<(data: unknown) => void>>();
  private heartbeatInterval = 0;
  private heartbeatTimeout = 0;
  private heartbeatId: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimeoutId: ReturnType<typeof setTimeout> | null = null;
  private nextHeartbeatTimeout = 0;
  private gapThreshold = 100;

  private reconnect = false;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;
  private reconnectionDelay = 5000;
  private maxReconnectAttempts = 10;
  private reconnectUrl = '';
  private options: NanoClientOptions;
  private connectPromise: Promise<void> | null = null;
  private connectResolve: (() => void) | null = null;
  private connectReject: ((err: Error) => void) | null = null;

  public connected = false;

  constructor(options: NanoClientOptions) {
    this.options = options;
  }

  connect(): Promise<void> {
    if (this.connected) return Promise.resolve();
    if (this.connectPromise) return this.connectPromise;

    this.connectPromise = new Promise((resolve, reject) => {
      const { host, port, path } = this.options;
      let url = `ws://${host}`;
      if (port) url += `:${port}`;
      if (path) url += path;
      this.reconnectUrl = url;
      this.connectResolve = resolve;
      this.connectReject = reject;
      this.doConnect(url);
    });

    return this.connectPromise;
  }

  private doConnect(url: string) {
    this.socket = new WebSocket(url);
    this.socket.binaryType = 'arraybuffer';

    this.socket.onopen = () => {
      if (this.reconnect) {
        this.emit('reconnect', undefined);
      }
      this.resetReconnect();
      const handshake = {
        sys: { type: 'js-websocket', version: '0.0.1' },
        user: {},
      };
      const body = encodePackage(
        PackageType.HANDSHAKE,
        strEncode(JSON.stringify(handshake))
      );
      this.send(body);
    };

    this.socket.onmessage = (event) => {
      const packages = decodePackage(event.data as ArrayBuffer);
      for (const pkg of packages) {
        this.processPackage(pkg);
      }
      if (this.heartbeatTimeout) {
        this.nextHeartbeatTimeout = Date.now() + this.heartbeatTimeout;
      }
    };

    this.socket.onerror = (event) => {
      this.emit('error', event);
      if (this.connectReject) {
        this.connectReject(new Error('WebSocket error'));
        this.connectReject = null;
      }
    };

    this.socket.onclose = () => {
      this.connected = false;
      this.connectPromise = null;
      this.emit('close', undefined);
      if (this.options.reconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnect = true;
        this.reconnectAttempts++;
        this.reconnectTimer = setTimeout(() => {
          this.doConnect(this.reconnectUrl);
        }, this.reconnectionDelay);
        this.reconnectionDelay *= 2;
      }
    };
  }

  private processPackage(pkg: { type: PackageType; body: Uint8Array }) {
    switch (pkg.type) {
      case PackageType.HANDSHAKE:
        this.onHandshake(pkg.body);
        break;
      case PackageType.HEARTBEAT:
        this.onHeartbeat();
        break;
      case PackageType.DATA:
        this.onData(pkg.body);
        break;
      case PackageType.KICK:
        this.onKick(pkg.body);
        break;
    }
  }

  private onHandshake(body: Uint8Array) {
    const data = JSON.parse(strDecode(body));
    if (data.code === 501) {
      this.emit('error', 'client version not fulfill');
      return;
    }
    if (data.code !== 200) {
      this.emit('error', 'handshake fail');
      return;
    }

    if (data.sys && data.sys.heartbeat) {
      this.heartbeatInterval = data.sys.heartbeat * 1000;
      this.heartbeatTimeout = this.heartbeatInterval * 2;
    } else {
      this.heartbeatInterval = 0;
      this.heartbeatTimeout = 0;
    }

    const ack = encodePackage(PackageType.HANDSHAKE_ACK);
    this.send(ack);
    this.connected = true;

    if (this.connectResolve) {
      this.connectResolve();
      this.connectResolve = null;
    }
    this.emit('connect', undefined);
  }

  private onHeartbeat() {
    if (!this.heartbeatInterval) return;
    if (this.heartbeatTimeoutId) {
      clearTimeout(this.heartbeatTimeoutId);
      this.heartbeatTimeoutId = null;
    }

    // Respond immediately — the Nano server expects a prompt ack of every
    // heartbeat. A delayed reply (e.g. via setTimeout) causes the server-side
    // timeout to fire, closing the connection with code 1001.
    const packet = encodePackage(PackageType.HEARTBEAT);
    this.send(packet);
    this.nextHeartbeatTimeout = Date.now() + this.heartbeatTimeout;
    this.heartbeatTimeoutId = setTimeout(() => this.heartbeatTimeoutCb(), this.heartbeatTimeout);
  }

  private heartbeatTimeoutCb() {
    const gap = this.nextHeartbeatTimeout - Date.now();
    if (gap > this.gapThreshold) {
      this.heartbeatTimeoutId = setTimeout(() => this.heartbeatTimeoutCb(), gap);
    } else {
      this.emit('heartbeat timeout', undefined);
      this.disconnect();
    }
  }

  private onData(body: Uint8Array) {
    const msg = decodeMessage(body);
    if (msg.type === MessageType.RESPONSE) {
      const cb = this.callbacks.get(msg.id);
      if (cb) {
        clearTimeout(cb.timer);
        this.callbacks.delete(msg.id);
        this.routeMap.delete(msg.id);
        try {
          const data = JSON.parse(strDecode(msg.body as Uint8Array));
          cb.resolve(data);
        } catch {
          cb.resolve(msg.body);
        }
      }
    } else if (msg.type === MessageType.PUSH) {
      const route = msg.route as string;
      try {
        const data = JSON.parse(strDecode(msg.body as Uint8Array));
        this.emit(route, data);
      } catch {
        this.emit(route, msg.body);
      }
    }
  }

  private onKick(body: Uint8Array) {
    try {
      const data = JSON.parse(strDecode(body));
      this.emit('onKick', data);
    } catch {
      this.emit('onKick', body);
    }
  }

  request(route: string, msg: Record<string, unknown>): Promise<unknown> {
    return new Promise((resolve, reject) => {
      this.reqId++;
      const id = this.reqId;
      const body = strEncode(JSON.stringify(msg));
      const packet = encodePackage(
        PackageType.DATA,
        encodeMessage(id, MessageType.REQUEST, 0, route, body)
      );
      this.send(packet);
      this.routeMap.set(id, route);

      const timer = setTimeout(() => {
        this.callbacks.delete(id);
        this.routeMap.delete(id);
        reject(new Error('request timeout'));
      }, 10000);

      this.callbacks.set(id, { resolve, reject, timer });
    });
  }

  notify(route: string, msg: Record<string, unknown>) {
    const body = strEncode(JSON.stringify(msg));
    const packet = encodePackage(
      PackageType.DATA,
      encodeMessage(0, MessageType.NOTIFY, 0, route, body)
    );
    this.send(packet);
  }

  on(event: string, handler: (data: unknown) => void) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(handler);
    return () => this.off(event, handler);
  }

  off(event: string, handler: (data: unknown) => void) {
    this.listeners.get(event)?.delete(handler);
  }

  private emit(event: string, data: unknown) {
    this.listeners.get(event)?.forEach((fn) => {
      try {
        fn(data);
      } catch (e) {
        console.error('NanoClient emit error:', e);
      }
    });
  }

  private send(data: Uint8Array) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(data.buffer);
    }
  }

  disconnect() {
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    if (this.heartbeatId) {
      clearTimeout(this.heartbeatId);
      this.heartbeatId = null;
    }
    if (this.heartbeatTimeoutId) {
      clearTimeout(this.heartbeatTimeoutId);
      this.heartbeatTimeoutId = null;
    }
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.connected = false;
    this.connectPromise = null;
  }

  private resetReconnect() {
    this.reconnect = false;
    this.reconnectionDelay = 5000;
    this.reconnectAttempts = 0;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }
}

function strEncode(str: string): Uint8Array {
  return new TextEncoder().encode(str);
}

function strDecode(buf: Uint8Array): string {
  return new TextDecoder().decode(buf);
}

// Singleton instance
let client: NanoClient | null = null;

export function getNanoClient(): NanoClient {
  if (!client) {
    client = new NanoClient({
      host: window.location.hostname,
      port: '3250',
      path: '/ws',
      reconnect: true,
    });
  }
  return client;
}

export function resetNanoClient() {
  if (client) {
    client.disconnect();
    client = null;
  }
}
