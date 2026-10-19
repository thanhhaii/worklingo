import { API_URL, MOCK } from "./config";
import type {
  AuthResponse,
  Evaluation,
  Message,
  Persona,
  Session,
  SessionDetails,
  User,
} from "./types";

const TOKEN_KEY = "worklingo.token";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null) {
  if (typeof window === "undefined") return;
  if (token) window.localStorage.setItem(TOKEN_KEY, token);
  else window.localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

type Handler401 = () => void;
let on401: Handler401 | null = null;
export function setUnauthorizedHandler(fn: Handler401 | null) {
  on401 = fn;
}

export type HttpRequest = { method: string; path: string; body?: unknown; token: string | null };
export type HttpResponse = { status: number; body: unknown };
export type Transport = (req: HttpRequest) => Promise<HttpResponse>;

const fetchTransport: Transport = async ({ method, path, body, token }) => {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers["Authorization"] = `Bearer ${token}`;
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError(0, "network_error", "Cannot reach the WorkLingo server. Is the backend running?");
  }
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: res.status, body: parsed };
};

let transportPromise: Promise<Transport> | null = null;
function getTransport(): Promise<Transport> {
  if (!transportPromise) {
    transportPromise = MOCK
      ? import("./mock/server").then((m) => m.mockTransport)
      : Promise.resolve(fetchTransport);
  }
  return transportPromise;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const transport = await getTransport();
  const token = getToken();
  const { status, body: resBody } = await transport({ method, path, body, token });
  if (status >= 200 && status < 300) return resBody as T;
  const err = (resBody as { error?: { code?: string; message?: string } } | null)?.error;
  const apiErr = new ApiError(status, err?.code || `http_${status}`, err?.message || `Request failed (${status})`);
  if (status === 401 && token && !path.startsWith("/auth/")) on401?.();
  throw apiErr;
}

function enc(s: string) {
  return encodeURIComponent(s);
}

export const api = {
  health: () => request<{ ok: boolean }>("GET", "/health"),
  register: (username: string, email: string, password: string) =>
    request<AuthResponse>("POST", "/auth/register", { username, email, password }),
  login: (email: string, password: string) => request<AuthResponse>("POST", "/auth/login", { email, password }),
  me: () => request<User>("GET", "/me"),
  personas: () => request<Persona[]>("GET", "/personas"),
  createSession: () => request<Session>("POST", "/sessions", {}),
  activeSession: async (): Promise<Session | null> => {
    try {
      return await request<Session>("GET", "/sessions/active");
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
  },
  sessions: () => request<Session[]>("GET", "/sessions"),
  session: (id: string) => request<SessionDetails>("GET", `/sessions/${enc(id)}`),
  endSession: (id: string) => request<Session>("POST", `/sessions/${enc(id)}/end`),
  messages: (sessionId: string, channelId: string, opts: { before?: string; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (opts.before) q.set("before", opts.before);
    q.set("limit", String(opts.limit ?? 50));
    return request<Message[]>("GET", `/sessions/${enc(sessionId)}/channels/${enc(channelId)}/messages?${q}`);
  },
  postMessage: (sessionId: string, channelId: string, content: string, parentId?: string | null) =>
    request<Message>(
      "POST",
      `/sessions/${enc(sessionId)}/channels/${enc(channelId)}/messages`,
      parentId ? { content, parentId } : { content },
    ),
  evaluation: async (sessionId: string): Promise<Evaluation | null> => {
    try {
      return await request<Evaluation>("GET", `/sessions/${enc(sessionId)}/evaluation`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
  },
};

export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}
