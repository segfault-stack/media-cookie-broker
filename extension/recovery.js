const INCOGNITO_PERMISSION_ERROR = "Enable Allow in incognito for this extension in Chrome's extension details, then retry.";
const EXISTING_INCOGNITO_ERROR = "Close all existing incognito windows before starting isolated recovery, then retry.";
const ACTIVE_ISOLATED_ERROR = "An isolated recovery is already in progress. Finish or close it before starting another.";

function recoveryMode(useIsolated) {
  return useIsolated ? "incognito" : "normal";
}

function tabBelongsToWindow(tab, windowID) {
  return Number.isInteger(tab?.id) && tab.windowId === windowID;
}

export async function resolveCookieStoreForTab(browser, tabID, {attempts = 10, delayMs = 150} = {}) {
  if (!Number.isInteger(tabID)) throw new Error("Recovery tab identity is unavailable; recovery was not started.");
  const attemptCount = Math.max(1, Math.trunc(attempts));
  for (let attempt = 0; attempt < attemptCount; attempt++) {
    const stores = await browser.cookies.getAllCookieStores();
    const store = stores.find(item => Array.isArray(item.tabIds) && item.tabIds.includes(tabID));
    if (store?.id) return store;
    if (attempt + 1 < attemptCount) await new Promise(resolve => setTimeout(resolve, Math.max(0, delayMs)));
  }
  throw new Error("Chrome did not expose the cookie store for the recovery tab in time.");
}

export async function resolveNormalCookieStore(browser, storeOptions) {
  const tabs = await browser.tabs.query({});
  const tab = tabs.find(candidate => Number.isInteger(candidate.id) && candidate.incognito === false);
  if (!tab) throw new Error("Chrome did not expose a normal browser tab for cookie-store resolution.");
  return resolveCookieStoreForTab(browser, tab.id, storeOptions);
}

export function recoveryMatchesTab(context, tab) {
  if (!context || !tabBelongsToWindow(tab, context.windowId) || tab.id !== context.tabId) return false;
  return Boolean(tab.incognito) === (context.recoveryMode === "incognito");
}

export async function resolveRecoveryCaptureContext(browser, providerID, profile, context, storeOptions) {
  if (!context || context.provider !== providerID || context.profile !== profile) {
    throw new Error("Recovery context does not match the requested provider/profile.");
  }
  const observed = await browser.windows.get(context.windowId, {populate: true});
  const expectedIncognito = context.recoveryMode === "incognito";
  if (!observed || observed.id !== context.windowId || Boolean(observed.incognito) !== expectedIncognito) {
    throw new Error("The tracked recovery window no longer matches its recorded browser mode.");
  }
  const tab = observed.tabs?.find(candidate => candidate.id === context.tabId);
  if (!recoveryMatchesTab(context, tab)) {
    throw new Error("The tracked recovery tab no longer matches its recorded browser mode.");
  }
  const store = await resolveCookieStoreForTab(browser, context.tabId, storeOptions);
  return {...context, cookieStoreId: store.id};
}

