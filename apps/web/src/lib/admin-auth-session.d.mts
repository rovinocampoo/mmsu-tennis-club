export type AdminAuthSession = { access_token: string; expires_at?: number | null };
export type AdminAuthClient = {
  getSession(): Promise<{ data: { session: AdminAuthSession | null }; error: unknown | null }>;
  refreshSession(): Promise<{ data: { session: AdminAuthSession | null }; error: unknown | null }>;
};
export type AdminFetchOptions = {
  now?: number;
  maxVerificationRetries?: number;
  sleep?: (milliseconds: number) => Promise<unknown>;
  onVerificationRetry?: (retrying: boolean) => void;
};

export function getCurrentAdminAccessToken(auth: AdminAuthClient, now?: number): Promise<string>;
export function fetchWithAdminSession(
  auth: AdminAuthClient,
  fetcher: typeof fetch,
  input: RequestInfo | URL,
  init?: RequestInit,
  options?: AdminFetchOptions,
): Promise<Response>;
