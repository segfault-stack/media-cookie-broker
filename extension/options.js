import {brokerPermissionOrigin, DEFAULTS, parseBrokerEndpoint} from "./config.js";
import {normalizeExpiryThresholds, validProfileID} from "./control-plane.js";
import {clearDiagnostics, diagnosticsState, recentDiagnostics} from "./diagnostics.js";
import {listProviders} from "./providers/registry.js";

const endpointInput = document.querySelector("#endpoint");
const usernameInput = document.querySelector("#username");
const passwordInput = document.querySelector("#password");
const intervalInput = document.querySelector("#interval");
const expiryThresholdsInput = document.querySelector("#expiry-thresholds");
const youtubeIncognitoInput = document.querySelector("#youtube-incognito");
const remoteDiagnosticsInput = document.querySelector("#remote-diagnostics");
const providersElement = document.querySelector("#providers");
const saveButton = document.querySelector("#save");
const result = document.querySelector("#result");
const context = document.querySelector("#context");
const diagnosticEvents = document.querySelector("#diagnostic-events");
const diagnosticStateElement = document.querySelector("#diagnostic-state");
const diagnosticFilter = document.querySelector("#diagnostic-filter");
let events = [];

async function renderDiagnostics() {
  events = await recentDiagnostics();
  const state = await diagnosticsState();
  diagnosticStateElement.textContent = `${state.queuedEvents} events pending remote upload in ${state.queuedBatches} batches${state.lastUploadedAt ? ` · last upload ${state.lastUploadedAt}` : ""}${state.lastError ? ` · last error: ${state.lastError}` : ""}`;
  const query = diagnosticFilter.value.trim().toLowerCase();
  diagnosticEvents.textContent = JSON.stringify(events.filter(event => !query || JSON.stringify(event).toLowerCase().includes(query)), null, 2);
}

function showContext() {
  context.textContent = `Spanning control plane: YouTube recovery will ${youtubeIncognitoInput.checked
    ? "require incognito access and no other open incognito window"
    : "use your current normal browser cookie session; the opened window will remain open"}.`;
  context.className = "context warning";
}

function renderProviderControls(enabledProviders, selectedProfiles) {
  providersElement.replaceChildren(...listProviders().map(provider => {
    const row = document.createElement("div");
    row.className = "check";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.className = "provider-enabled";
    input.value = provider.id;
    input.checked = enabledProviders.includes(provider.id);
    const name = document.createElement("span");
    name.textContent = provider.label;
    const help = document.createElement("small");
    help.textContent = provider.recovery ? "interactive provider recovery" : "experimental regular capture";
    const profile = document.createElement("input");
    profile.className = "provider-profile";
    profile.dataset.provider = provider.id;
    profile.value = selectedProfiles[provider.id] || "default";
    profile.placeholder = "default profile ID";
    profile.title = `${provider.label} active capture profile`;
    row.append(input, name, help, profile);
    return row;
  }));
}

async function load() {
  const config = await chrome.storage.local.get(DEFAULTS);
  endpointInput.value = config.endpoint;
  usernameInput.value = config.username;
  passwordInput.value = config.password;
  intervalInput.value = config.intervalMinutes;
  expiryThresholdsInput.value = normalizeExpiryThresholds(config.expiryThresholdHours).join(", ");
  youtubeIncognitoInput.checked = config.youtubeUseIncognito;
  remoteDiagnosticsInput.checked = config.remoteDiagnosticsEnabled;
  renderProviderControls(config.enabledProviders, {...DEFAULTS.selectedProfiles, ...(config.selectedProfiles || {})});
  showContext();
  await renderDiagnostics();
}

saveButton.onclick = async () => {
  result.className = "result";
  if (!usernameInput.value.trim()) {
    result.textContent = "Publisher username is required.";
    usernameInput.focus();
    return;
  }
  if (!passwordInput.value) {
    result.textContent = "Publisher password is required.";
    passwordInput.focus();
    return;
  }
  let endpoint;
  let permissionOrigin;
  try {
    endpoint = parseBrokerEndpoint(endpointInput.value);
    permissionOrigin = brokerPermissionOrigin(endpoint);
  } catch (error) {
    result.textContent = error.message;
    endpointInput.focus();
    return;
  }
  const enabledProviders = [...providersElement.querySelectorAll(".provider-enabled:checked")].map(element => element.value);
  if (!enabledProviders.length) {
    result.textContent = "Enable at least one provider.";
    return;
  }
  const selectedProfiles = Object.fromEntries([...providersElement.querySelectorAll(".provider-profile")].map(element => [element.dataset.provider, element.value.trim() || "default"]));
  const invalidProfile = enabledProviders.find(provider => !validProfileID(selectedProfiles[provider]));
  if (invalidProfile) {
    result.textContent = `Invalid profile ID for ${invalidProfile}. Use lowercase letters, digits, dot, underscore, or hyphen.`;
    return;
  }
  const expiryThresholdHours = normalizeExpiryThresholds(expiryThresholdsInput.value);
  if (expiryThresholdsInput.value.trim() && !expiryThresholdHours.length) {
    result.textContent = "Expiry thresholds must be positive hours (maximum 8760).";
    return;
  }
  const granted = await chrome.permissions.request({origins: [permissionOrigin]});
  if (!granted) {
    result.textContent = "Broker origin access was not granted; settings were not saved.";
    return;
  }
  const previous = await chrome.storage.local.get("brokerPermissionOrigin");
  await chrome.storage.local.set({
    endpoint,
    username: usernameInput.value.trim(),
    password: passwordInput.value,
    intervalMinutes: Math.min(5, Math.max(2, Number(intervalInput.value) || 5)),
    enabledProviders,
    selectedProfiles,
    expiryThresholdHours,
    youtubeUseIncognito: youtubeIncognitoInput.checked,
    remoteDiagnosticsEnabled: remoteDiagnosticsInput.checked,
    brokerPermissionOrigin: permissionOrigin
  });
  if (previous.brokerPermissionOrigin && previous.brokerPermissionOrigin !== permissionOrigin) {
    await chrome.permissions.remove({origins: [previous.brokerPermissionOrigin]});
  }
  await chrome.runtime.sendMessage({type: "settings-changed", enabledProviders, remoteDiagnosticsEnabled: remoteDiagnosticsInput.checked});
  result.textContent = "Saved. Broker status check has started; open the popup for details.";
  result.className = "result success";
  showContext();
};

diagnosticFilter.addEventListener("input", renderDiagnostics);
youtubeIncognitoInput.addEventListener("change", showContext);
document.querySelector("#diagnostic-copy").onclick = async () => navigator.clipboard.writeText(diagnosticEvents.textContent);
document.querySelector("#diagnostic-download").onclick = () => {
  const blob = new Blob([diagnosticEvents.textContent], {type: "application/json"});
  const url = URL.createObjectURL(blob);
  const link = Object.assign(document.createElement("a"), {href: url, download: "media-cookie-broker-diagnostics.json"});
  link.click();
  URL.revokeObjectURL(url);
};
document.querySelector("#diagnostic-clear").onclick = async () => {
  await clearDiagnostics();
  await renderDiagnostics();
};

load();
