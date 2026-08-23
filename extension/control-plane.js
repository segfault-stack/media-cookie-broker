export const DEFAULT_EXPIRY_THRESHOLDS = [24, 6, 1];

export function validProfileID(profile) {
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(String(profile || ""));
}

export function scopeKey(provider, profile = "default") {
  return `${provider}/${profile || "default"}`;
}

export function createPollCoordinator(operation) {
  let inFlight = null;

  function run() {
    if (inFlight) return inFlight;
    const current = Promise.resolve().then(operation).finally(() => {
      if (inFlight === current) inFlight = null;
    });
    inFlight = current;
    return current;
  }

  async function runAfterCurrent() {
    const previous = inFlight;
    if (previous) {
      try { await previous; } catch (_) {}
    }
    return run();
  }

  return {run, runAfterCurrent};
}

export async function refreshStatusAfterPublication(refreshStatus, onError = async () => {}) {
  try {
    return Boolean(await refreshStatus());
  } catch (error) {
    try { await onError(error); } catch (_) {}
    return false;
  }
}

export function normalizeExpiryThresholds(value) {
  const items = Array.isArray(value) ? value : String(value ?? "").split(",");
  const values = items
    .map(item => Number(String(item).trim()))
    .filter(item => Number.isFinite(item) && item > 0 && item <= 8760)
    .map(item => Math.round(item * 100) / 100);
  return [...new Set(values)].sort((a, b) => b - a);
}

export function evaluateBrokerStatus(status, previous = {}, thresholds = DEFAULT_EXPIRY_THRESHOLDS, now = Date.now()) {
  const revision = Number(status.revision || 0);
  const sameRevision = Number(previous.revision || 0) === revision;
  const state = sameRevision
    ? {...previous, firedExpiryThresholds: [...(previous.firedExpiryThresholds || [])]}
    : {revision, firedExpiryThresholds: [], expiryHintPassed: false};
  const notifications = [];
  const health = status.auth_health || "missing";
  if (health === "refresh_required" && (!sameRevision || previous.health !== "refresh_required")) {
    notifications.push({event: "refresh_required"});
  }
  state.health = health;

  const expiresAt = Date.parse(status.auth_expires_at || "");
  if (revision > 0 && Number.isFinite(expiresAt)) {
    const remainingHours = (expiresAt - now) / 3600000;
    const normalized = normalizeExpiryThresholds(thresholds);
    const fired = new Set(state.firedExpiryThresholds);
    if (remainingHours <= 0) {
      if (!state.expiryHintPassed) notifications.push({event: "expiry_hint_passed"});
      state.expiryHintPassed = true;
      for (const threshold of normalized) fired.add(threshold);
      state.firedExpiryThresholds = [...fired].sort((a, b) => b - a);
    } else {
      const crossed = normalized.filter(threshold => remainingHours <= threshold && !fired.has(threshold));
      if (crossed.length) {
        const currentThreshold = Math.min(...crossed);
        notifications.push({event: "expiry_threshold", thresholdHours: currentThreshold, remainingHours});
        for (const threshold of crossed) fired.add(threshold);
        state.firedExpiryThresholds = [...fired].sort((a, b) => b - a);
      }
    }
  }
  return {state, notifications};
}

export function expiryHintText(value, now = Date.now()) {
  const expires = Date.parse(value || "");
  if (!Number.isFinite(expires)) return "";
  if (expires <= now) return " · auth-expiry hint passed";
  const hours = (expires - now) / 3600000;
  return ` · auth-expiry hint in ~${hours < 1 ? "<1" : Math.ceil(hours)}h`;
}

export function profileVisualState(status, capture = {}) {
  if (status.auth_health === "refresh_required" || capture.ok === false) return "error";
  if (status.auth_health === "healthy" && Number(status.revision || 0) > 0) return "ok";
  return "warning";
}
