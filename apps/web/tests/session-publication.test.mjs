import assert from "node:assert/strict";
import test from "node:test";
import {
  eligibleUnpublishedSessions,
  phtDateKey,
  publicationWindow,
  skippedUnpublishedSessions,
} from "../src/lib/session-publication.mjs";

test("publicationWindow covers a Manila day as a half-open range", () => {
  assert.deepEqual(publicationWindow("day", "2026-10-19"), {
    start: "2026-10-19",
    end: "2026-10-20",
  });
  assert.deepEqual(publicationWindow("day", "2026-10-31"), {
    start: "2026-10-31",
    end: "2026-11-01",
  });
});

test("publicationWindow covers a Manila month with a year rollover", () => {
  assert.deepEqual(publicationWindow("month", "2026-10-15"), {
    start: "2026-10-01",
    end: "2026-11-01",
  });
  assert.deepEqual(publicationWindow("month", "2026-12-05"), {
    start: "2026-12-01",
    end: "2027-01-01",
  });
});

test("phtDateKey attributes late-UTC sessions to the next Manila day", () => {
  assert.equal(phtDateKey("2026-10-19T16:30:00Z"), "2026-10-20");
  assert.equal(phtDateKey("2026-10-19T15:59:00Z"), "2026-10-19");
});

const sessions = [
  {
    id: "hidden-open-in-range",
    status: "open",
    is_public: false,
    starts_at: "2026-10-19T01:00:00Z",
  },
  {
    id: "published-open-in-range",
    status: "open",
    is_public: true,
    starts_at: "2026-10-19T02:00:00Z",
  },
  {
    id: "hidden-closed-in-range",
    status: "closed",
    is_public: false,
    starts_at: "2026-10-19T03:00:00Z",
  },
  {
    id: "hidden-cancelled-in-range",
    status: "cancelled",
    is_public: false,
    starts_at: "2026-10-19T04:00:00Z",
  },
  {
    id: "hidden-open-next-day",
    status: "open",
    is_public: false,
    starts_at: "2026-10-20T04:00:00Z",
  },
  {
    id: "hidden-open-late-utc",
    status: "open",
    is_public: false,
    starts_at: "2026-10-19T16:30:00Z",
  },
];

test("eligibleUnpublishedSessions keeps only hidden open sessions in range", () => {
  const eligible = eligibleUnpublishedSessions(sessions, "day", "2026-10-19");
  assert.deepEqual(
    eligible.map((session) => session.id),
    ["hidden-open-in-range"],
  );
});

test("eligibleUnpublishedSessions spans the whole month window", () => {
  const eligible = eligibleUnpublishedSessions(sessions, "month", "2026-10-01");
  assert.deepEqual(
    eligible.map((session) => session.id),
    ["hidden-open-in-range", "hidden-open-next-day", "hidden-open-late-utc"],
  );
});

test("skippedUnpublishedSessions reports hidden closed and cancelled sessions", () => {
  const skipped = skippedUnpublishedSessions(sessions, "day", "2026-10-19");
  assert.deepEqual(
    skipped.map((session) => session.id),
    ["hidden-closed-in-range", "hidden-cancelled-in-range"],
  );
});
