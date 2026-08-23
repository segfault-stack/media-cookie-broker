import assert from "node:assert/strict";
import test from "node:test";

const storage = new Map();
globalThis.chrome = {
  storage: {
    local: {
      async get(key) {
        if (typeof key === "string") return {[key]: storage.get(key)};
        return {};
      },
      async set(values) {
        for (const [key, value] of Object.entries(values)) storage.set(key, value);
      }
    }
  }
};

async function gzipBase64(value) {
  const stream = new Blob([JSON.stringify(value)]).stream().pipeThrough(new CompressionStream("gzip"));
  const bytes = new Uint8Array(await new Response(stream).arrayBuffer());
  return btoa(String.fromCharCode(...bytes));
}

async function gunzipJSON(value) {
  const stream = new Blob([value]).stream().pipeThrough(new DecompressionStream("gzip"));
  return new Response(stream).json();
}

test("remote diagnostics are separated by event provider", async () => {
  const events = [
    {timestamp: new Date().toISOString(), type: "youtube_event", severity: "info", details: {provider: "youtube"}},
    {timestamp: new Date().toISOString(), type: "tiktok_event", severity: "warning", details: {provider: "tiktok"}},
    {timestamp: new Date().toISOString(), type: "system_event", severity: "info", details: {}}
  ];
  storage.set("diagnosticBatches", [{
    firstAt: events[0].timestamp,
    lastAt: events.at(-1).timestamp,
    count: events.length,
    gzip: await gzipBase64(events),
    pending: true
  }]);
  const requests = [];
  globalThis.fetch = async (_url, options) => {
    requests.push(await gunzipJSON(options.body));
    return {ok: true, status: 201};
  };

  const {uploadDiagnostics} = await import("../diagnostics.js");
  const uploaded = await uploadDiagnostics({
    endpoint: "https://broker.example.com",
    username: "publisher",
    password: "not-a-real-password",
    enabledProviders: ["youtube", "tiktok"],
    remoteDiagnosticsEnabled: true
  }, () => "Basic fake-authorization");

  assert.equal(uploaded, true);
  assert.deepEqual(requests.map(request => request.provider), ["youtube", "tiktok"]);
  assert.deepEqual(requests.map(request => request.profile), ["default", "default"]);
  assert.deepEqual(requests.map(request => request.events.map(event => event.type)), [["youtube_event"], ["tiktok_event"]]);
  assert.equal(storage.get("diagnosticBatches")[0].pending, false);
});

test("remote diagnostics require explicit enablement without disabling local history", async () => {
  let fetched = false;
  globalThis.fetch = async () => { fetched = true; throw new Error("must not fetch"); };
  const {logDiagnostic, recentDiagnostics, uploadDiagnostics} = await import("../diagnostics.js");
  await logDiagnostic("broker_status_poll_failed", {provider: "youtube", profile: "default", password: "must-redact"}, "warning");
  const uploaded = await uploadDiagnostics({
    endpoint: "https://broker.example.com",
    username: "publisher",
    password: "not-a-real-password",
    enabledProviders: ["youtube"]
  }, () => "Basic fake-authorization");
  assert.equal(uploaded, true);
  assert.equal(fetched, false);
  const events = await recentDiagnostics();
  const event = events.find(item => item.type === "broker_status_poll_failed");
  assert(event);
  assert.equal(event.details.password, "[redacted]");
});

test("diagnostic keys and obvious credential-shaped strings are redacted conservatively", async () => {
  const {redactDiagnosticValue} = await import("../diagnostics.js");
  const redacted = redactDiagnosticValue({
    password: "one",
    passwd: "two",
    requestAuthorization: "three",
    basic_credentials: "three-b",
    bearer_auth: "three-c",
    cookie: "three-d",
    cookieValue: "four",
    cookie_header: "five",
    accessToken: "six",
    access_token_expiry: "six-b",
    client_secret: "seven",
    client_secret_status: "seven-b",
    apiKey: "eight",
    apiKeyPresent: "eight-b",
    brokerMasterKey: "nine",
    private_key: "ten",
    error: "Authorization: Basic abcdefghijkl; Cookie: SID=cookie-secret; token=query-secret access_token=second api_key=third secret=fourth password=fifth master_key=sixth Bearer abcdefghijklmnop",
    cookie_names: ["SID"],
    auth_health: "refresh_required",
    token_bucket_state: "exhausted"
  });
  for (const key of ["password", "passwd", "requestAuthorization", "basic_credentials", "bearer_auth", "cookie", "cookieValue", "cookie_header", "accessToken", "access_token_expiry", "client_secret", "client_secret_status", "apiKey", "apiKeyPresent", "brokerMasterKey", "private_key"]) {
    assert.equal(redacted[key], "[redacted]", key);
  }
  assert.doesNotMatch(redacted.error, /cookie-secret|query-secret|second|third|fourth|fifth|sixth|abcdefghijkl/);
  assert.match(redacted.error, /\[redacted\]/);
  assert.deepEqual(redacted.cookie_names, ["SID"]);
  assert.equal(redacted.auth_health, "refresh_required");
  assert.equal(redacted.token_bucket_state, "exhausted");
});

test("diagnostic redaction enforces depth across objects, arrays, and mixed structures", async () => {
  const {redactDiagnosticValue} = await import("../diagnostics.js");
  const atLimit = redactDiagnosticValue({one: {two: {three: {four: "safe"}}}});
  assert.equal(atLimit.one.two.three.four, "safe");

  const nestedObjects = redactDiagnosticValue({one: {two: {three: {four: {five: "must-not-survive"}}}}});
  assert.equal(nestedObjects.one.two.three.four, "[truncated]");
  assert.doesNotMatch(JSON.stringify(nestedObjects), /must-not-survive/);

  const nestedArrays = redactDiagnosticValue({one: [[[[["must-not-survive"]]]]]});
  assert.equal(nestedArrays.one[0][0][0], "[truncated]");
  assert.doesNotMatch(JSON.stringify(nestedArrays), /must-not-survive/);

  const mixed = redactDiagnosticValue({one: [{two: [{three: [{four: "must-not-survive"}]}]}]});
  assert.equal(mixed.one[0].two[0], "[truncated]");
  assert.doesNotMatch(JSON.stringify(mixed), /must-not-survive/);
});

test("sensitive diagnostic fields are redacted near the limit and truncated beyond it", async () => {
  const {redactDiagnosticValue} = await import("../diagnostics.js");
  const nearLimit = redactDiagnosticValue({one: {two: {three: {password: "must-not-survive"}}}});
  assert.equal(nearLimit.one.two.three.password, "[redacted]");
  const beyondLimit = redactDiagnosticValue({one: {two: {three: {four: {password: "must-not-survive"}}}}});
  assert.equal(beyondLimit.one.two.three.four, "[truncated]");
  assert.doesNotMatch(JSON.stringify(beyondLimit), /must-not-survive/);
});
