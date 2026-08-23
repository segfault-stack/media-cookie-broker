import {configurationError, DEFAULTS} from "./config.js";
import {expiryHintText, profileVisualState, scopeKey} from "./control-plane.js";
import {logDiagnostic} from "./diagnostics.js";
import {runRecoveryAction} from "./popup-ui.js";
import {getProvider} from "./providers/registry.js";

const statusElement = document.querySelector("#status");
const syncButton = document.querySelector("#sync");
const pollButton = document.querySelector("#poll");
const optionsButton = document.querySelector("#options");
const mode = document.querySelector("#mode");
const connectivity = document.querySelector("#broker-connectivity");
const setupRequired = document.querySelector("#setup-required");
const actionResult = document.querySelector("#action-result");

function time(value) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

async function render() {
  const config = {...DEFAULTS, ...await chrome.storage.local.get(Object.keys(DEFAULTS))};
  config.selectedProfiles = {...DEFAULTS.selectedProfiles, ...(config.selectedProfiles || {})};
  const ready = !configurationError(config);
  setupRequired.hidden = ready;
  syncButton.disabled = !ready;
  pollButton.disabled = !ready;
  mode.textContent = `Spanning control plane · YouTube recovery isolation ${config.youtubeUseIncognito ? "enabled" : "disabled"}`;
  mode.className = "context warning";

  const stored = await chrome.storage.local.get(["syncStatus", "brokerStatus", "controlPlaneFocus"]);
  const local = stored.syncStatus || {};
  const broker = stored.brokerStatus || {connected: false, profiles: []};
  connectivity.textContent = broker.connected
    ? `Broker connected · checked ${time(broker.lastPolledAt)}`
    : `Broker status unavailable${broker.error ? ` · ${broker.error}` : ""}`;
  connectivity.className = `context ${broker.connected ? "good" : "warning"}`;
  const profiles = broker.profiles?.length ? broker.profiles : config.enabledProviders.map(provider => ({
    provider,
    profile: config.selectedProfiles[provider] || "default",
    auth_health: "missing",
    revision: 0
  }));
  const focusKey = stored.controlPlaneFocus ? scopeKey(stored.controlPlaneFocus.provider, stored.controlPlaneFocus.profile) : "";
  profiles.sort((a, b) => Number(scopeKey(b.provider, b.profile) === focusKey) - Number(scopeKey(a.provider, a.profile) === focusKey));
  const rows = profiles.map(status => {
    const provider = getProvider(status.provider);
    if (!provider) return null;
    const key = scopeKey(status.provider, status.profile);
    const capture = local[key] || {};
    const row = document.createElement("article");
    row.className = `provider ${profileVisualState(status, capture)}${key === focusKey ? " focused" : ""}`;
    const title = document.createElement("strong");
    title.textContent = `${provider.label} / ${status.profile}`;
    const detail = document.createElement("span");
    if (status.auth_health === "refresh_required") {
      detail.textContent = `Authentication requires attention · revision ${status.revision} · ${status.auth_required_count || 0} consumer report(s)`;
    } else if (status.auth_health === "healthy") {
      detail.textContent = `Healthy · revision ${status.revision}${expiryHintText(status.auth_expires_at)}`;
    } else {
      detail.textContent = "No snapshot published for this profile";
    }
    const stamp = document.createElement("small");
    const parts = [];
    if (status.last_report_at) parts.push(`Last report: ${time(status.last_report_at)}`);
    if (capture.ok) parts.push(`Last local publish: ${time(capture.uploaded_at || capture.at)}`);
    else if (capture.error) parts.push(`Local publisher: ${capture.error}`);
    stamp.textContent = parts.join(" · ") || "No local capture activity recorded";
    row.append(title, detail, stamp);
    if (provider.setup) {
      const button = document.createElement("button");
      button.className = "primary";
      button.textContent = "Refresh session";
      button.disabled = !ready || !config.enabledProviders.includes(provider.id);
      button.onclick = async () => {
        await runRecoveryAction({
          button,
          resultElement: actionResult,
          requestRecovery: async () => {
            await logDiagnostic("profile_selected", {provider: provider.id, profile: status.profile});
            const selectedProfiles = {...config.selectedProfiles, [provider.id]: status.profile};
            await chrome.storage.local.set({selectedProfiles});
            return chrome.runtime.sendMessage({type: "start-provider-recovery", provider: provider.id, profile: status.profile});
          }
        });
      };
      row.append(button);
    }
    return row;
  }).filter(Boolean);
  statusElement.replaceChildren(...rows);
}

syncButton.onclick = async () => {
  syncButton.disabled = true;
  syncButton.textContent = "Synchronizing…";
  await chrome.runtime.sendMessage({type: "sync-now"});
  syncButton.textContent = "↻ Sync ordinary browser providers";
  await render();
};
pollButton.onclick = async () => {
  pollButton.disabled = true;
  pollButton.textContent = "Checking…";
  await chrome.runtime.sendMessage({type: "poll-now"});
  pollButton.textContent = "Check broker status";
  await render();
};
optionsButton.onclick = () => chrome.runtime.openOptionsPage();

render();
