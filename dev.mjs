#!/usr/bin/env node
// dev.mjs — start the full ARgus stack (Windows, macOS, Linux)
// Usage: npm start  OR  node dev.mjs

import { spawn, execSync } from "child_process";
import { readFileSync, writeFileSync, existsSync } from "fs";
import { resolve } from "path";
import { createServer } from "net";

const ROOT = resolve(import.meta.dirname || ".");
const FE_DIR = resolve(ROOT, "frontend/web-client");
const BACKEND_PORT = 8080;
const FRONTEND_PORT = 3000;

// ── Colour helpers ──────────────────────────────────────────────────────────
const c = (code, msg) => `\x1b[${code}m${msg}\x1b[0m`;
const info = (m) => console.log(c("36", `▸ ${m}`));
const ok = (m) => console.log(c("32", `✓ ${m}`));
const warn = (m) => console.log(c("33", `⚠ ${m}`));
const fatal = (m) => { console.error(c("31", `✗ ${m}`)); process.exit(1); };

// ── Load .env ───────────────────────────────────────────────────────────────
function loadEnv(file) {
  if (!existsSync(file)) return;
  for (const line of readFileSync(file, "utf8").split("\n")) {
    const m = line.match(/^\s*([^#][^=]*?)\s*=\s*(.+)$/);
    if (m && !process.env[m[1].trim()]) process.env[m[1].trim()] = m[2].trim();
  }
}

// ── Port check ──────────────────────────────────────────────────────────────
function waitForPort(port, timeoutMs = 15000) {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const check = () => {
      const sock = createServer();
      sock.once("error", () => {
        // Port is in use — something is listening, which is what we want
        sock.close();
        resolve();
      });
      sock.once("listening", () => {
        // Port is free — server not up yet
        sock.close();
        if (Date.now() - start > timeoutMs) return reject(new Error(`Timeout waiting for port ${port}`));
        setTimeout(check, 500);
      });
      sock.listen(port);
    };
    check();
  });
}

// ── Pre-flight ──────────────────────────────────────────────────────────────
info("Checking environment...");

if (!existsSync(resolve(ROOT, "go.mod"))) fatal("Run this from the ARgus repo root.");

loadEnv(resolve(ROOT, ".env"));

if (!process.env.GEMINI_API_KEY || process.env.GEMINI_API_KEY === "your_gemini_api_key_here") {
  fatal("GEMINI_API_KEY is not set. Copy .env.example to .env and add your key.");
}
ok("GEMINI_API_KEY present");

const envLocal = resolve(FE_DIR, ".env.local");
if (!existsSync(envLocal)) {
  warn(".env.local missing — creating with local defaults");
  writeFileSync(envLocal, `NEXT_PUBLIC_WS_URL=ws://localhost:${BACKEND_PORT}/ws\n`);
  ok("Created frontend/web-client/.env.local");
}
ok(".env.local present");

if (!existsSync(resolve(FE_DIR, "node_modules"))) {
  info("Installing frontend dependencies (first run)...");
  execSync("npm install", { cwd: FE_DIR, stdio: "inherit" });
  ok("npm install done");
}

// ── Start backend ───────────────────────────────────────────────────────────
info(`Starting Go backend on :${BACKEND_PORT}...`);
const isWin = process.platform === "win32";
const backend = spawn("go", ["run", "./cmd/server"], {
  cwd: ROOT,
  env: { ...process.env, PORT: String(BACKEND_PORT) },
  stdio: ["ignore", "pipe", "pipe"],
  shell: isWin,
});
backend.stdout.on("data", (d) => process.stdout.write(`  [backend] ${d}`));
backend.stderr.on("data", (d) => process.stderr.write(`  [backend] ${d}`));

await waitForPort(BACKEND_PORT, 30000).then(() => ok("Backend ready")).catch(() => fatal("Backend did not start within 30s"));

// ── Start frontend ──────────────────────────────────────────────────────────
info(`Starting Next.js frontend on :${FRONTEND_PORT}...`);
const frontend = spawn("npm", ["run", "dev"], {
  cwd: FE_DIR,
  env: { ...process.env, PORT: String(FRONTEND_PORT) },
  stdio: ["ignore", "pipe", "pipe"],
  shell: isWin,
});
frontend.stdout.on("data", (d) => process.stdout.write(`  [frontend] ${d}`));
frontend.stderr.on("data", (d) => process.stderr.write(`  [frontend] ${d}`));

await waitForPort(FRONTEND_PORT, 30000).catch(() => warn("Frontend still compiling..."));

// ── Summary ─────────────────────────────────────────────────────────────────
const token = (process.env.DEMO_TOKENS || "ARGUS-DEMO1").split(",")[0];
console.log(`
${c("32", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")}
${c("32", "  ARgus dev stack running")}
${c("32", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")}
  App      →  ${c("36", `http://localhost:${FRONTEND_PORT}/session`)}
  Backend  →  ${c("36", `http://localhost:${BACKEND_PORT}`)}
  Token    →  ${c("33", token)}
${c("32", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")}

  Press ${c("33", "Ctrl-C")} to stop both servers
`);

// ── Cleanup ─────────────────────────────────────────────────────────────────
function shutdown() {
  info("Shutting down...");
  backend.kill();
  frontend.kill();
  ok("Stopped.");
  process.exit(0);
}
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
process.on("exit", shutdown);

// Keep alive
await new Promise(() => {});
