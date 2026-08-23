import {configurationError, DEFAULTS, parseBrokerEndpoint} from "./config.js";
import {createPollCoordinator, evaluateBrokerStatus, normalizeExpiryThresholds, refreshStatusAfterPublication, scopeKey, validProfileID} from "./control-plane.js";
import {installationId, logDiagnostic, uploadDiagnostics} from "./diagnostics.js";
import {getProvider, listProviders, providerForCookieDomain} from "./providers/registry.js";
import {createRecoveryCoordinator, recoveryMatchesTab, resolveNormalCookieStore, resolveRecoveryCaptureContext} from "./recovery.js";

const PERIODIC_ALARM = "media-cookie-broker-periodic";
const CHANGE_ALARM_PREFIX = "cookie-change:";
const MIN_INTERVAL_MINUTES = 2;
const MAX_INTERVAL_MINUTES = 5;
const RETRIES = 3;
const RETRY_DELAYS_MS = [1000, 3000, 10000];
const RECOVERY_CONTEXTS_KEY = "recoveryContexts";
let cycleInFlight = null;

async function settings() {
  const config = {...DEFAULTS, ...await chrome.storage.local.get(Object.keys(DEFAULTS))};
  config.selectedProfiles = {...DEFAULTS.selectedProfiles, ...(config.selectedProfiles || {})};
  config.expiryThresholdHours = normalizeExpiryThresholds(config.expiryThresholdHours);
  return config;
}

function selectedProfile(config, providerID) {
  const profile = config.selectedProfiles?.[providerID] || "default";
  return validProfileID(profile) ? profile : "default";
}

function expectedCaptureMode(provider, config) {
  if (provider.recovery?.isolationConfigurable) return config.youtubeUseIncognito ? "incognito" : "normal";
  return provider.captureContext === "ephemeral-incognito" ? "incognito" : "normal";
}

async function captureContext(provider, profile, config, recoveryContext) {
  if (recoveryContext) {
    return resolveRecoveryCaptureContext(chrome, provider.id, profile, recoveryContext);
  }
  if (expectedCaptureMode(provider, config) === "incognito") return null;
  const store = await resolveNormalCookieStore(chrome);
  return {provider: provider.id, profile, recoveryMode: "normal", cookieStoreId: store.id};
}

async function collect(provider, profile, context) {
  const storeId = context.cookieStoreId;
  const byKey = new Map();
  const now = Math.floor(Date.now() / 1000);
  for (const domain of provider.cookieDomains) {
    const query = {domain};
    if (storeId !== undefined) query.storeId = storeId;
    for (const cookie of await chrome.cookies.getAll(query)) {
      if (providerForCookieDomain(cookie.domain)?.id !== provider.id || cookie.partitionKey) continue;
      const expiration = Math.trunc(cookie.expirationDate || 0);
      if (expiration && expiration <= now) continue;
      const item = {
        domain: cookie.domain.toLowerCase(),
        path: cookie.path || "/",
        name: cookie.name,
        value: cookie.value,
        expiration,
        secure: cookie.secure,
        http_only: cookie.httpOnly,
        same_site: cookie.sameSite || "unspecified"
      };
      byKey.set(`${item.domain}\0${item.path}\0${item.name}`, item);
    }
  }
  const cookies = [...byKey.values()];
  await logDiagnostic("cookies_collected", {
    provider: provider.id,
    profile,
    cookie_count: cookies.length,
    cookie_names: cookies.map(cookie => cookie.name),
    cookie_domains: cookies.map(cookie => cookie.domain),
    store_id: storeId,
    recovery_context: context.recoveryMode
  });
  return {cookies, storeId};
}

function basic(username, password) {
  const bytes = new TextEncoder().encode(`${username}:${password}`);
  let raw = "";
  for (const byte of bytes) raw += String.fromCharCode(byte);
  return `Basic ${btoa(raw)}`;
}

async function statusMap() {
  return (await chrome.storage.local.get("syncStatus")).syncStatus || {};
}

async function record(provider, profile, patch) {
  const current = await statusMap();
  const key = scopeKey(provider, profile);
  current[key] = {...(current[key] || {}), provider, profile, ...patch};
  await chrome.storage.local.set({syncStatus: current});
}

