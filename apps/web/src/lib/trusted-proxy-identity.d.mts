export function trustedProxyIdentity(
  headers: Pick<Headers, "get">,
  sharedSecret: string,
  isProduction: boolean,
): { clientIP: string; sharedSecret: string } | { error: true } | null;
