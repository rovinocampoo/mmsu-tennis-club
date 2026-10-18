import assert from "node:assert/strict";
import test from "node:test";
import { isBackendRouteAllowed } from "../src/lib/backend-api-paths.mjs";

const sessionId = "11111111-1111-4111-8111-111111111111";

test("allows coach availability and admin scheduling API methods", () => {
  for (const [method, path] of [
    ["GET", ["coach", "availability"]],
    ["GET", ["coach", "sessions"]],
    ["POST", ["coach", "availability"]],
    ["DELETE", ["coach", "availability", sessionId]],
    ["GET", ["admin", "coach-availability"]],
    ["GET", ["admin", "training-programs"]],
    ["GET", ["admin", "training-sessions"]],
    ["POST", ["admin", "training-sessions"]],
    ["POST", ["admin", "training-sessions", "batch"]],
    ["PUT", ["admin", "training-sessions", sessionId]],
    ["PATCH", ["admin", "training-sessions", sessionId]],
    ["PATCH", ["admin", "training-sessions", sessionId, "coach"]],
  ]) {
    assert.equal(isBackendRouteAllowed(method, path), true, `${method} /${path.join("/")}`);
  }
});

test("keeps the backend proxy method and identifier allowlist narrow", () => {
  assert.equal(isBackendRouteAllowed("PUT", ["admin", "training-sessions"]), false);
  assert.equal(isBackendRouteAllowed("PUT", ["admin", "training-sessions", "not-a-uuid"]), false);
  assert.equal(isBackendRouteAllowed("DELETE", ["admin", "training-sessions", sessionId]), false);
  assert.equal(isBackendRouteAllowed("POST", ["admin", "training-sessions", "unknown"]), false);
  assert.equal(isBackendRouteAllowed("PATCH", ["admin", "training-sessions", "not-a-uuid"]), false);
  assert.equal(isBackendRouteAllowed("PATCH", ["admin", "training-sessions", "not-a-uuid", "coach"]), false);
  assert.equal(isBackendRouteAllowed("GET", ["admin", "unknown"]), false);
  assert.equal(isBackendRouteAllowed("GET", ["coach", "availability", sessionId]), false);
});
