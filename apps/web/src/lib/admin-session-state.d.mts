export type AdminSessionState = { status: "checking" | "signed_out" | "authenticated" | "unavailable" };
export type AdminDataState = { status: "idle" | "loading" | "ready" | "unavailable" | "denied" };
export const initialAdminSessionState: AdminSessionState;
export const initialAdminDataState: AdminDataState;
export function adminSessionReducer(state: AdminSessionState, action: { type: string; hasSession?: boolean }): AdminSessionState;
export function adminDataReducer(state: AdminDataState, action: { type: string }): AdminDataState;
