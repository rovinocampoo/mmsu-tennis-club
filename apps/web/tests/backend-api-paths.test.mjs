import assert from "node:assert/strict";
import test from "node:test";
import { isBackendRouteAllowed } from "../src/lib/backend-api-paths.mjs";

const sessionId = "11111111-1111-4111-8111-111111111111";

test("allows coach portal and admin scheduling API methods", () => {
  for (const [method, path] of [
    ["GET", ["coach", "schedule"]],
    ["GET", ["coach", "coaches"]],
    ["GET", ["coach", "sessions"]],
    ["GET", ["admin", "coach-availability"]],
    ["GET", ["admin", "coaches"]],
    ["POST", ["admin", "coaches"]],
    ["PATCH", ["admin", "coaches", sessionId]],
    ["GET", ["admin", "training-programs"]],
    ["GET", ["admin", "training-sessions"]],
    ["POST", ["admin", "training-sessions"]],
    ["POST", ["admin", "training-sessions", "batch"]],
    ["POST", ["admin", "training-sessions", "publish"]],
    ["PUT", ["admin", "training-sessions", sessionId]],
    ["PATCH", ["admin", "training-sessions", sessionId]],
    ["PATCH", ["admin", "training-sessions", sessionId, "coach"]],
    ["POST", ["admin", "booking-requests", sessionId, "calendar", "retry"]],
    ["POST", ["feedback-submissions"]],
    ["GET", ["admin", "feedback-submissions"]],
    ["PATCH", ["admin", "feedback-submissions", sessionId]],
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
  assert.equal(isBackendRouteAllowed("POST", ["admin", "booking-requests", "not-a-uuid", "calendar", "retry"]), false);
  assert.equal(isBackendRouteAllowed("POST", ["admin", "booking-requests", sessionId, "calendar", "unknown"]), false);
  assert.equal(isBackendRouteAllowed("GET", ["coach", "availability"]), false);
  assert.equal(isBackendRouteAllowed("POST", ["coach", "availability"]), false);
  assert.equal(isBackendRouteAllowed("DELETE", ["coach", "availability", sessionId]), false);
  assert.equal(isBackendRouteAllowed("POST", ["admin", "coaches", sessionId]), false);
  assert.equal(isBackendRouteAllowed("PATCH", ["admin", "coaches", "not-a-uuid"]), false);
  assert.equal(isBackendRouteAllowed("DELETE", ["admin", "coaches", sessionId]), false);
  assert.equal(isBackendRouteAllowed("GET", ["feedback-submissions"]), false);
  assert.equal(isBackendRouteAllowed("DELETE", ["admin", "feedback-submissions", sessionId]), false);
  assert.equal(isBackendRouteAllowed("PATCH", ["admin", "feedback-submissions", "not-a-uuid"]), false);
});
