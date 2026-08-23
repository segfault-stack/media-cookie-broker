import {parseBrokerEndpoint} from "./config.js";
import {getProvider} from "./providers/registry.js";

const BATCHES_KEY = "diagnosticBatches";
const STATE_KEY = "diagnosticState";
const INSTALLATION_KEY = "diagnosticInstallationId";
const RETENTION_MS = 30 * 24 * 60 * 60 * 1000;
const MAX_EVENTS = 10000;
const EVENTS_PER_BATCH = 100;
const MAX_DIAGNOSTIC_DEPTH = 4;
let lock = Promise.resolve();

function sensitiveDiagnosticKey(key) {
  const normalized = String(key).replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_|_$/g, "");
  const compact = normalized.replaceAll("_", "");
  const parts = normalized.split("_");
  if (parts.some(part => ["password", "passwd", "authorization", "secret"].includes(part))) return true;
  const pairs = new Set(parts.slice(0, -1).map((part, index) => `${part}_${parts[index + 1]}`));
  if (["access_token", "refresh_token", "bearer_token", "id_token", "api_key", "auth_header", "basic_auth", "bearer_auth", "basic_credentials", "bearer_credentials", "cookie_value", "cookie_header", "master_key", "private_key"].some(pair => pairs.has(pair))) return true;
  return normalized === "token" || normalized.endsWith("_token") ||
    normalized === "secret" || normalized.endsWith("_secret") ||
    normalized === "api_key" || normalized === "apikey" || normalized.endsWith("_api_key") || normalized.endsWith("_apikey") ||
    normalized === "auth_header" || normalized.endsWith("_auth_header") ||
    normalized === "basic_auth" || normalized.endsWith("_basic_auth") ||
    normalized === "bearer_auth" || normalized.endsWith("_bearer_auth") ||
    normalized === "basic_credentials" || normalized.endsWith("_basic_credentials") ||
    normalized === "bearer_credentials" || normalized.endsWith("_bearer_credentials") ||
    normalized === "cookie" || normalized === "cookies" || normalized.endsWith("_cookie") || normalized.endsWith("_cookies") ||
    normalized === "cookie_value" || normalized.endsWith("_cookie_value") ||
    normalized === "cookie_header" || normalized.endsWith("_cookie_header") ||
    normalized === "master_key" || normalized.endsWith("_master_key") ||
    normalized === "private_key" || normalized.endsWith("_private_key") ||
    compact.includes("apikey") || compact.includes("cookievalue") || compact.includes("cookieheader") ||
    compact.includes("masterkey") || compact.includes("privatekey") || compact.includes("accesstoken") ||
    compact.includes("refreshtoken") || compact.includes("bearertoken");
}

function redactText(value) {
  return value
    .replace(/(authorization\s*[:=]\s*)[^,;\r\n]+/ig, "$1[redacted]")
    .replace(/(cookie\s*:\s*)[^,\r\n]+/ig, "$1[redacted]")
    .replace(/-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----/ig, "[redacted private key]")
    .replace(/\b(bearer|basic)\s+[A-Za-z0-9+/._~=-]+/ig, "$1 [redacted]")
    .replace(/\b(token|access_token|api_key|apikey|secret|password|passwd|master_key|private_key)\s*=\s*[^\s&,;]+/ig, "$1=[redacted]")
    .slice(0, 4096);
}

export function redactDiagnosticValue(value, key = "", depth = 0) {
  if (sensitiveDiagnosticKey(key)) return "[redacted]";
  if (typeof value === "string") return redactText(value);
  if (depth >= MAX_DIAGNOSTIC_DEPTH && value && typeof value === "object") return "[truncated]";
  if (Array.isArray(value)) return value.map(item => redactDiagnosticValue(item, "", depth + 1));
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([name, item]) => [name, redactDiagnosticValue(item, name, depth + 1)]));
  return value;
}

async function gzip(value) {
  const stream = new Blob([JSON.stringify(value)]).stream().pipeThrough(new CompressionStream("gzip"));
  const bytes = new Uint8Array(await new Response(stream).arrayBuffer());
  return btoa(String.fromCharCode(...bytes));
}
async function gunzip(value) {
  const bytes = Uint8Array.from(atob(value), char => char.charCodeAt(0));
  return new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"))).json();
}
async function batches() { return (await chrome.storage.local.get(BATCHES_KEY))[BATCHES_KEY] || []; }

export async function installationId() {
  const current = (await chrome.storage.local.get(INSTALLATION_KEY))[INSTALLATION_KEY];
  if (current) return current;
  const id = crypto.randomUUID();
  await chrome.storage.local.set({[INSTALLATION_KEY]: id});
  return id;
}