async function createNotification(provider, profile, revision, event, title, message) {
  const id = `media-cookie-broker:${provider}:${profile}:${revision || 0}:${event}`;
  const stored = await chrome.storage.local.get("notificationContexts");
  const contexts = stored.notificationContexts || {};
  contexts[id] = {provider, profile, revision, event, createdAt: new Date().toISOString()};
  for (const staleID of Object.keys(contexts).slice(0, Math.max(0, Object.keys(contexts).length - 200))) delete contexts[staleID];
  await chrome.storage.local.set({notificationContexts: contexts});
  try {
    await chrome.notifications.create(id, {type: "basic", iconUrl: "icon128.png", title, message});
    await logDiagnostic("notification_shown", {provider, profile, revision, event});
  } catch (error) {
    await logDiagnostic("notification_failed", {provider, profile, revision, event, error: String(error?.message || error)}, "warning");
  }
}

function scopeResourceURL(config, provider, profile, resource) {
  const base = parseBrokerEndpoint(config.endpoint);
  if (profile === "default") return `${base}/v1/providers/${encodeURIComponent(provider)}/${resource}`;
  return `${base}/v1/providers/${encodeURIComponent(provider)}/profiles/${encodeURIComponent(profile)}/${resource}`;
}

async function requestUpload(config, provider, profile, data, publicationReason) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20000);
  try {
    const target = scopeResourceURL(config, provider.id, profile, "cookies");
    const started = performance.now();
    const parsedTarget = new URL(target);
    await logDiagnostic("cookie_upload_request", {
      provider: provider.id,
      profile,
      endpoint: parsedTarget.host + parsedTarget.pathname,
      cookie_count: data.cookies.length
    });
    const response = await fetch(target, {
      method: "PUT",
      headers: {Authorization: basic(config.username, config.password), "Content-Type": "application/json"},
      body: JSON.stringify({schema_version: 1, publication_reason: publicationReason, captured_at: new Date().toISOString(), cookies: data.cookies}),
      signal: controller.signal
    });
    await logDiagnostic("cookie_upload_response", {
      provider: provider.id,
      profile,
      http_status: response.status,
      duration_ms: Math.round(performance.now() - started)
    });
    return response;
  } finally {
    clearTimeout(timeout);
  }
}

