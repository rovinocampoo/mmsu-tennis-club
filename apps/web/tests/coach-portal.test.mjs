import assert from "node:assert/strict";
import test from "node:test";
import {
  calendarGrid,
  phtDateKey,
  periodRange,
  rosterLabel,
  sessionInRange,
  sessionsInRange,
  shiftPeriod,
  sortSessionsChronologically,
  summarizeSchedule,
} from "../src/lib/coach-portal.mjs";

const session = (id, startsAt, assignedCoachID = null, participants = []) => ({
  id,
  program_name: "Clinic",
  program_description: null,
  starts_at: startsAt,
  ends_at: startsAt,
  location: "Court 1",
  status: "open",
  is_public: true,
  assigned_coach_id: assignedCoachID,
  assigned_coach_name: assignedCoachID ? "Coach" : null,
  participant_names: participants,
  confirmed_booking_count: participants.length,
});

test("phtDateKey formats Manila calendar dates and rejects invalid input", () => {
  assert.equal(phtDateKey("2026-10-19T02:00:00Z"), "2026-10-19");
  assert.equal(phtDateKey("2026-10-18T16:30:00Z"), "2026-10-19");
  assert.equal(phtDateKey("not-a-date"), "");
});

test("periodRange builds Monday-based weeks and calendar months", () => {
  const week = periodRange("week", "2026-10-21");
  assert.equal(week.startKey, "2026-10-19");
  assert.equal(week.endKey, "2026-10-26");

  const month = periodRange("month", "2026-10-21");
  assert.equal(month.startKey, "2026-10-01");
  assert.equal(month.endKey, "2026-11-01");
  assert.equal(periodRange("week", "bad"), null);
});

test("shiftPeriod moves the anchor by the selected period", () => {
  assert.equal(shiftPeriod("week", "2026-10-21", 1), "2026-10-26");
  assert.equal(shiftPeriod("month", "2026-10-21", -1), "2026-09-01");
});

test("sessionsInRange keeps only sessions inside the range, sorted chronologically", () => {
  const range = periodRange("week", "2026-10-21");
  const sessions = [
    session("b", "2026-10-21T02:00:00Z"),
    session("a", "2026-10-19T02:00:00Z"),
    session("out", "2026-11-01T02:00:00Z"),
    session("invalid", "broken"),
  ];
  const result = sessionsInRange(sessions, range);
  assert.deepEqual(result.map((item) => item.id), ["a", "b"]);
  assert.equal(sessionInRange(sessions[2], range), false);
  assert.equal(sessionInRange(sessions[3], range), false);
});

test("sortSessionsChronologically is stable against invalid dates", () => {
  const sorted = sortSessionsChronologically([
    session("late", "2026-10-22T02:00:00Z"),
    session("bad", "broken"),
    session("early", "2026-10-20T02:00:00Z"),
  ]);
  assert.deepEqual(sorted.map((item) => item.id), ["early", "late", "bad"]);
});

test("summarizeSchedule counts upcoming, assignment coverage, and participants", () => {
  const now = new Date("2026-10-20T00:00:00Z");
  const summary = summarizeSchedule([
    session("past", "2026-10-19T02:00:00Z", "11111111-1111-4111-8111-111111111111", ["Alex", "Bea"]),
    session("soon", "2026-10-21T02:00:00Z"),
    session("later", "2026-10-25T02:00:00Z", "22222222-2222-4222-8222-222222222222", ["Cara"]),
  ], now);
  assert.deepEqual(summary, { total: 3, upcoming: 2, assigned: 2, unassigned: 1, participantCount: 3 });
});

test("calendarGrid builds a Monday-based 42-cell month grid", () => {
  const grid = calendarGrid(2026, 10);
  assert.equal(grid.length, 42);
  assert.deepEqual(grid[0], { key: "2026-09-28", day: 28, inMonth: false });
  assert.deepEqual(grid[3], { key: "2026-10-01", day: 1, inMonth: true });
  assert.equal(grid[0].inMonth, false);
  assert.deepEqual(calendarGrid(2026, 13), []);
  assert.deepEqual(calendarGrid(2026, 1.5), []);
});

test("rosterLabel joins names and falls back when empty", () => {
  assert.equal(rosterLabel(session("x", "2026-10-19T02:00:00Z", null, ["Alex", "Bea"])), "Alex, Bea");
  assert.equal(rosterLabel(session("y", "2026-10-19T02:00:00Z")), "No confirmed participants yet");
});
