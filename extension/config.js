import {listProviders} from "./providers/registry.js";
import {DEFAULT_EXPIRY_THRESHOLDS} from "./control-plane.js";

export const DEFAULTS = {
  endpoint: "",
  username: "browser-publisher",
  password: "",
  intervalMinutes: 5,
  enabledProviders: listProviders().filter(provider => provider.enabledByDefault).map(provider => provider.id),
  selectedProfiles: Object.fromEntries(listProviders().map(provider => [provider.id, "default"])),
  expiryThresholdHours: DEFAULT_EXPIRY_THRESHOLDS,
  remoteDiagnosticsEnabled: false,
  youtubeUseIncognito: true
};

export function parseBrokerEndpoint(raw) {
  let url;
  try {
    url = new URL(String(raw || "").trim());
  } catch (_) {
    throw new Error("Broker URL is required and must be an absolute URL.");
  }
  const hostname = url.hostname.toLowerCase();
  const loopback = hostname === "localhost" || hostname === "127.0.0.1" || hostname === "[::1]" || hostname === "::1";
  const safeScheme = url.protocol === "https:" || (url.protocol === "http:" && loopback);
  if (!safeScheme || url.username || url.password || url.search || url.hash) {
    throw new Error("Use HTTPS, or HTTP on loopback, without credentials, query, or fragment.");
  }
  return url.href.replace(/\/$/, "");
}

export function brokerPermissionOrigin(raw) {
  const url = new URL(parseBrokerEndpoint(raw));
  return `${url.protocol}//${url.host}/*`;
}

export function configurationError(config) {
  try {
    parseBrokerEndpoint(config.endpoint);
  } catch (error) {
    return error.message;
  }
  if (!String(config.username || "").trim()) return "Publisher username is required.";
  if (!config.password) return "Publisher password is required.";
  return "";
}
