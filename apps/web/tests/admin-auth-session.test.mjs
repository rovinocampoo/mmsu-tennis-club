import assert from "node:assert/strict";
import test from "node:test";
import { fetchWithAdminSession, getCurrentAdminAccessToken } from "../src/lib/admin-auth-session.mjs";
import { adminDataReducer, adminSessionReducer, initialAdminDataState, initialAdminSessionState } from "../src/lib/admin-session-state.mjs";

test("reads the current Supabase session instead of retaining an earlier token", async () => {
  const auth = {
    async getSession() { return { data: { session: { access_token: "rotated-token", expires_at: 2_000_000_000 } }, error: null }; },
    async refreshSession() { assert.fail("a session with ample lifetime should not refresh"); },
  };
  assert.equal(await getCurrentAdminAccessToken(auth, 1_000_000_000_000), "rotated-token");
});

test("refreshes a near-expiry session before sending the admin request", async () => {
  let refreshCalls = 0;
  let authorization = "";
  const auth = {
    async getSession() { return { data: { session: { access_token: "near-expiry", expires_at: 1_010 } }, error: null }; },
    async refreshSession() { refreshCalls++; return { data: { session: { access_token: "refreshed-token", expires_at: 2_000 } }, error: null }; },
  };
  const response = await fetchWithAdminSession(auth, async (_input, init) => {
    authorization = new Headers(init.headers).get("Authorization");
    return new Response("{}", { status: 200 });
  }, "/admin", {}, { now: 1_000_000, sleep: async () => {} });
  assert.equal(refreshCalls, 1);
  assert.equal(authorization, "Bearer refreshed-token");
  assert.equal(response.status, 200);
});

test("retries one 401 with a concurrently refreshed Supabase token", async () => {
  let sessionReads = 0;
  const sent = [];
  const auth = {
    async getSession() {
      sessionReads++;
      const token = sessionReads === 1 ? "old-token" : "new-token";
      return { data: { session: { access_token: token, expires_at: 2_000 } }, error: null };
    },
    async refreshSession() { assert.fail("the newer shared session should be reused"); },
  };
  const response = await fetchWithAdminSession(auth, async (_input, init) => {
    const token = new Headers(init.headers).get("Authorization");
    sent.push(token);
    return new Response("{}", { status: token === "Bearer old-token" ? 401 : 200 });
  }, "/admin", {}, { now: 1_000_000, sleep: async () => {} });
  assert.deepEqual(sent, ["Bearer old-token", "Bearer new-token"]);
  assert.equal(response.status, 200);
});

test("does not call an admin API without a Supabase session", async () => {
  const auth = {
    async getSession() { return { data: { session: null }, error: null }; },
    async refreshSession() { assert.fail("no session should not refresh"); },
  };
  await assert.rejects(getCurrentAdminAccessToken(auth), /session has expired/i);
});

test("initial session restoration stays in checking until it resolves", () => {
  let state = initialAdminSessionState;
  assert.equal(state.status, "checking");
  state = adminSessionReducer(state, { type: "snapshot", hasSession: true });
  assert.equal(state.status, "authenticated");
});

test("restored session proceeds without requiring another sign-in and wins snapshot races", () => {
  const signedIn = adminSessionReducer(initialAdminSessionState, { type: "auth_event", hasSession: true });
  const afterLateEmptySnapshot = adminSessionReducer(signedIn, { type: "snapshot", hasSession: false });
  assert.equal(afterLateEmptySnapshot.status, "authenticated");
});

test("transient verifier 503 recovers on a bounded retry with neutral progress", async () => {
  let calls = 0;
  const retryStates = [];
  const auth = {
    async getSession() { return { data: { session: { access_token: "current-token", expires_at: 2_000 } }, error: null }; },
    async refreshSession() { assert.fail("valid session should not need refresh"); },
  };
  const response = await fetchWithAdminSession(auth, async () => {
    calls++;
    return calls === 1
      ? Response.json({ error: "authentication service is temporarily unavailable" }, { status: 503 })
      : Response.json({ requests: [] }, { status: 200 });
  }, "/admin", {}, { now: 1_000_000, sleep: async () => {}, onVerificationRetry: (retrying) => retryStates.push(retrying) });
  assert.equal(response.status, 200);
  assert.equal(calls, 2);
  assert.deepEqual(retryStates, [true, false]);
});

test("persistent verifier 503 returns after bounded retries in retryable UI state", async () => {
  let calls = 0;
  const auth = {
    async getSession() { return { data: { session: { access_token: "current-token", expires_at: 2_000 } }, error: null }; },
    async refreshSession() { assert.fail("valid session should not need refresh"); },
  };
  const response = await fetchWithAdminSession(auth, async () => {
    calls++;
    return Response.json({ error: "authentication service is temporarily unavailable" }, { status: 503 });
  }, "/admin", {}, { now: 1_000_000, sleep: async () => {} });
  assert.equal(response.status, 503);
  assert.equal(calls, 3);
  assert.equal(adminDataReducer(initialAdminDataState, { type: "unavailable" }).status, "unavailable");
});

test("invalid tokens and verified non-admin responses are not retried", async () => {
  for (const status of [401, 403]) {
    let calls = 0;
    const auth = {
      async getSession() { return { data: { session: { access_token: "same-token", expires_at: 2_000 } }, error: null }; },
      async refreshSession() { assert.fail("a verified response must not trigger session refresh"); },
    };
    const response = await fetchWithAdminSession(auth, async () => { calls++; return new Response("{}", { status }); }, "/admin", {}, { now: 1_000_000, sleep: async () => {} });
    assert.equal(response.status, status);
    assert.equal(calls, 1);
  }
  assert.equal(adminDataReducer(initialAdminDataState, { type: "denied" }).status, "denied");
});
