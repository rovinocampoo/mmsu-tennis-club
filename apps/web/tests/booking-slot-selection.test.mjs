import test from "node:test";
import assert from "node:assert/strict";
import {
  MAX_BOOKING_DATES,
  MAX_BOOKING_SLOTS,
  validateBookingSelection,
} from "../src/lib/booking-slot-selection.mjs";
import { formatSubmittedAt } from "../src/lib/admin-submission-time.mjs";

function slot(id, date, startHour, endHour) {
  return {
    id,
    program_name: "Adult Tennis",
    location: "MMSU Court 1",
    starts_at: `${date}T${String(startHour - 8).padStart(2, "0")}:00:00Z`,
    ends_at: `${date}T${String(endHour - 8).padStart(2, "0")}:00:00Z`,
    status: "open",
    capacity: 12,
    remaining_capacity: 4,
  };
}

test("allows adjacent slots across multiple Manila dates", () => {
  const sessions = [
    slot("a", "2026-11-01", 9, 10),
    slot("b", "2026-11-01", 10, 11),
    slot("c", "2026-11-08", 9, 10),
  ];
  assert.equal(validateBookingSelection(sessions.map((item) => item.id), sessions), null);
});

test("allows independent times on one date and different times across dates", () => {
  const adjacent = [slot("a", "2026-11-01", 9, 10), slot("b", "2026-11-01", 10, 11)];
  const separateTimes = [adjacent[0], slot("b", "2026-11-01", 13, 14)];
  assert.equal(validateBookingSelection(["a", "b"], separateTimes), null);
  const multipleDates = [adjacent[0], slot("b", "2026-11-08", 15, 16)];
  assert.equal(validateBookingSelection(["a", "b"], multipleDates), null);
});

test("requires one program and location per grouped request", () => {
  const adjacent = [slot("a", "2026-11-01", 9, 10), slot("b", "2026-11-01", 10, 11)];
  const differentProgram = [adjacent[0], { ...adjacent[1], program_name: "Junior Tennis" }];
  assert.match(validateBookingSelection(["a", "b"], differentProgram), /same training program and location/);
  const differentLocation = [adjacent[0], { ...adjacent[1], location: "MMSU Court 2" }];
  assert.match(validateBookingSelection(["a", "b"], differentLocation), /same training program and location/);
});

test("rejects duplicates, overlaps, unavailable slots, and excessive dates or slots", () => {
  const adjacent = [slot("a", "2026-11-01", 9, 10), slot("b", "2026-11-01", 10, 11)];
  assert.match(validateBookingSelection(["a", "a"], adjacent), /selected more than once/);
  const overlapping = [slot("a", "2026-11-01", 9, 11), slot("b", "2026-11-01", 10, 12)];
  assert.match(validateBookingSelection(["a", "b"], overlapping), /overlap/);
  const overnightConflict = [
    { ...adjacent[0], starts_at: "2026-10-31T15:00:00Z", ends_at: "2026-11-01T01:00:00Z" },
    { ...adjacent[1], starts_at: "2026-11-01T00:30:00Z", ends_at: "2026-11-01T01:30:00Z" },
  ];
  assert.match(validateBookingSelection(["a", "b"], overnightConflict), /overlap/);
  const differentProgram = [{ ...adjacent[0] }, { ...adjacent[1], program_name: "Junior Tennis" }];
  assert.match(validateBookingSelection(["a", "b"], differentProgram), /same training program and location/);
  const differentLocation = [{ ...adjacent[0] }, { ...adjacent[1], location: "MMSU Court 2" }];
  assert.match(validateBookingSelection(["a", "b"], differentLocation), /same training program and location/);
  const unavailable = [{ ...adjacent[0], remaining_capacity: 0 }, adjacent[1]];
  assert.match(validateBookingSelection(["a", "b"], unavailable), /no longer available/);

  const tooManyDates = Array.from({ length: MAX_BOOKING_DATES + 1 }, (_, index) =>
    slot(String(index), `2026-11-${String(index + 1).padStart(2, "0")}`, 9, 10),
  );
  assert.match(validateBookingSelection(tooManyDates.map((item) => item.id), tooManyDates), /no more than 8 dates/);

  const tooManySlots = Array.from({ length: MAX_BOOKING_SLOTS + 1 }, (_, index) =>
    slot(String(index), "2026-11-01", 8 + index, 9 + index),
  );
  assert.match(validateBookingSelection(tooManySlots.map((item) => item.id), tooManySlots), /no more than 16 session slots/);
});

test("formats the persisted submission timestamp in Manila time", () => {
  const formatted = formatSubmittedAt("2026-10-19T02:30:00.000Z");
  assert.match(formatted, /19/);
  assert.match(formatted, /10:30/);
  assert.equal(formatSubmittedAt("not-a-date"), "Unavailable");
});
