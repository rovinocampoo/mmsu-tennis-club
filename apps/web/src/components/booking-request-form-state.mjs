export const initialBookingRequestState = {
  phase: "editing",
  requestReference: null,
  trackingToken: null,
  error: null,
};

export function bookingRequestReducer(state, action) {
  switch (action.type) {
    case "start":
      return state.phase === "editing" ? { phase: "submitting", requestReference: null, trackingToken: null, error: null } : state;
    case "success":
      return state.phase === "submitting" ? { phase: "accepted", requestReference: action.requestReference, trackingToken: action.trackingToken, error: null } : state;
    case "failure":
      return state.phase === "submitting" ? { phase: "editing", requestReference: null, trackingToken: null, error: action.error } : state;
    default:
      return state;
  }
}
