import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import test from "node:test";

import {brokerPermissionOrigin, DEFAULTS, parseBrokerEndpoint} from "../config.js";
import {getProvider, listProviders, providerForCookieDomain} from "../providers/registry.js";

test("provider registry owns labels, cookie scopes, and defaults", () => {
  assert.deepEqual(listProviders().map(provider => provider.id), ["youtube", "tiktok", "instagram", "x"]);
  assert.deepEqual(getProvider("youtube").cookieDomains, ["youtube.com"]);
  assert.equal(providerForCookieDomain(".accounts.google.com"), undefined);
  assert.equal(providerForCookieDomain("music.youtube.com").id, "youtube");
  assert.equal(DEFAULTS.endpoint, "");
  assert.deepEqual(DEFAULTS.enabledProviders, ["youtube"]);
  assert.equal(DEFAULTS.selectedProfiles.youtube, "default");
  assert.equal(DEFAULTS.youtubeUseIncognito, true);
  assert.equal(DEFAULTS.remoteDiagnosticsEnabled, false);
});

test("declared provider domains match manifest cookie permissions", async () => {
  const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));
	assert.equal(manifest.incognito, "spanning");
  const expected = listProviders().flatMap(provider => provider.cookieDomains.map(domain => `https://*.${domain}/*`)).sort();
  assert.deepEqual([...manifest.host_permissions].sort(), expected);
  assert(!manifest.host_permissions.some(permission => permission.includes("google.com") || permission.includes("youtu.be")));
});

test("spanning recovery never selects a target from ambient extension context", async () => {
  const sources = await Promise.all([
    readFile(new URL("../service-worker.js", import.meta.url), "utf8"),
    readFile(new URL("../recovery.js", import.meta.url), "utf8")
  ]);
  assert.doesNotMatch(sources.join("\n"), /inIncognitoContext/);
  assert.match(sources[0], /resolveRecoveryCaptureContext/);
  assert.match(sources[0], /publicationReason: "recovery"/);
});

test("extension never infers browser mode from nonexistent CookieStore metadata", async () => {
  const sources = await Promise.all([
    readFile(new URL("../service-worker.js", import.meta.url), "utf8"),
    readFile(new URL("../recovery.js", import.meta.url), "utf8")
  ]);
  const forbiddenProperty = ["store", "incognito"].join(".");
  const forbiddenIteratorProperty = ["item", "incognito"].join(".");
  assert(!sources.join("\n").includes(forbiddenProperty));
  assert(!sources.join("\n").includes(forbiddenIteratorProperty));
});

test("server and extension provider registries stay aligned", async () => {
  const source = await readFile(new URL("../../internal/providers/registry.go", import.meta.url), "utf8");
  const serverProviders = new Map();
  const entries = source.matchAll(/"([a-z0-9_-]+)":\s*{\s*ID:\s*"([a-z0-9_-]+)",\s*AllowedDomains:\s*\[\]string{([^}]*)}/g);
  for (const entry of entries) {
    assert.equal(entry[1], entry[2], "Go registry key and ID differ");
    serverProviders.set(entry[1], [...entry[3].matchAll(/"([^"]+)"/g)].map(match => match[1]));
  }
  const extensionProviders = new Map(listProviders().map(provider => [provider.id, provider.cookieDomains]));
  assert.deepEqual(serverProviders, extensionProviders);
  const youtubeAuth = source.match(/"youtube":\s*\{[\s\S]*?AuthCookieNames:\s*\[\]string\{([^}]*)}/);
  assert(youtubeAuth, "server YouTube auth-cookie policy is missing");
  assert.deepEqual([...youtubeAuth[1].matchAll(/"([^"]+)"/g)].map(match => match[1]), getProvider("youtube").authCookieNames);
});

test("broker endpoint policy requires HTTPS except on loopback", () => {
  assert.equal(parseBrokerEndpoint("https://broker.example.com/"), "https://broker.example.com");
  assert.equal(parseBrokerEndpoint("http://127.0.0.1:8787/"), "http://127.0.0.1:8787");
  assert.equal(parseBrokerEndpoint("http://[::1]:8787/"), "http://[::1]:8787");
  assert.equal(brokerPermissionOrigin("https://broker.example.com/base"), "https://broker.example.com/*");
  for (const value of ["", "http://broker.example.com", "https://user:password@broker.example.com", "https://broker.example.com?token=fake"]) {
    assert.throws(() => parseBrokerEndpoint(value));
  }
});
