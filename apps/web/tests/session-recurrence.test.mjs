import test from "node:test";
import assert from "node:assert/strict";
import {
  addDateKey,
  findScheduleConflicts,
  getDateRangeError,
  getOccurrenceDates,
  getTimeRangeError,
  phtTimeInput,
  toPHTISO,
} from "../src/lib/session-recurrence.mjs";

test("generates daily, weekday, and selected weekday occurrences inclusively", () => {
  assert.deepEqual(getOccurrenceDates({ startDate: "2026-10-19", endDate: "2026-10-25", frequency: "daily" }), [
    "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23", "2026-10-24", "2026-10-25",
  ]);
  assert.deepEqual(getOccurrenceDates({ startDate: "2026-10-19", endDate: "2026-10-25", frequency: "weekdays" }), [
    "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23",
  ]);
  assert.deepEqual(getOccurrenceDates({ startDate: "2026-10-19", endDate: "2026-10-25", frequency: "selected", weekdays: [1, 4, 6] }), [
    "2026-10-19", "2026-10-22", "2026-10-24",
  ]);
});

test("validates inclusive recurrence range and same-day times", () => {
  assert.equal(getDateRangeError("2026-10-20", "2026-10-19"), "The end date must be on or after the start date.");
  assert.match(getDateRangeError("2026-10-01", "2026-12-15"), /62 days/);
  assert.equal(getDateRangeError("2026-10-01", "2026-11-01"), "");
  assert.match(getTimeRangeError("10:00", "09:00"), /later than the start time/);
  assert.equal(getTimeRangeError("07:00", "08:30"), "");
});

test("builds Manila timestamps and converts a session time back to PHT input", () => {
  assert.equal(toPHTISO("2026-10-22", "10:00"), "2026-10-22T02:00:00.000Z");
  assert.equal(phtTimeInput("2026-10-22T02:00:00.000Z"), "10:00");
  assert.equal(addDateKey("2026-10-22", 7), "2026-10-29");
});

test("conflict detection blocks overlapping same-venue or assigned-coach sessions but ignores cancelled sessions", () => {
  const base = {
    id: "s1",
    program_name: "Junior Tennis",
    starts_at: "2026-10-22T02:00:00.000Z",
    ends_at: "2026-10-22T03:00:00.000Z",
    location: "MMSU Court 1",
    status: "open",
    assigned_coach_id: "coach-1",
  };
  assert.equal(findScheduleConflicts({ startsAt: "2026-10-22T02:30:00.000Z", endsAt: "2026-10-22T03:30:00.000Z", location: "mmsu court 1", sessions: [base] }).length, 1);
  assert.equal(findScheduleConflicts({ startsAt: "2026-10-22T02:30:00.000Z", endsAt: "2026-10-22T03:30:00.000Z", location: "MMSU Court 2", coachID: "coach-1", sessions: [base] }).length, 1);
  assert.equal(findScheduleConflicts({ startsAt: "2026-10-22T02:30:00.000Z", endsAt: "2026-10-22T03:30:00.000Z", location: "MMSU Court 2", sessions: [{ ...base, status: "cancelled" }] }).length, 0);
  assert.equal(findScheduleConflicts({ startsAt: "2026-10-22T03:00:00.000Z", endsAt: "2026-10-22T04:00:00.000Z", location: "MMSU Court 1", sessions: [base] }).length, 0);
});
