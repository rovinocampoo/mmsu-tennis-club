export const initialAdminSessionState = { status: "checking" };
export const initialAdminDataState = { status: "idle" };

export function adminSessionReducer(state, action) {
  switch (action.type) {
    case "snapshot":
      return state.status === "checking"
        ? { status: action.hasSession ? "authenticated" : "signed_out" }
        : state;
    case "auth_event":
      return { status: action.hasSession ? "authenticated" : "signed_out" };
    case "unavailable":
      return state.status === "checking" ? { status: "unavailable" } : state;
    case "retry":
      return { status: "checking" };
    default:
      return state;
  }
}

export function adminDataReducer(_state, action) {
  switch (action.type) {
    case "loading": return { status: "loading" };
    case "ready": return { status: "ready" };
    case "unavailable": return { status: "unavailable" };
    case "denied": return { status: "denied" };
    case "reset": return initialAdminDataState;
    default: return _state;
  }
}
