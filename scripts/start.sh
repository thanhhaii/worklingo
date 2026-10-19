#!/usr/bin/env bash
# Start the full dev stack: Postgres (docker) -> Go backend -> Next.js -> dev proxy.
# Open http://localhost:5323 when it's up. Ctrl+C stops everything this script started.
#
# Env overrides: BACKEND_PORT (8080), FRONTEND_PORT (3000), PROXY_PORT (5323).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT/.logs"
export BACKEND_PORT="${BACKEND_PORT:-8080}"
export FRONTEND_PORT="${FRONTEND_PORT:-3000}"
export PROXY_PORT="${PROXY_PORT:-5323}"
DB_PORT=5433
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

mkdir -p "$LOG_DIR"
PIDS=()

log() { printf '\033[1;36m[start]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[start]\033[0m %s\n' "$*" >&2; exit 1; }

port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

wait_for() { # wait_for <name> <seconds> <command...>
  local name=$1 secs=$2; shift 2
  for _ in $(seq "$secs"); do
    "$@" && { log "$name ready"; return 0; }
    sleep 1
  done
  die "$name not ready after ${secs}s (see $LOG_DIR)"
}

cleanup() {
  trap - INT TERM EXIT
  log "stopping..."
  for pid in "${PIDS[@]}"; do
    # Kill the whole process group so children (go run, next) die too.
    kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  log "stopped (Postgres container left running; 'docker compose down' to stop it)"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

start_bg() { # start_bg <name> <dir> <command...>
  local name=$1 dir=$2; shift 2
  (cd "$dir" && exec setsid "$@") >"$LOG_DIR/$name.log" 2>&1 &
  PIDS+=("$!")
  log "$name started (pid $!, log .logs/$name.log)"
}

for p in "$BACKEND_PORT" "$FRONTEND_PORT" "$PROXY_PORT"; do
  port_open "$p" && die "port $p already in use - is the stack already running?"
done

# 1. Postgres
if port_open "$DB_PORT"; then
  log "Postgres already listening on :$DB_PORT"
else
  log "starting Postgres via docker compose..."
  if docker info >/dev/null 2>&1; then
    docker compose -f "$ROOT/docker-compose.yml" up -d postgres
  else
    sudo docker compose -f "$ROOT/docker-compose.yml" up -d postgres
  fi
  wait_for "Postgres" 60 port_open "$DB_PORT"
fi

# 2. Backend (runs migrations on boot)
[ -f "$ROOT/backend/.env" ] || die "backend/.env missing - copy backend/.env.example and fill it in"
start_bg backend "$ROOT/backend" go run ./cmd/api
wait_for "backend" 120 curl -sf "http://127.0.0.1:$BACKEND_PORT/api/health" -o /dev/null

# 3. Frontend
if [ ! -d "$ROOT/frontend/node_modules" ]; then
  log "installing frontend deps..."
  (cd "$ROOT/frontend" && pnpm install)
fi
start_bg frontend "$ROOT/frontend" pnpm dev --port "$FRONTEND_PORT"
wait_for "frontend" 120 port_open "$FRONTEND_PORT"

# 4. Single-port proxy
start_bg proxy "$ROOT" node scripts/dev-proxy.mjs
wait_for "proxy" 15 port_open "$PROXY_PORT"

log "all up -> http://localhost:$PROXY_PORT   (Ctrl+C to stop)"

# Exit (and clean up) as soon as any service dies.
wait -n "${PIDS[@]}" || true
log "a service exited unexpectedly - check $LOG_DIR"
