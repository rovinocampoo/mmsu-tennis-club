export const FEEDBACK_STATUSES = ["new", "reviewed", "archived"];

export const FEEDBACK_CATEGORIES = [
  "suggestion",
  "feedback",
  "facility",
  "training",
  "scheduling",
  "other",
];

export const FEEDBACK_CATEGORY_LABELS = {
  suggestion: "Suggestion",
  feedback: "Feedback",
  facility: "Facility",
  training: "Training",
  scheduling: "Scheduling",
  other: "Other",
};

const isoDate = (value) => (typeof value === "string" ? value.slice(0, 10) : "");

export function summarizeSubmissions(submissions) {
  const summary = { new: 0, reviewed: 0, archived: 0, total: 0 };
  for (const submission of submissions ?? []) {
    summary.total += 1;
    if (FEEDBACK_STATUSES.includes(submission.status)) {
      summary[submission.status] += 1;
    }
  }
  return summary;
}

// Filters are combined (AND). `dateKeyOf` maps a record to a YYYY-MM-DD key so
// the inbox can filter by the same local date it displays (Asia/Manila).
export function filterSubmissions(
  submissions,
  { query = "", status = "", category = "", dateFrom = "", dateTo = "" } = {},
  dateKeyOf = (submission) => isoDate(submission.created_at),
) {
  const needle = query.trim().toLowerCase();
  return (submissions ?? []).filter((submission) => {
    if (status && submission.status !== status) return false;
    if (category && submission.category !== category) return false;
    if (dateFrom || dateTo) {
      const key = dateKeyOf(submission);
      if (dateFrom && key < dateFrom) return false;
      if (dateTo && key > dateTo) return false;
    }
    if (needle) {
      const haystack = `${submission.message ?? ""} ${submission.name ?? ""}`
        .toLowerCase();
      if (!haystack.includes(needle)) return false;
    }
    return true;
  });
}

// Protect spreadsheets from formula injection, then quote for CSV safety.
export function escapeCsvValue(value) {
  let text = value === null || value === undefined ? "" : String(value);
  if (/^[=+\-@\t\r]/.test(text)) text = `'${text}`;
  if (/[",\r\n]/.test(text)) text = `"${text.replace(/"/g, '""')}"`;
  return text;
}

export function toCsv(columns, rows) {
  const header = columns.map((column) => escapeCsvValue(column.label)).join(",");
  const lines = rows.map((row) =>
    columns.map((column) => escapeCsvValue(column.value(row))).join(","),
  );
  return [header, ...lines].join("\r\n");
}

export function buildFeedbackCsv(submissions, { includeEmail = false } = {}) {
  const columns = [
    { label: "Submission ID", value: (row) => row.id },
    { label: "Date submitted", value: (row) => row.created_at },
    { label: "Category", value: (row) => row.category },
    { label: "Status", value: (row) => row.status },
    { label: "Message", value: (row) => row.message },
    { label: "Submitter name", value: (row) => row.name ?? "" },
  ];
  if (includeEmail) {
    columns.push({ label: "Submitter email", value: (row) => row.email ?? "" });
  }
  return toCsv(columns, submissions ?? []);
}

export function feedbackExportFilename(date = new Date()) {
  return `mmsu-feedback-${date.toISOString().slice(0, 10)}.csv`;
}