async function upload(providerID, profile, {publicationReason = "ordinary", recoveryContext = null} = {}) {
  const provider = getProvider(providerID);
  if (!provider || !validProfileID(profile)) return false;
  const config = await settings();
  const mode = recoveryContext?.recoveryMode || expectedCaptureMode(provider, config);
  await logDiagnostic("sync_attempt", {provider: provider.id, profile, recovery_context: mode, publication_reason: publicationReason});
  if (!config.enabledProviders.includes(provider.id)) {
    await logDiagnostic("sync_skipped", {provider: provider.id, profile, reason: "provider disabled"});
    return false;
  }
  const attemptedAt = new Date().toISOString();
  const configProblem = configurationError(config);
  if (configProblem) {
    await record(provider.id, profile, {ok: false, error: configProblem, at: attemptedAt});
    return false;
  }
  let context;
  try {
    context = await captureContext(provider, profile, config, recoveryContext);
  } catch (error) {
    await logDiagnostic("sync_skipped", {provider: provider.id, profile, recovery_context: mode, reason: String(error?.message || error)}, "warning");
    return false;
  }
  if (!context) {
    await logDiagnostic("sync_skipped", {provider: provider.id, profile, recovery_context: mode, reason: "requires explicit isolated recovery"}, "warning");
    return false;
  }
  const data = await collect(provider, profile, context);
  if (!data.cookies.length) {
    const error = "No unexpired matching cookies found";
    await record(provider.id, profile, {ok: false, error, at: attemptedAt, cookie_count: 0, incognito: context.recoveryMode === "incognito"});
    await createNotification(provider.id, profile, 0, "no-cookies", `${provider.label} / ${profile}: no cookies found`, `Sign in to ${provider.label}, then try again.`);
    return false;
  }
  if (provider.validateCapture) {
    const validation = provider.validateCapture({cookies: data.cookies, recoveryContext: context.recoveryMode});
    if (!validation.ok) {
      await record(provider.id, profile, {ok: false, error: validation.error, at: attemptedAt, cookie_count: data.cookies.length, incognito: context.recoveryMode === "incognito"});
      await createNotification(provider.id, profile, 0, "capture-policy", `${provider.label} / ${profile}: sign-in required`, validation.notification);
      return false;
    }
  }
  let lastError = "upload failed";
  for (let attempt = 0; attempt < RETRIES; attempt++) {
    try {
      const response = await requestUpload(config, provider, profile, data, publicationReason);
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.detail || `HTTP ${response.status}`);
      await record(provider.id, profile, {
        ok: true,
        error: "",
        at: attemptedAt,
        uploaded_at: new Date().toISOString(),
        revision: payload.revision,
        cookie_count: payload.cookie_count,
        sha256: payload.sha256,
        incognito: context.recoveryMode === "incognito",
        store_id: data.storeId
      });
      await logDiagnostic("sync_succeeded", {provider: provider.id, profile, revision: payload.revision, cookie_count: payload.cookie_count, publication_reason: publicationReason});
      await refreshStatusAfterPublication(
        () => brokerStatusPoller.runAfterCurrent(),
        error => logDiagnostic("post_upload_status_refresh_failed", {
          provider: provider.id,
          profile,
          revision: payload.revision,
          error: String(error?.message || error)
        }, "warning")
      );
      return true;
    } catch (error) {
      lastError = String(error?.message || error);
      await logDiagnostic("cookie_upload_failed", {provider: provider.id, profile, attempt: attempt + 1, error: lastError}, "error");
      if (attempt + 1 < RETRIES) await new Promise(resolve => setTimeout(resolve, RETRY_DELAYS_MS[attempt]));
    }
  }
  await record(provider.id, profile, {ok: false, error: lastError, at: attemptedAt, cookie_count: data.cookies.length, incognito: context.recoveryMode === "incognito"});
  await createNotification(provider.id, profile, 0, "upload-failed", `${provider.label} / ${profile}: upload failed`, `${lastError}. Check the broker connection, then try again.`);
  await logDiagnostic("sync_failed", {provider: provider.id, profile, error: lastError}, "error");
  return false;
}

async function recoveryContexts() {
  return (await chrome.storage.local.get(RECOVERY_CONTEXTS_KEY))[RECOVERY_CONTEXTS_KEY] || {};
}

const recoveryCoordinator = createRecoveryCoordinator({
  browser: chrome,
  logDiagnostic,
  loadContexts: recoveryContexts,
  saveContexts: contexts => chrome.storage.local.set({[RECOVERY_CONTEXTS_KEY]: contexts})
});

async function startProviderRecovery(providerID, profile) {
  const provider = getProvider(providerID);
  if (!provider?.setup || !validProfileID(profile)) return {ok: false, error: "This provider/profile does not have a recovery workflow."};
  const config = await settings();
  if (configurationError(config)) return {ok: false, error: "Configure the broker connection first."};
  const useIsolated = provider.recovery?.isolationConfigurable ? Boolean(config.youtubeUseIncognito) : Boolean(provider.recovery?.defaultIsolated);
  await logDiagnostic("provider_profile_refresh_requested", {provider: provider.id, profile, recovery_context: useIsolated ? "incognito" : "normal"});
  return recoveryCoordinator.start(provider, profile, useIsolated);
}

