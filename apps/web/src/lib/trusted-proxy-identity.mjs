import { isIP } from "node:net";

export function trustedProxyIdentity(headers, sharedSecret, isProduction) {
  if (!sharedSecret || sharedSecret.length < 32) {
    return isProduction ? { error: true } : null;
  }

  const clientIP = headers.get("x-vercel-forwarded-for")?.trim();
  if (!clientIP || isIP(clientIP) === 0) {
    return isProduction ? { error: true } : null;
  }

  return { clientIP, sharedSecret };
}
