import assert from "node:assert/strict";
import test from "node:test";

import {
  createRecoveryCoordinator,
  recoveryMatchesTab,
  resolveCookieStoreForTab,
  resolveNormalCookieStore,
  resolveRecoveryCaptureContext
} from "../recovery.js";
import {youtubeProvider} from "../providers/youtube.js";

function harness({allowed = true, windows = [], contexts = {}, cookieStoreSnapshots = null} = {}) {
  const currentWindows = structuredClone(windows);
  const created = [];
  const removed = [];
  const events = [];
  let cookieStoreCalls = 0;
  let saved = structuredClone(contexts);
  const browser = {
    extension: {async isAllowedIncognitoAccess() { return allowed; }},
    windows: {
      async getAll() { return structuredClone(currentWindows); },
      async get(windowID) {
        const window = currentWindows.find(item => item.id === windowID);
        if (!window) throw new Error("window not found");
        return structuredClone(window);
      },
      async create(options) {
        created.push(options);
        const window = {
          id: 91,
          incognito: options.incognito,
          tabs: [{id: 191, windowId: 91, incognito: options.incognito, url: options.url}]
        };
        currentWindows.push(window);
        return structuredClone(window);
      },
      async remove(windowID) {
        removed.push(windowID);
        const index = currentWindows.findIndex(item => item.id === windowID);
        if (index >= 0) currentWindows.splice(index, 1);
      }
    },
    tabs: {
      async query() {
        return structuredClone(currentWindows.flatMap(window => window.tabs || []));
      }
    },
    cookies: {
      async getAllCookieStores() {
        const stores = [
          {id: "normal-store", tabIds: currentWindows.filter(window => !window.incognito).flatMap(window => window.tabs?.map(tab => tab.id) || [])},
          {id: "incognito-store", tabIds: currentWindows.filter(window => window.incognito).flatMap(window => window.tabs?.map(tab => tab.id) || [])}
        ];
        const snapshot = cookieStoreSnapshots?.[Math.min(cookieStoreCalls, cookieStoreSnapshots.length - 1)];
        cookieStoreCalls++;
        return structuredClone(snapshot || stores);
      }
    }
  };
  const coordinator = createRecoveryCoordinator({
    browser,
    logDiagnostic: async (type, details, severity = "info") => events.push({type, details, severity}),
    loadContexts: async () => structuredClone(saved),
    saveContexts: async value => { saved = structuredClone(value); }
  });
  return {browser, coordinator, created, removed, events, windows: currentWindows, contexts: () => saved, cookieStoreCalls: () => cookieStoreCalls};
}

test("incognito permission denial refuses recovery before creating a window", async () => {
  const state = harness({allowed: false});
  const result = await state.coordinator.start(youtubeProvider, "default", true);
  assert.equal(result.ok, false);
  assert.match(result.error, /Allow in incognito/);
  assert.deepEqual(state.created, []);
  assert.equal(state.events.at(-1).details.reason, "incognito_access_disabled");
});

test("an existing incognito session refuses isolated recovery without closing it", async () => {
  const state = harness({windows: [{id: 44, incognito: true, tabs: [{id: 144, windowId: 44, incognito: true}]}]});
  const result = await state.coordinator.start(youtubeProvider, "default", true);
  assert.equal(result.ok, false);
  assert.match(result.error, /Close all existing incognito windows/);
  assert.deepEqual(state.created, []);
  assert.deepEqual(state.removed, []);
});

