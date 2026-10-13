import assert from "node:assert/strict";
import test from "node:test";
import { bookingRequestReducer, initialBookingRequestState } from "../src/components/booking-request-form-state.mjs";

test("accepted submission displays pending status and reference and cannot return to editing", () => {
  const submitting = bookingRequestReducer(initialBookingRequestState, { type: "start" });
  const accepted = bookingRequestReducer(submitting, {
    type: "success",
    requestReference: "MMSU-123",
    trackingToken: "opaque-private-token",
  });

  assert.deepEqual(accepted, {
    phase: "accepted",
    requestReference: "MMSU-123",
    trackingToken: "opaque-private-token",
    error: null,
  });
  assert.equal(bookingRequestReducer(accepted, { type: "start" }), accepted);
});

test("failed submission returns to editable form with useful error", () => {
  const submitting = bookingRequestReducer(initialBookingRequestState, { type: "start" });
  const failed = bookingRequestReducer(submitting, { type: "failure", error: "Session is unavailable." });

  assert.deepEqual(failed, {
    phase: "editing",
    requestReference: null,
    trackingToken: null,
    error: "Session is unavailable.",
  });
});
