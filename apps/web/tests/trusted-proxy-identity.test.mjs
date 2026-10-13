import assert from "node:assert/strict";
import test from "node:test";
import { trustedProxyIdentity } from "../src/lib/trusted-proxy-identity.mjs";

const secret = "0123456789abcdef0123456789abcdef";

test("uses only Vercel's forwarded client IP for trusted proxy identity", () => {
  const headers = new Headers({
    "x-vercel-forwarded-for": "198.51.100.24",
    "x-forwarded-for": "203.0.113.99",
    forwarded: "for=203.0.113.98",
  });
  assert.deepEqual(trustedProxyIdentity(headers, secret, true), {
    clientIP: "198.51.100.24",
    sharedSecret: secret,
  });
});

test("production public proxy requests fail closed without shared secret", () => {
  const headers = new Headers({ "x-vercel-forwarded-for": "198.51.100.24" });
  assert.deepEqual(trustedProxyIdentity(headers, "", true), { error: true });
  assert.deepEqual(trustedProxyIdentity(headers, "short", true), { error: true });
});

test("production public proxy requests fail closed without a valid Vercel IP", () => {
  for (const value of [null, "", "not-an-ip", "198.51.100.1, 203.0.113.1"]) {
    const headers = new Headers();
    if (value !== null) headers.set("x-vercel-forwarded-for", value);
    assert.deepEqual(trustedProxyIdentity(headers, secret, true), { error: true });
  }
});

test("development without proxy configuration leaves API identity to RemoteAddr", () => {
  const headers = new Headers({ "x-forwarded-for": "203.0.113.99" });
  assert.equal(trustedProxyIdentity(headers, "", false), null);
});
