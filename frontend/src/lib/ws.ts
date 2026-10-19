import { MOCK, WS_URL } from "./config";
import type { ClientEvent, ServerEvent } from "./types";

/** Minimal socket surface shared by the browser WebSocket and the mock socket. */
export interface SocketLike {
  readyState: number;
  onopen: ((ev: unknown) => void) | null;
  onclose: ((ev: unknown) => void) | null;
  onerror: ((ev: unknown) => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export type WsStatus = "idle" | "connecting" | "open" | "reconnecting" | "closed";

type Options = {
  getToken: () => string | null;
  onEvent: (ev: ServerEvent) => void;
  onStatus?: (s: WsStatus) => void;
  /** Called after a successful *re*connect (not the first connect) so callers can refetch gaps. */
  onReconnect?: () => void;
};

const PING_MS = 25_000;
const MAX_BACKOFF_MS = 15_000;

async function openSocket(url: string): Promise<SocketLike> {
  if (MOCK) {
    const { MockSocket } = await import("./mock/server");
    return new MockSocket(url);
  }
  return new WebSocket(url) as unknown as SocketLike;
}

export class WsClient {
  private opts: Options;
  private socket: SocketLike | null = null;
  private sessionId: string | null = null;
  private attempt = 0;
  private everOpened = false;
  private stopped = false;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  status: WsStatus = "idle";

  constructor(opts: Options) {
    this.opts = opts;
  }

  private setStatus(s: WsStatus) {
    this.status = s;
    this.opts.onStatus?.(s);
  }

  connect() {
    this.stopped = false;
    void this.open();
  }

  private async open() {
    const token = this.opts.getToken();
    if (!token || this.stopped) return;
    this.setStatus(this.everOpened ? "reconnecting" : "connecting");
    let sock: SocketLike;
    try {
      sock = await openSocket(`${WS_URL}?token=${encodeURIComponent(token)}`);
    } catch {
      this.scheduleReconnect();
      return;
    }
    if (this.stopped) {
      sock.close();
      return;
    }
    this.socket = sock;
    sock.onopen = () => {
      const isReconnect = this.everOpened;
      this.everOpened = true;
      this.attempt = 0;
      this.setStatus("open");
      if (this.sessionId) this.send({ type: "subscribe", data: { sessionId: this.sessionId } });
      this.clearPing();
      this.pingTimer = setInterval(() => this.send({ type: "ping", data: {} }), PING_MS);
      if (isReconnect) this.opts.onReconnect?.();
    };
    sock.onmessage = (ev) => {
      if (typeof ev.data !== "string") return;
      // Server may batch several JSON envelopes separated by newlines.
      for (const line of ev.data.split("\n")) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        try {
          const parsed = JSON.parse(trimmed) as ServerEvent;
          if (parsed && typeof parsed.type === "string") this.opts.onEvent(parsed);
        } catch {
          // ignore malformed frames
        }
      }
    };
    sock.onerror = () => {
      // onclose will follow and handle reconnect
    };
    sock.onclose = () => {
      this.clearPing();
      if (this.socket === sock) this.socket = null;
      if (this.stopped) {
        this.setStatus("closed");
        return;
      }
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (this.stopped) return;
    this.setStatus("reconnecting");
    const base = Math.min(MAX_BACKOFF_MS, 500 * 2 ** this.attempt);
    const delay = base / 2 + Math.random() * (base / 2);
    this.attempt++;
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = setTimeout(() => void this.open(), delay);
  }

  private clearPing() {
    if (this.pingTimer) clearInterval(this.pingTimer);
    this.pingTimer = null;
  }

  subscribe(sessionId: string) {
    this.sessionId = sessionId;
    this.send({ type: "subscribe", data: { sessionId } });
  }

  sendTyping(channelId: string) {
    if (!this.sessionId) return;
    this.send({ type: "typing", data: { sessionId: this.sessionId, channelId } });
  }

  send(ev: ClientEvent) {
    if (this.socket && this.socket.readyState === 1) {
      this.socket.send(JSON.stringify(ev));
    }
  }

  close() {
    this.stopped = true;
    this.clearPing();
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = null;
    this.socket?.close(1000, "client closed");
    this.socket = null;
    this.setStatus("closed");
  }
}
