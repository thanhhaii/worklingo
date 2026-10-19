// NEXT_PUBLIC_API_URL / NEXT_PUBLIC_WS_URL may be absolute or same-origin relative ("/api", "/ws").
function resolveWs(raw: string): string {
  if (!raw.startsWith("/") || typeof window === "undefined") return raw;
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}${raw}`;
}

export const API_URL = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api").replace(/\/$/, "");
export const WS_URL = resolveWs((process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080/ws").replace(/\/$/, ""));
export const MOCK = process.env.NEXT_PUBLIC_MOCK === "1" || process.env.NEXT_PUBLIC_MOCK === "true";
