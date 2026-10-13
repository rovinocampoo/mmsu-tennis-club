export type BookingRequestState = {
  phase: "editing" | "submitting" | "accepted";
  requestReference: string | null;
  trackingToken: string | null;
  error: string | null;
};

export type BookingRequestAction =
  | { type: "start" }
  | { type: "success"; requestReference: string; trackingToken: string }
  | { type: "failure"; error: string };

export const initialBookingRequestState: BookingRequestState;
export function bookingRequestReducer(state: BookingRequestState, action: BookingRequestAction): BookingRequestState;
