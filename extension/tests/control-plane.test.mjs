import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import test from "node:test";

import {
  createPollCoordinator,
  DEFAULT_EXPIRY_THRESHOLDS,
  evaluateBrokerStatus,
  expiryHintText,
  normalizeExpiryThresholds,
  profileVisualState,
  refreshStatusAfterPublication,
  scopeKey,
  validProfileID
} from "../control-plane.js";
import {youtubeProvider} from "../providers/youtube.js";

test("profile keys and configurable expiry thresholds are conservative", () => {
  assert.equal(scopeKey("youtube"), "youtube/default");
  assert.equal(scopeKey("youtube", "music-bot"), "youtube/music-bot");
  assert(validProfileID("private.account_1"));
  assert(!validProfileID("../private"));
  assert.deepEqual(DEFAULT_EXPIRY_THRESHOLDS, [24, 6, 1]);
  assert.deepEqual(normalizeExpiryThresholds("1, 24, 6, 6, invalid, -1"), [24, 6, 1]);
  assert.deepEqual(normalizeExpiryThresholds(""), []);
});

test("health transitions notify once per observed state transition", () => {
  const required = {provider: "youtube", profile: "default", revision: 17, auth_health: "refresh_required"};
  const first = evaluateBrokerStatus(required);
  assert.deepEqual(first.notifications.map(item => item.event), ["refresh_required"]);
  const repeated = evaluateBrokerStatus(required, first.state);
  assert.deepEqual(repeated.notifications, []);
  const healthy = evaluateBrokerStatus({...required, auth_health: "healthy"}, repeated.state);
  const requiredAgain = evaluateBrokerStatus(required, healthy.state);
  assert.deepEqual(requiredAgain.notifications.map(item => item.event), ["refresh_required"]);
  const nextRevision = evaluateBrokerStatus({...required, revision: 18}, requiredAgain.state);
  assert.deepEqual(nextRevision.notifications.map(item => item.event), ["refresh_required"]);
});

test("expiry warnings dedupe, escalate, and skip stale thresholds", () => {
  const now = Date.parse("2026-08-23T00:00:00Z");
  const status = {provider: "youtube", profile: "default", revision: 17, auth_health: "healthy"};
  const within20h = evaluateBrokerStatus({...status, auth_expires_at: "2026-08-23T20:00:00Z"}, {}, [24, 6, 1], now);
  assert.deepEqual(within20h.notifications.map(item => item.thresholdHours), [24]);
  const repeated = evaluateBrokerStatus({...status, auth_expires_at: "2026-08-23T19:00:00Z"}, within20h.state, [24, 6, 1], now);
  assert.deepEqual(repeated.notifications, []);
  const within5h = evaluateBrokerStatus({...status, auth_expires_at: "2026-08-23T05:00:00Z"}, repeated.state, [24, 6, 1], now);
  assert.deepEqual(within5h.notifications.map(item => item.thresholdHours), [6]);
  const jumpTo30m = evaluateBrokerStatus({...status, revision: 18, auth_expires_at: "2026-08-23T00:30:00Z"}, within5h.state, [24, 6, 1], now);
  assert.deepEqual(jumpTo30m.notifications.map(item => item.thresholdHours), [1]);
  assert.deepEqual(jumpTo30m.state.firedExpiryThresholds, [24, 6, 1]);
  const disabled = evaluateBrokerStatus({...status, revision: 19, auth_expires_at: "2026-08-23T00:30:00Z"}, {}, [], now);
  assert.deepEqual(disabled.notifications, []);
});

test("passed expiry hints are distinct, deduplicated, and remain hints", () => {
  const now = Date.parse("2026-08-23T12:00:00Z");
  const status = {provider: "youtube", profile: "default", revision: 17, auth_health: "healthy", auth_expires_at: "2026-08-23T11:59:59Z"};
  const first = evaluateBrokerStatus(status, {}, [24, 6, 1], now);
  assert.deepEqual(first.notifications, [{event: "expiry_hint_passed"}]);
  assert.equal(first.state.expiryHintPassed, true);
  assert.deepEqual(first.state.firedExpiryThresholds, [24, 6, 1]);
  assert.deepEqual(evaluateBrokerStatus(status, first.state, [24, 6, 1], now).notifications, []);
  assert.equal(expiryHintText(status.auth_expires_at, now), " · auth-expiry hint passed");
  assert.match(expiryHintText("2026-08-23T12:30:00Z", now), /hint in ~<1h/);
});