test("an active or concurrently starting isolated recovery refuses a second flow", async () => {
  const active = harness({
    windows: [{id: 44, incognito: true, tabs: [{id: 144, windowId: 44, incognito: true}]}],
    contexts: {44: {provider: "youtube", profile: "default", recoveryMode: "incognito", windowId: 44, tabId: 144, brokerOwned: true}}
  });
  assert.match((await active.coordinator.start(youtubeProvider, "music-bot", true)).error, /already in progress/);
  assert.deepEqual(active.created, []);

  const concurrent = harness();
  let release;
  concurrent.browser.windows.create = options => new Promise(resolve => {
    concurrent.created.push(options);
    release = () => {
      const window = {id: 91, incognito: true, tabs: [{id: 191, windowId: 91, incognito: true, url: options.url}]};
      concurrent.windows.push(window);
      resolve(structuredClone(window));
    };
  });
  const first = concurrent.coordinator.start(youtubeProvider, "default", true);
  await Promise.resolve();
  const second = await concurrent.coordinator.start(youtubeProvider, "music-bot", true);
  assert.equal(second.ok, false);
  assert.match(second.error, /already in progress/);
  while (!release) await Promise.resolve();
  release();
  assert.equal((await first).ok, true);
  assert.equal(concurrent.created.length, 1);
});

test("isolated startup records window and tab identity without resolving the cookie store", async () => {
  const state = harness();
  const result = await state.coordinator.start(youtubeProvider, "default", true);
  assert.equal(result.ok, true);
  assert.equal(state.created[0].incognito, true);
  assert.deepEqual(state.contexts()[91], {
    provider: "youtube",
    profile: "default",
    recoveryMode: "incognito",
    windowId: 91,
    tabId: 191,
    openedAt: state.contexts()[91].openedAt,
    brokerOwned: true
  });
  assert.equal(state.cookieStoreCalls(), 0);
  assert(recoveryMatchesTab(state.contexts()[91], state.windows[0].tabs[0]));
  assert(!recoveryMatchesTab(state.contexts()[91], {...state.windows[0].tabs[0], id: 192}));
});

test("startup rejects a recovery tab whose incognito mode conflicts with its window", async () => {
  const state = harness();
  const originalGet = state.browser.windows.get;
  state.browser.windows.get = async windowID => {
    const observed = await originalGet(windowID);
    observed.tabs[0].incognito = false;
    return observed;
  };
  const result = await state.coordinator.start(youtubeProvider, "default", true);
  assert.equal(result.ok, false);
  assert.match(result.error, /requested incognito recovery tab/);
  assert.deepEqual(state.removed, [91]);
  assert.equal(state.cookieStoreCalls(), 0);
});

test("cookie stores are selected from tracked tab membership for isolated and normal recovery", async () => {
  const state = harness({windows: [
    {id: 30, incognito: false, tabs: [{id: 130, windowId: 30, incognito: false}]},
    {id: 40, incognito: true, tabs: [{id: 140, windowId: 40, incognito: true}]}
  ]});
  assert.equal((await resolveCookieStoreForTab(state.browser, 140)).id, "incognito-store");
  assert.equal((await resolveCookieStoreForTab(state.browser, 130)).id, "normal-store");
  assert.equal((await resolveNormalCookieStore(state.browser)).id, "normal-store");
  await assert.rejects(resolveCookieStoreForTab(state.browser, 999, {attempts: 2, delayMs: 0}), /in time/);
});

test("capture tolerates delayed cookie-store association for the tracked recovery tab", async () => {
  const absent = [
    {id: "normal-store", tabIds: []},
    {id: "incognito-store", tabIds: []}
  ];
  const present = [
    {id: "normal-store", tabIds: []},
    {id: "incognito-store", tabIds: [191]}
  ];
  const state = harness({cookieStoreSnapshots: [absent, absent, present]});
  const started = await state.coordinator.start(youtubeProvider, "default", true);
  assert.equal(started.ok, true);
  assert.equal(state.windows[0].tabs[0].incognito, true);
  assert(present[1].tabIds.includes(started.tabId));
  assert.equal(Object.hasOwn(present[1], "incognito"), false);
  assert.equal(state.cookieStoreCalls(), 0, "startup must not inspect cookie stores");

  const capture = await resolveRecoveryCaptureContext(
    state.browser,
    "youtube",
    "default",
    state.contexts()[91],
    {attempts: 3, delayMs: 0}
  );
  assert.equal(capture.cookieStoreId, "incognito-store");
  assert.equal(state.cookieStoreCalls(), 3);
});