async function pollBrokerStatusOnce() {
  const config = await settings();
  const problem = configurationError(config);
  if (problem) {
    await chrome.storage.local.set({brokerStatus: {connected: false, profiles: [], error: problem, lastPolledAt: new Date().toISOString()}});
    return false;
  }
  const started = performance.now();
  await logDiagnostic("broker_status_poll_started");
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20000);
  try {
    const response = await fetch(`${parseBrokerEndpoint(config.endpoint)}/v1/status`, {
      headers: {Authorization: basic(config.username, config.password)},
      signal: controller.signal
    });
    await logDiagnostic("broker_status_poll_response", {http_status: response.status, duration_ms: Math.round(performance.now() - started)});
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const payload = await response.json();
    if (!Array.isArray(payload.profiles)) throw new Error("Invalid broker status response");
    const stored = await chrome.storage.local.get(["controlPlaneState", "brokerStatus"]);
    const previousStates = stored.controlPlaneState || {};
    const nextStates = {};
    for (const status of payload.profiles) {
      if (!getProvider(status.provider) || !validProfileID(status.profile)) continue;
      const key = scopeKey(status.provider, status.profile);
      const previous = previousStates[key] || {};
      const evaluated = evaluateBrokerStatus(status, previous, config.expiryThresholdHours);
      nextStates[key] = evaluated.state;
      if (previous.health && previous.health !== evaluated.state.health) {
        await logDiagnostic("provider_profile_health_transition", {provider: status.provider, profile: status.profile, revision: status.revision, previous_health_state: previous.health, health_state: evaluated.state.health});
      }
      if (status.auth_required_count || status.last_report_at) {
        await logDiagnostic("consumer_health_summary_observed", {provider: status.provider, profile: status.profile, revision: status.revision, health_state: status.auth_health, report_count: status.auth_required_count || 0});
      }
      const provider = getProvider(status.provider);
      for (const notice of evaluated.notifications) {
        if (notice.event === "refresh_required") {
          await createNotification(status.provider, status.profile, status.revision, notice.event, `${provider.label} / ${status.profile} authentication needs attention`, "A consumer reported that the current session requires authentication.");
        } else if (notice.event === "expiry_hint_passed") {
          await logDiagnostic("expiry_hint_passed", {provider: status.provider, profile: status.profile, revision: status.revision, auth_expires_at: status.auth_expires_at});
          await createNotification(status.provider, status.profile, status.revision, notice.event, `${provider.label} / ${status.profile} auth-expiry hint passed`, "This timing hint has passed; it does not prove the session is invalid. Refresh when convenient.");
        } else {
          await logDiagnostic("expiry_threshold_crossed", {provider: status.provider, profile: status.profile, revision: status.revision, expiry_threshold_hours: notice.thresholdHours, auth_expires_at: status.auth_expires_at});
          await createNotification(status.provider, status.profile, status.revision, `expiry-${notice.thresholdHours}`, `${provider.label} / ${status.profile} session may need refresh within ~${notice.thresholdHours} hours`, "Refresh while you are at your browser to reduce the chance of later downtime.");
        }
      }
    }
    const brokerStatus = {connected: true, profiles: payload.profiles, error: "", lastPolledAt: new Date().toISOString()};
    await chrome.storage.local.set({brokerStatus, controlPlaneState: nextStates});
    return true;
  } catch (error) {
    const previous = (await chrome.storage.local.get("brokerStatus")).brokerStatus || {profiles: []};
    await chrome.storage.local.set({brokerStatus: {...previous, connected: false, error: String(error?.message || error), lastPolledAt: new Date().toISOString()}});
    await logDiagnostic("broker_status_poll_failed", {duration_ms: Math.round(performance.now() - started), error: String(error?.message || error)}, "warning");
    return false;
  } finally {
    clearTimeout(timeout);
  }
}

const brokerStatusPoller = createPollCoordinator(pollBrokerStatusOnce);

function pollBrokerStatus() {
  return brokerStatusPoller.run();
}

async function uploadAll() {
  const config = await settings();
  for (const providerID of config.enabledProviders) await upload(providerID, selectedProfile(config, providerID));
}

async function runPeriodicCycle() {
  if (cycleInFlight) return cycleInFlight;
  cycleInFlight = (async () => {
    await pollBrokerStatus();
    const config = await settings();
    if (!configurationError(config)) await uploadDiagnostics(config, basic);
  })().finally(() => { cycleInFlight = null; });
  return cycleInFlight;
}

async function ensureAlarm() {
  const config = await settings();
  const configured = Number(config.intervalMinutes) || DEFAULTS.intervalMinutes;
  const interval = Math.min(MAX_INTERVAL_MINUTES, Math.max(MIN_INTERVAL_MINUTES, configured));
  await chrome.alarms.create(PERIODIC_ALARM, {periodInMinutes: interval});
}