test("missing profiles use a warning state rather than healthy styling", () => {
  assert.equal(profileVisualState({auth_health: "missing", revision: 0}), "warning");
  assert.equal(profileVisualState({auth_health: "healthy", revision: 17}), "ok");
  assert.equal(profileVisualState({auth_health: "healthy", revision: 17}, {ok: false}), "error");
});

test("successful recovery refreshes stale broker status after any in-flight poll", async () => {
  let brokerStatus = {
    connected: true,
    profiles: [{provider: "youtube", profile: "default", revision: 0, auth_health: "missing"}]
  };
  let pollCalls = 0;
  let activePolls = 0;
  let maximumActivePolls = 0;
  let releaseStalePoll;
  const stalePollBlocked = new Promise(resolve => { releaseStalePoll = resolve; });
  const poller = createPollCoordinator(async () => {
    pollCalls++;
    activePolls++;
    maximumActivePolls = Math.max(maximumActivePolls, activePolls);
    try {
      if (pollCalls === 1) {
        await stalePollBlocked;
      } else {
        brokerStatus = {
          connected: true,
          profiles: [{provider: "youtube", profile: "default", revision: 1, auth_health: "healthy"}]
        };
      }
      return true;
    } finally {
      activePolls--;
    }
  });

  const previousPoll = poller.run();
  const recoveryUpload = (async () => {
    const response = {ok: true, revision: 1};
    await refreshStatusAfterPublication(() => poller.runAfterCurrent());
    return response;
  })();
  releaseStalePoll();

  assert.deepEqual(await recoveryUpload, {ok: true, revision: 1});
  await previousPoll;
  assert.equal(pollCalls, 2, "publication must get a poll newer than an already-running stale poll");
  assert.equal(maximumActivePolls, 1, "status polls must not overlap");
  assert.equal(brokerStatus.profiles[0].revision, 1);
  assert.equal(profileVisualState(brokerStatus.profiles[0]), "ok");
});

test("a failed post-upload status refresh does not change publication success", async () => {
  const publication = {ok: true, revision: 1};
  let logged = "";
  const refreshed = await refreshStatusAfterPublication(
    async () => { throw new Error("status unavailable"); },
    async error => { logged = error.message; }
  );
  assert.equal(refreshed, false);
  assert.deepEqual(publication, {ok: true, revision: 1});
  assert.equal(logged, "status unavailable");
});

test("YouTube capture errors match isolated and normal recovery context", () => {
  const cookies = [{name: "unrelated"}];
  assert.match(youtubeProvider.validateCapture({cookies, recoveryContext: "incognito"}).notification, /incognito window/);
  const normal = youtubeProvider.validateCapture({cookies, recoveryContext: "normal"}).notification;
  assert.match(normal, /current normal browser session/);
  assert.doesNotMatch(normal, /incognito/);
});

test("service worker has explicit click UI, spanning polling, and normal-window close guard", async () => {
  const source = await readFile(new URL("../service-worker.js", import.meta.url), "utf8");
  assert.match(source, /chrome\.notifications\.onClicked\.addListener/);
  assert.match(source, /chrome\.tabs\.create\(\{url: chrome\.runtime\.getURL\("popup\.html"\)\}\)/);
  assert.match(source, /recoveryCoordinator\.complete/);
  assert.match(source, /\/v1\/status/);
  const uploadSucceeded = source.indexOf('logDiagnostic("sync_succeeded"');
  assert(uploadSucceeded >= 0);
  assert(source.indexOf("refreshStatusAfterPublication", uploadSucceeded) > uploadSucceeded);
  assert.doesNotMatch(source, /actualContext === "normal"[^\n]*chrome\.windows\.remove/);
  assert.doesNotMatch(source, /inIncognitoContext/);
});
