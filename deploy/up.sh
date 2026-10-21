#!/usr/bin/env bash
# One-shot deploy on this host (run as root): fill missing secrets, build+start stack, install nginx site.
set -euo pipefail
cd "$(dirname "$0")/.."
ENV=.env.prod

# Generate secrets once if empty (POSTGRES_PASSWORD only takes effect on first DB init).
fill() { # fill <VAR> <bytes>
  if ! grep -qE "^$1=.+" "$ENV"; then
    local v; v=$(openssl rand -hex "$2")
    if grep -qE "^$1=" "$ENV"; then sed -i "s|^$1=.*|$1=$v|" "$ENV"; else echo "$1=$v" >>"$ENV"; fi
    echo "generated $1"
  fi
}
fill POSTGRES_PASSWORD 24
fill JWT_SECRET 48
chmod 600 "$ENV"

docker compose -f docker-compose.prod.yml --env-file "$ENV" up -d --build

install -m 644 deploy/nginx/worklingo.conf /etc/nginx/sites-available/worklingo
ln -sf /etc/nginx/sites-available/worklingo /etc/nginx/sites-enabled/worklingo
nginx -t
systemctl reload nginx

for _ in $(seq 60); do curl -sf http://127.0.0.1:8088/api/health >/dev/null && break; sleep 2; done
docker compose -f docker-compose.prod.yml --env-file "$ENV" ps
echo "local:  $(curl -s http://127.0.0.1:8088/api/health)"
echo "public: $(curl -s https://worklingo.hailam.dev/api/health)"
