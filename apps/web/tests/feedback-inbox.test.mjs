import assert from "node:assert/strict";
import test from "node:test";
import {
  buildFeedbackCsv,
  escapeCsvValue,
  feedbackExportFilename,
  filterSubmissions,
  summarizeSubmissions,
} from "../src/lib/feedback-inbox.mjs";

function record(overrides = {}) {
  return {
    id: "id-1",
    category: "feedback",
    message: "Great sessions",
    name: "Ada",
    email: "ada@example.com",
    status: "new",
    created_at: "2026-10-18T02:00:00.000Z",
    reviewed_at: null,
    ...overrides,
  };
}

test("summary counts each status and a consistent total", () => {
  const summary = summarizeSubmissions([
    record({ status: "new" }),
    record({ id: "2", status: "reviewed" }),
    record({ id: "3", status: "archived" }),
    record({ id: "4", status: "reviewed" }),
  ]);

  assert.deepEqual(summary, {
    new: 1,
    reviewed: 2,
    archived: 1,
    total: 4,
  });
});

test("filters combine search, status, category, and date", () => {
  const submissions = [
    record({ id: "1", message: "More weekend slots", category: "scheduling", status: "new" }),
    record({ id: "2", message: "Court lights were dim", category: "facility", status: "reviewed" }),
    record({ id: "3", message: "Court booking was easy", category: "facility", status: "new", created_at: "2026-09-01T00:00:00.000Z" }),
  ];

  assert.equal(filterSubmissions(submissions, {}).length, 3);
  assert.deepEqual(
    filterSubmissions(submissions, { query: "court" }).map((row) => row.id),
    ["2", "3"],
  );
  assert.deepEqual(
    filterSubmissions(submissions, { status: "new", category: "scheduling" }).map((row) => row.id),
    ["1"],
  );
  assert.deepEqual(
    filterSubmissions(submissions, { dateFrom: "2026-10-01" }).map((row) => row.id),
    ["1", "2"],
  );
  assert.deepEqual(
    filterSubmissions(submissions, { query: "nonexistent" }),
    [],
  );
});

test("search matches submitter names as well as messages", () => {
  const submissions = [record({ id: "1", message: "no keyword", name: "Grace Hopper" })];
  assert.deepEqual(
    filterSubmissions(submissions, { query: "hopper" }).map((row) => row.id),
    ["1"],
  );
});

test("csv escapes commas, quotes, and newlines", () => {
  assert.equal(escapeCsvValue("plain"), "plain");
  assert.equal(escapeCsvValue("a,b"), '"a,b"');
  assert.equal(escapeCsvValue('say "hi"'), '"say ""hi"""');
  assert.equal(escapeCsvValue("line1\nline2"), '"line1\nline2"');
});

test("csv neutralizes spreadsheet formula injection", () => {
  assert.equal(escapeCsvValue("=1+1"), "'=1+1");
  assert.equal(escapeCsvValue("+SUM(A1:A2)"), "'+SUM(A1:A2)");
  assert.equal(escapeCsvValue("-2+3"), "'-2+3");
  assert.equal(escapeCsvValue("@cmd"), "'@cmd");
  // A comma forces quoting after the leading apostrophe is applied.
  assert.equal(escapeCsvValue("=a,b"), '"\'=a,b"');
});

test("exported csv includes a header, all rows, and excludes email by default", () => {
  const submissions = [
    record({ id: "1", message: "Hello, world" }),
    record({ id: "2", message: "=HYPERLINK(evil)", name: null }),
  ];
  const csv = buildFeedbackCsv(submissions);
  const lines = csv.split("\r\n");

  assert.equal(lines[0], "Submission ID,Date submitted,Category,Status,Message,Submitter name");
  assert.equal(lines.length, 3);
  assert.ok(csv.includes('"Hello, world"'));
  assert.ok(csv.includes("'=HYPERLINK(evil)"));
  assert.ok(!csv.includes("ada@example.com"));
  assert.ok(!csv.toLowerCase().includes("submitter email"));
});

test("email can be explicitly included in the export", () => {
  const csv = buildFeedbackCsv([record()], { includeEmail: true });
  assert.ok(csv.includes("Submitter email"));
  assert.ok(csv.includes("ada@example.com"));
});

test("export respects the active filter result set", () => {
  const submissions = [
    record({ id: "keep-me", category: "feedback", status: "new", message: "keep this" }),
    record({ id: "drop-me", category: "facility", status: "reviewed", message: "drop that" }),
  ];
  const filtered = filterSubmissions(submissions, { status: "new" });
  const csv = buildFeedbackCsv(filtered);
  assert.ok(csv.includes("keep-me"));
  assert.ok(csv.includes("keep this"));
  assert.ok(!csv.includes("drop-me"));
  assert.ok(!csv.includes("drop that"));
});

test("export filename carries the export date", () => {
  assert.equal(
    feedbackExportFilename(new Date("2026-10-19T08:00:00.000Z")),
    "mmsu-feedback-2026-10-19.csv",
  );
});