export function createRecoveryCoordinator({browser, logDiagnostic, loadContexts, saveContexts}) {
  let isolatedStartInFlight = false;

  async function refuse(provider, profile, reason, error) {
    await logDiagnostic("recovery_context_refused", {
      provider: provider.id,
      profile,
      recovery_context: "incognito",
      reason
    }, "warning");
    return {ok: false, error};
  }

  async function observeRecoveryWindow(result, expectedMode) {
    if (!Number.isInteger(result.windowId)) throw new Error("Chrome did not return the recovery window identity.");
    const observed = await browser.windows.get(result.windowId, {populate: true});
    if (!observed || observed.id !== result.windowId || Boolean(observed.incognito) !== (expectedMode === "incognito")) {
      throw new Error(`Chrome did not create the requested ${expectedMode} recovery window.`);
    }
    const tab = observed.tabs?.find(item => item.id === result.tabId) || observed.tabs?.[0];
    if (!tabBelongsToWindow(tab, observed.id)) throw new Error("Chrome did not return the recovery tab identity.");
    if (Boolean(tab.incognito) !== (expectedMode === "incognito")) {
      throw new Error(`Chrome did not create the requested ${expectedMode} recovery tab.`);
    }
    return {window: observed, tab};
  }

  async function start(provider, profile, useIsolated) {
    if (useIsolated && isolatedStartInFlight) {
      return refuse(provider, profile, "isolated_recovery_start_in_progress", ACTIVE_ISOLATED_ERROR);
    }

    if (useIsolated) isolatedStartInFlight = true;
    let createdWindowID;
    try {
      const contexts = await loadContexts();
      if (useIsolated) {
        const allowed = await browser.extension.isAllowedIncognitoAccess();
        if (!allowed) return refuse(provider, profile, "incognito_access_disabled", INCOGNITO_PERMISSION_ERROR);
        const windows = await browser.windows.getAll({populate: false});
        const openIncognitoIDs = new Set(windows.filter(window => window.incognito).map(window => window.id));
        if (Object.values(contexts).some(context => context.recoveryMode === "incognito" && openIncognitoIDs.has(context.windowId))) {
          return refuse(provider, profile, "isolated_recovery_active", ACTIVE_ISOLATED_ERROR);
        }
        let pruned = false;
        for (const [windowID, context] of Object.entries(contexts)) {
          if (context.recoveryMode === "incognito" && !openIncognitoIDs.has(context.windowId)) {
            delete contexts[windowID];
            pruned = true;
          }
        }
        if (pruned) await saveContexts(contexts);
        if (openIncognitoIDs.size) {
          return refuse(provider, profile, "existing_incognito_window", EXISTING_INCOGNITO_ERROR);
        }
      }

      const mode = recoveryMode(useIsolated);
      const result = await provider.setup({logDiagnostic, profile, useIsolated, browser});
      if (!result.ok) return result;
      createdWindowID = result.windowId;
      const observed = await observeRecoveryWindow(result, mode);
      if (useIsolated) {
        const windows = await browser.windows.getAll({populate: false});
        if (windows.some(window => window.incognito && window.id !== observed.window.id)) {
          throw new Error(EXISTING_INCOGNITO_ERROR);
        }
      }
      const context = {
        provider: provider.id,
        profile,
        recoveryMode: mode,
        windowId: observed.window.id,
        tabId: observed.tab.id,
        openedAt: new Date().toISOString(),
        brokerOwned: true
      };
      contexts[String(context.windowId)] = context;
      await saveContexts(contexts);
      return {ok: true, ...context};
    } catch (error) {
      if (useIsolated && Number.isInteger(createdWindowID)) {
        try { await browser.windows.remove(createdWindowID); } catch (_) {}
      }
      const message = String(error?.message || error);
      await logDiagnostic("recovery_context_failed", {
        provider: provider.id,
        profile,
        recovery_context: recoveryMode(useIsolated),
        error: message
      }, "error");
      return {ok: false, error: message};
    } finally {
      if (useIsolated) isolatedStartInFlight = false;
    }
  }

  async function complete(contexts, context, provider) {
    if (!context || contexts[String(context.windowId)]?.tabId !== context.tabId) return false;
    delete contexts[String(context.windowId)];
    await saveContexts(contexts);
    if (context.recoveryMode !== "incognito" || !provider.recovery?.closeIsolatedWindowAfterUpload || !context.brokerOwned) {
      return true;
    }

    try {
      const windows = await browser.windows.getAll({populate: false});
      if (windows.some(window => window.incognito && window.id !== context.windowId)) {
        await logDiagnostic("isolated_recovery_shared_session_warning", {
          provider: context.provider,
          profile: context.profile,
          recovery_context: context.recoveryMode,
          reason: "another_incognito_window_opened_during_recovery"
        }, "warning");
      }
    } catch (_) {}
    try { await browser.windows.remove(context.windowId); } catch (_) {}
    return true;
  }

  return {start, complete};
}