chrome.runtime.onInstalled.addListener(() => {
  installationId().then(() => logDiagnostic("service_worker_installed"));
  ensureAlarm();
  runPeriodicCycle();
});
chrome.runtime.onStartup.addListener(() => {
  logDiagnostic("service_worker_started");
  ensureAlarm();
  runPeriodicCycle();
});
chrome.alarms.onAlarm.addListener(alarm => {
  if (alarm.name === PERIODIC_ALARM) {
    runPeriodicCycle();
    return;
  }
  if (alarm.name.startsWith(CHANGE_ALARM_PREFIX)) {
    const [provider, profile = "default"] = alarm.name.slice(CHANGE_ALARM_PREFIX.length).split("/");
    upload(provider, profile);
  }
});
chrome.cookies.onChanged.addListener(({cookie}) => {
  const provider = providerForCookieDomain(cookie.domain);
  if (!provider || provider.recovery) return;
  settings().then(config => {
    if (expectedCaptureMode(provider, config) !== "normal") return;
    const profile = selectedProfile(config, provider.id);
    logDiagnostic("cookie_changed", {provider: provider.id, profile, name: cookie.name, domain: cookie.domain, expiration: cookie.expirationDate, store_id: cookie.storeId});
    chrome.alarms.create(`${CHANGE_ALARM_PREFIX}${scopeKey(provider.id, profile)}`, {delayInMinutes: 0.5});
  }).catch(error => logDiagnostic("cookie_change_setup_failed", {provider: provider.id, error: String(error?.message || error)}, "error"));
});
chrome.tabs.onUpdated.addListener((_tabID, changeInfo, tab) => {
  if (changeInfo.status !== "complete") return;
  recoveryContexts().then(async contexts => {
    const context = contexts[String(tab.windowId)];
    const provider = getProvider(context?.provider);
    if (!provider?.matchesSetupNavigation?.(tab.url) || !recoveryMatchesTab(context, tab)) return;
    const ok = await upload(provider.id, context.profile, {publicationReason: "recovery", recoveryContext: context});
    if (!ok) {
      await logDiagnostic("recovery_context_failed", {provider: provider.id, profile: context.profile, recovery_context: context.recoveryMode, error: "capture_or_upload_failed"}, "error");
      return;
    }
    await logDiagnostic("recovery_context_completed", {provider: provider.id, profile: context.profile, recovery_context: context.recoveryMode});
    await recoveryCoordinator.complete(contexts, context, provider);
  }).catch(error => logDiagnostic("recovery_context_failed", {error: String(error?.message || error)}, "error"));
});
chrome.windows.onRemoved.addListener(windowID => {
  recoveryContexts().then(async contexts => {
    const context = contexts[String(windowID)];
    if (!context) return;
    delete contexts[String(windowID)];
    await chrome.storage.local.set({[RECOVERY_CONTEXTS_KEY]: contexts});
    await logDiagnostic("recovery_context_failed", {provider: context.provider, profile: context.profile, recovery_context: context.recoveryMode, error: "window_closed_before_completion"}, "warning");
  });
});
chrome.notifications.onClicked.addListener(notificationID => {
  chrome.storage.local.get("notificationContexts").then(async stored => {
    const context = stored.notificationContexts?.[notificationID];
    if (!context) return;
    await chrome.storage.local.set({controlPlaneFocus: context});
    await logDiagnostic("notification_clicked", context);
    await chrome.tabs.create({url: chrome.runtime.getURL("popup.html")});
    await chrome.notifications.clear(notificationID);
  });
});
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === "sync-now") {
    uploadAll().then(() => sendResponse({ok: true})).catch(error => sendResponse({ok: false, error: String(error)}));
    return true;
  }
  if (message?.type === "settings-changed") {
    logDiagnostic("settings_changed", {enabled_providers: message.enabledProviders || [], remote_diagnostics_enabled: message.remoteDiagnosticsEnabled});
    ensureAlarm().then(runPeriodicCycle).then(() => sendResponse({ok: true})).catch(error => sendResponse({ok: false, error: String(error)}));
    return true;
  }
  if (message?.type === "start-provider-recovery") {
    startProviderRecovery(message.provider, message.profile || "default").then(sendResponse);
    return true;
  }
  if (message?.type === "poll-now") {
    pollBrokerStatus().then(ok => sendResponse({ok})).catch(error => sendResponse({ok: false, error: String(error)}));
    return true;
  }
});
ensureAlarm();
