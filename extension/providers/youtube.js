const setupRobotsURL = "https://www.youtube.com/robots.txt?media_cookie_broker_setup=1";

export const youtubeProvider = {
  id: "youtube",
  label: "YouTube",
  cookieDomains: ["youtube.com"],
  enabledByDefault: true,
  helpText: "Uses a fresh incognito session when no other incognito window is open; isolation can be disabled in settings.",
  authCookieNames: ["SAPISID", "SID", "HSID", "SSID", "LOGIN_INFO", "__Secure-3PSID", "__Secure-1PSID"],
  setupRobotsURL,
  recovery: {
    defaultIsolated: true,
    isolationConfigurable: true,
    closeIsolatedWindowAfterUpload: true
  },
  validateCapture({cookies, recoveryContext}) {
    if (cookies.some(cookie => this.authCookieNames.includes(cookie.name))) return {ok: true};
    const notification = recoveryContext === "incognito"
      ? "Sign in to YouTube in the broker-created incognito window, then try again."
      : "Sign in to YouTube in your current normal browser session, then try again.";
    return {
      ok: false,
      error: "YouTube authentication cookies are missing",
      notification
    };
  },
  matchesSetupNavigation(url) {
    return url?.startsWith(setupRobotsURL);
  },
  async setup({logDiagnostic, profile, useIsolated, browser = chrome}) {
    try {
      const loginURL = `https://accounts.google.com/ServiceLogin?service=youtube&continue=${encodeURIComponent(setupRobotsURL)}`;
      const window = await browser.windows.create({incognito: useIsolated, focused: true, type: "normal", url: loginURL});
      await logDiagnostic("recovery_context_opened", {provider: this.id, profile, recovery_context: useIsolated ? "incognito" : "normal"});
      return {ok: true, windowId: window.id, tabId: window.tabs?.[0]?.id};
    } catch (error) {
      const message = String(error?.message || error);
      await logDiagnostic("recovery_context_failed", {provider: this.id, profile, recovery_context: useIsolated ? "incognito" : "normal", error: message}, "error");
      return {ok: false, error: message};
    }
  }
};