export async function logDiagnostic(type, details = {}, severity = "info") {
  lock = lock.then(async () => {
    const event = {timestamp: new Date().toISOString(), type, severity, details: redactDiagnosticValue(details)};
    const stored = await batches();
    let batch = stored.at(-1), events = [];
    if (batch && batch.count < EVENTS_PER_BATCH) events = await gunzip(batch.gzip);
    else { batch = {firstAt: event.timestamp, lastAt: event.timestamp, count: 0, gzip: "", pending: true}; stored.push(batch); }
    events.push(event); batch.lastAt = event.timestamp; batch.count = events.length; batch.gzip = await gzip(events); batch.pending = true;
    let count = stored.reduce((n, item) => n + item.count, 0), cutoff = Date.now() - RETENTION_MS;
    // FIFO pruning: remove the oldest events, not the newest events or browser cookies.
    while (stored.length && (Date.parse(stored[0].lastAt) < cutoff || count > MAX_EVENTS)) {
      const oldest = stored[0];
      const oldestEvents = await gunzip(oldest.gzip);
      const removeForAge = oldestEvents.filter(item => Date.parse(item.timestamp) >= cutoff);
      const keepCount = Math.max(0, Math.min(removeForAge.length, MAX_EVENTS - (count - oldestEvents.length)));
      const keep = removeForAge.slice(Math.max(0, removeForAge.length - keepCount));
      count -= oldestEvents.length;
      if (keep.length) {
        oldest.firstAt = keep[0].timestamp;
        oldest.lastAt = keep.at(-1).timestamp;
        oldest.count = keep.length;
        oldest.gzip = await gzip(keep);
        count += keep.length;
        break;
      }
      stored.shift();
    }
    await chrome.storage.local.set({[BATCHES_KEY]: stored});
  }).catch(() => {});
  return lock;
}

export async function recentDiagnostics() {
  const events = [];
  for (const batch of await batches()) try { events.push(...await gunzip(batch.gzip)); } catch (_) {}
  return events.sort((a, b) => b.timestamp.localeCompare(a.timestamp));
}
export async function diagnosticsState() {
  const stored = await batches(), state = (await chrome.storage.local.get(STATE_KEY))[STATE_KEY] || {};
  const pending = stored.filter(item => item.pending);
  return {...state, queuedBatches: pending.length, queuedEvents: pending.reduce((n, item) => n + item.count, 0)};
}
export async function clearDiagnostics() { await chrome.storage.local.set({[BATCHES_KEY]: [], [STATE_KEY]: {lastClearedAt: new Date().toISOString()}}); }

export async function uploadDiagnostics(cfg, basic) {
  if (cfg.remoteDiagnosticsEnabled !== true) return true;
  const state = (await chrome.storage.local.get(STATE_KEY))[STATE_KEY] || {};
  if (!cfg.password || state.nextAttemptAt > Date.now()) return false;
  const stored = await batches(), pending = stored.filter(item => item.pending).slice(0, 10);
  if (!pending.length) return true;
  const selected = [];
  const grouped = new Map();
  const completed = new Set();
  const updateBatches = () => stored.map(batch => {
    const item = selected.find(candidate => candidate.batch === batch);
    if (!item) return batch;
    const uploadedScopes = new Set(batch.uploadedScopes || []);
    for (const scope of completed) uploadedScopes.add(scope);
    const stillPending = [...item.neededScopes].some(scope => !uploadedScopes.has(scope));
    return {...batch, uploadedScopes: [...uploadedScopes], pending: stillPending};
  });
  try {
    for (const batch of pending) {
      const events = await gunzip(batch.gzip);
      const neededScopes = new Set();
      const uploadedScopes = new Set(batch.uploadedScopes || []);
      for (const event of events) {
        const provider = event.details?.provider;
        if (!getProvider(provider) || !cfg.enabledProviders.includes(provider)) continue;
        const profile = event.details?.profile || "default";
        const scope = `${provider}/${profile}`;
        neededScopes.add(scope);
        if (uploadedScopes.has(scope)) continue;
        if (!grouped.has(scope)) grouped.set(scope, {provider, profile, events: []});
        grouped.get(scope).events.push(event);
      }
      selected.push({batch, neededScopes});
    }
    for (const [scope, group] of grouped) {
      const data = JSON.stringify({schema_version: 1, provider: group.provider, profile: group.profile, installation_id: await installationId(), events: group.events});
      const compressed = await new Response(new Blob([data]).stream().pipeThrough(new CompressionStream("gzip"))).arrayBuffer();
      const url = new URL(parseBrokerEndpoint(cfg.endpoint));
      url.pathname = `${url.pathname.replace(/\/$/, "")}/v1/diagnostics/events`;
      const response = await fetch(url, {
        method: "POST",
        headers: {Authorization: basic(cfg.username, cfg.password), "Content-Type": "application/json", "Content-Encoding": "gzip"},
        body: compressed
      });
      if (!response.ok) throw new Error(`${scope}: HTTP ${response.status}`);
      completed.add(scope);
    }
    await chrome.storage.local.set({
      [BATCHES_KEY]: updateBatches(),
      [STATE_KEY]: {lastUploadedAt: new Date().toISOString(), failures: 0}
    });
    return true; // Successful acknowledgements are deliberately not logged.
  } catch (error) {
    const failures = Number(state.failures || 0) + 1;
    await chrome.storage.local.set({
      [BATCHES_KEY]: updateBatches(),
      [STATE_KEY]: {failures, lastError: String(error?.message || error), nextAttemptAt: Date.now() + Math.min(3600000, 1000 * 2 ** failures)}
    });
    return false;
  }
}
