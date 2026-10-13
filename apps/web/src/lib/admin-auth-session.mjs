const refreshWindowMs = 60_000;

function sessionExpired() {
  return new Error("Your admin session has expired. Please sign in again.");
}

export async function getCurrentAdminAccessToken(auth, now = Date.now()) {
  let result;
  try {
    result = await auth.getSession();
  } catch {
    throw new Error("Could not check your admin session. Try again shortly.");
  }
  if (result.error) throw new Error("Could not check your admin session. Try again shortly.");

  let session = result.data.session;
  if (!session?.access_token) throw sessionExpired();
  if (typeof session.expires_at === "number" && session.expires_at * 1000 <= now + refreshWindowMs) {
    let refreshed;
    try {
      refreshed = await auth.refreshSession();
    } catch {
      throw new Error("Could not refresh your admin session. Check your connection and retry.");
    }
    if (refreshed.error) throw new Error("Could not refresh your admin session. Check your connection and retry.");
    session = refreshed.data.session;
    if (!session?.access_token) throw sessionExpired();
  }
  return session.access_token;
}

async function isAuthVerifierUnavailable(response) {
  if (response.status !== 503) return false;
  const body = await response.clone().json().catch(() => ({}));
  return typeof body?.error === "string" && body.error.toLowerCase().includes("authentication");
}

const wait = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

export async function fetchWithAdminSession(auth, fetcher, input, init = {}, options = {}) {
  const send = (token) => {
    const headers = new Headers(init.headers);
    headers.set("Authorization", `Bearer ${token}`);
    return fetcher(input, { ...init, headers, cache: "no-store" });
  };

  const maxRetries = Number.isInteger(options.maxVerificationRetries)
    ? Math.max(0, options.maxVerificationRetries)
    : 2;
  let retrying = false;
  try {
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      let accessToken = await getCurrentAdminAccessToken(auth, options.now ?? Date.now());
      let response = await send(accessToken);
      if (response.status === 401) {
        // Another request may have refreshed the shared Supabase session while this
        // request was in flight. Retry once with that token rather than stale React state.
        try {
          const latest = await auth.getSession();
          if (!latest.error && latest.data.session?.access_token && latest.data.session.access_token !== accessToken) {
            accessToken = latest.data.session.access_token;
            response = await send(accessToken);
          }
        } catch {
          // Keep the original API response; callers will handle a definite 401 safely.
        }
      }
      if (attempt === maxRetries || !(await isAuthVerifierUnavailable(response))) return response;
      if (!retrying) {
        retrying = true;
        options.onVerificationRetry?.(true);
      }
      const delayMs = Math.min(200 * (attempt + 1), 600);
      await (options.sleep || wait)(delayMs);
    }
  } finally {
    if (retrying) options.onVerificationRetry?.(false);
  }
  throw new Error("Admin request retry loop ended unexpectedly.");
}
