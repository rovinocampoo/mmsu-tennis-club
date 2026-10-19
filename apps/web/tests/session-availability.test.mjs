import assert from "node:assert/strict";
import test from "node:test";
import {
  sessionAvailability,
  unavailableReason,
} from "../src/lib/session-availability.mjs";

test("a session at capacity is reported as full and labelled 'Fully booked'", () => {
  const availability = sessionAvailability({
    status: "open",
    capacity: 8,
    remaining_capacity: 0,
  });

  assert.equal(availability.kind, "full");
  assert.equal(unavailableReason(availability.kind), "Fully booked");
});

test("a session closed for another reason is never labelled 'Fully booked'", () => {
  // Closed to requests but still has spare capacity.
  const availability = sessionAvailability({
    status: "closed",
    capacity: 8,
    remaining_capacity: 5,
  });

  assert.equal(availability.kind, "closed");
  assert.equal(unavailableReason(availability.kind), "Not accepting requests");
  assert.notEqual(unavailableReason(availability.kind), "Fully booked");
});

test("a cancelled session (also non-open) uses the generic message", () => {
  const availability = sessionAvailability({
    status: "cancelled",
    capacity: 8,
    remaining_capacity: 8,
  });

  assert.equal(availability.kind, "closed");
  assert.equal(unavailableReason(availability.kind), "Not accepting requests");
});

test("an open session with capacity is bookable and not marked unavailable", () => {
  const availability = sessionAvailability({
    status: "open",
    capacity: 8,
    remaining_capacity: 3,
  });

  assert.equal(availability.kind, "open");
  assert.equal(availability.label, "3 spaces left");
});
