// Single-port dev gateway: /api + /ws -> Go backend, everything else -> Next.js.
import http from "node:http";
import net from "node:net";

const PORT = Number(process.env.PROXY_PORT || 5323);
const BACKEND = { host: "127.0.0.1", port: Number(process.env.BACKEND_PORT || 8080) };
const FRONTEND = { host: "127.0.0.1", port: Number(process.env.FRONTEND_PORT || 3000) };

const target = (url) => (url.startsWith("/api") || url.startsWith("/ws") ? BACKEND : FRONTEND);

const server = http.createServer((req, res) => {
  const t = target(req.url);
  const up = http.request({ ...t, path: req.url, method: req.method, headers: req.headers }, (r) => {
    res.writeHead(r.statusCode, r.headers);
    r.pipe(res);
  });
  up.on("error", () => { res.writeHead(502); res.end("upstream unavailable"); });
  req.pipe(up);
});

// Raw TCP pass-through for WebSocket upgrades (backend WS and Next HMR).
server.on("upgrade", (req, socket, head) => {
  const t = target(req.url);
  const up = net.connect(t.port, t.host, () => {
    const lines = [`${req.method} ${req.url} HTTP/${req.httpVersion}`];
    for (let i = 0; i < req.rawHeaders.length; i += 2) lines.push(`${req.rawHeaders[i]}: ${req.rawHeaders[i + 1]}`);
    up.write(lines.join("\r\n") + "\r\n\r\n");
    if (head?.length) up.write(head);
    up.pipe(socket).pipe(up);
  });
  up.on("error", () => socket.destroy());
  socket.on("error", () => up.destroy());
});

server.listen(PORT, "0.0.0.0", () => console.log(`proxy :${PORT} -> api/ws :${BACKEND.port}, web :${FRONTEND.port}`));