test("capture fails after a bounded wait when Chrome never exposes the recovery store", async () => {
  const wrongAssociation = [{id: "incognito-store", tabIds: [999]}];
  const state = harness({
    windows: [{id: 91, incognito: true, tabs: [{id: 191, windowId: 91, incognito: true}]}],
    cookieStoreSnapshots: [wrongAssociation]
  });
  const context = {provider: "youtube", profile: "default", recoveryMode: "incognito", windowId: 91, tabId: 191, brokerOwned: true};
  await assert.rejects(
    resolveRecoveryCaptureContext(state.browser, "youtube", "default", context, {attempts: 3, delayMs: 0}),
    /did not expose the cookie store for the recovery tab in time/
  );
  assert.equal(state.cookieStoreCalls(), 3);
});

test("normal recovery resolves its store from a real normal tab without CookieStore mode metadata", async () => {
  const normalStores = [{id: "normal-store", tabIds: [130]}];
  const state = harness({
    windows: [{id: 30, incognito: false, tabs: [{id: 130, windowId: 30, incognito: false}]}],
    cookieStoreSnapshots: [normalStores]
  });
  const context = {provider: "youtube", profile: "default", recoveryMode: "normal", windowId: 30, tabId: 130, brokerOwned: true};
  assert.equal(state.windows[0].tabs[0].incognito, false);
  assert(normalStores[0].tabIds.includes(context.tabId));
  assert.equal(Object.hasOwn(normalStores[0], "incognito"), false);
  const capture = await resolveRecoveryCaptureContext(state.browser, "youtube", "default", context, {attempts: 1, delayMs: 0});
  assert.equal(capture.cookieStoreId, "normal-store");
  assert.equal((await resolveNormalCookieStore(state.browser, {attempts: 1, delayMs: 0})).id, "normal-store");
});

test("normal recovery uses explicit normal identity and skips incognito safeguards", async () => {
  const state = harness({allowed: false, windows: [{id: 44, incognito: true, tabs: [{id: 144, windowId: 44, incognito: true}]}]});
  state.browser.extension.isAllowedIncognitoAccess = async () => { throw new Error("must not be called"); };
  const result = await state.coordinator.start(youtubeProvider, "music-bot", false);
  assert.equal(result.ok, true);
  assert.equal(state.created[0].incognito, false);
  assert.equal(state.contexts()[91].recoveryMode, "normal");
  assert.equal(state.contexts()[91].cookieStoreId, undefined);
  assert.equal(state.cookieStoreCalls(), 0);
});

test("completion closes only a broker-owned isolated window and never unrelated windows", async () => {
  const context = {provider: "youtube", profile: "default", recoveryMode: "incognito", windowId: 91, tabId: 191, brokerOwned: true};
  const state = harness({
    windows: [
      {id: 91, incognito: true, tabs: [{id: 191, windowId: 91, incognito: true}]},
      {id: 92, incognito: true, tabs: [{id: 192, windowId: 92, incognito: true}]}
    ],
    contexts: {91: context}
  });
  assert.equal(await state.coordinator.complete(structuredClone(state.contexts()), context, youtubeProvider), true);
  assert.deepEqual(state.removed, [91]);
  assert(state.windows.some(window => window.id === 92));
  assert(state.events.some(event => event.type === "isolated_recovery_shared_session_warning"));
});

test("normal and unowned windows are never auto-closed", async () => {
  const normalContext = {provider: "youtube", profile: "default", recoveryMode: "normal", windowId: 91, tabId: 191, brokerOwned: true};
  const normal = harness({contexts: {91: normalContext}});
  await normal.coordinator.complete(structuredClone(normal.contexts()), normalContext, youtubeProvider);
  assert.deepEqual(normal.removed, []);

  const unownedContext = {provider: "youtube", profile: "default", recoveryMode: "incognito", windowId: 92, tabId: 192, brokerOwned: false};
  const unowned = harness({contexts: {92: unownedContext}});
  await unowned.coordinator.complete(structuredClone(unowned.contexts()), unownedContext, youtubeProvider);
  assert.deepEqual(unowned.removed, []);
});
