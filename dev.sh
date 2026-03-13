#!/usr/bin/env bash
# dev.sh — start the full ARgus stack locally
# Works on Windows (Git Bash / MSYS2), macOS, and Linux.
# Usage: ./dev.sh [--port-backend 8080] [--port-frontend 3000]

BACKEND_PORT=8080
FRONTEND_PORT=3000

while [[ $# -gt 0 ]]; do
  case "$1" in
    --port-backend)  BACKEND_PORT="$2"; shift 2 ;;
    --port-frontend) FRONTEND_PORT="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# ── Colour helpers ───────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; RESET='\033[0m'
info()  { echo -e "${CYAN}▸ $*${RESET}"; }
ok()    { echo -e "${GREEN}✓ $*${RESET}"; }
warn()  { echo -e "${YELLOW}⚠ $*${RESET}"; }
fatal() { echo -e "${RED}✗ $*${RESET}"; exit 1; }

# ── Port probe — curl only (works on Windows, macOS, Linux) ─────────────────
port_open() {
  # curl to a non-existent path; any HTTP response (even 404) means the port is open
  curl -s --max-time 1 "http://localhost:$1/" -o /dev/null 2>/dev/null
}

# ── Kill whatever is on a port ───────────────────────────────────────────────
free_port() {
  local port=$1 pid=""
  if command -v netstat &>/dev/null && command -v taskkill &>/dev/null; then
    # Windows
    pid=$(netstat -ano 2>/dev/null | grep ":${port}[[:space:]].*LISTENING" | awk '{print $NF}' | head -1 || true)
    if [[ -n "$pid" && "$pid" != "0" ]]; then
      warn "Killing stale process on :$port (pid $pid)"
      taskkill //PID "$pid" //F >/dev/null 2>&1 || true
      sleep 0.6
    fi
  elif command -v lsof &>/dev/null; then
    # macOS / Linux
    pid=$(lsof -ti tcp:"$port" 2>/dev/null || true)
    if [[ -n "$pid" ]]; then
      warn "Killing stale process on :$port (pid $pid)"
      kill "$pid" 2>/dev/null || true
      sleep 0.6
    fi
  fi
}

# ── Pre-flight ───────────────────────────────────────────────────────────────
info "Checking environment..."

[[ -f go.mod ]] || fatal "Run this from the ARgus repo root."

set -a
source .env 2>/dev/null || true
set +a

[[ -n "${GEMINI_API_KEY:-}" ]] || fatal "GEMINI_API_KEY is not set in .env"
ok "GEMINI_API_KEY present"

[[ -f frontend/web-client/.env.local ]] || {
  warn ".env.local missing — creating with local defaults"
  echo "NEXT_PUBLIC_WS_URL=ws://localhost:${BACKEND_PORT}/ws" > frontend/web-client/.env.local
  ok "Created frontend/web-client/.env.local"
}
ok ".env.local present"

if [[ ! -d frontend/web-client/node_modules ]]; then
  info "Installing frontend dependencies (first run)..."
  (cd frontend/web-client && npm install --silent)
  ok "npm install done"
fi

# ── Clear stale processes ────────────────────────────────────────────────────
free_port "$BACKEND_PORT"
free_port "$FRONTEND_PORT"

# ── Start backend ────────────────────────────────────────────────────────────
info "Starting Go backend on :${BACKEND_PORT}..."
PORT=$BACKEND_PORT go run ./cmd/server 2>&1 | sed "s/^/  [backend] /" &
BACKEND_PID=$!

for i in $(seq 1 30); do
  port_open "$BACKEND_PORT" && { ok "Backend ready"; break; }
  if [[ $i -eq 30 ]]; then fatal "Backend did not start within 15s"; fi
  sleep 0.5
done

# ── Start frontend ───────────────────────────────────────────────────────────
info "Starting Next.js frontend on :${FRONTEND_PORT}..."
(cd frontend/web-client && PORT=$FRONTEND_PORT npm run dev 2>&1 | sed "s/^/  [frontend] /") &
FRONTEND_PID=$!

for i in $(seq 1 60); do
  port_open "$FRONTEND_PORT" && break
  if [[ $i -eq 60 ]]; then
    warn "Frontend taking long — still compiling, opening browser anyway"
    break
  fi
  sleep 0.5
done

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${GREEN}  ARgus dev stack running${RESET}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "  App      →  ${CYAN}http://localhost:${FRONTEND_PORT}/session${RESET}"
echo -e "  Backend  →  ${CYAN}http://localhost:${BACKEND_PORT}${RESET}"
echo -e "  Token    →  ${YELLOW}${DEMO_TOKENS%%,*}${RESET}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo ""
echo -e "  Press ${YELLOW}Ctrl-C${RESET} to stop both servers"

cleanup() {
  echo ""
  info "Shutting down..."
  kill "$BACKEND_PID" "$FRONTEND_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  ok "Stopped."
}
trap cleanup INT TERM

wait
