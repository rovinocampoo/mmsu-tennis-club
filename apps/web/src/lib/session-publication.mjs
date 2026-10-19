// Publication helpers for the admin bulk-publish flow. All calendar boundaries
// are expressed as Asia/Manila (PHT) date keys so a session that starts late in
// the UTC day is attributed to the correct Manila day, matching how the admin
// calendar groups sessions.

const phtDate = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Manila",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

export function phtDateKey(value) {
  return phtDate.format(new Date(value));
}

// publicationWindow returns the half-open [start, end) date-key range for a
// Manila day or month. The month is derived from the date's year and month.
export function publicationWindow(scope, date) {
  const [year, month, day] = date.split("-").map(Number);

  if (scope === "month") {
    const start = `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-01`;
    const end =
      month === 12
        ? `${String(year + 1).padStart(4, "0")}-01-01`
        : `${String(year).padStart(4, "0")}-${String(month + 1).padStart(2, "0")}-01`;
    return { start, end };
  }

  const next = new Date(Date.UTC(year, month - 1, day + 1));
  return { start: date, end: next.toISOString().slice(0, 10) };
}

// eligibleUnpublishedSessions mirrors the SQL eligibility rule: a session is
// publishable only when it is hidden and still open for registration. This is a
// preview for admins; the database re-checks the same rule on confirm.
export function eligibleUnpublishedSessions(sessions, scope, date) {
  const { start, end } = publicationWindow(scope, date);
  return (sessions || []).filter((session) => {
    if (session.is_public || session.status !== "open") return false;
    const key = phtDateKey(session.starts_at);
    return key >= start && key < end;
  });
}

// skippedUnpublishedSessions reports hidden sessions in range that will be left
// alone because they are closed or cancelled, so the preview can explain the
// difference between "eligible" and "everything hidden in this period".
export function skippedUnpublishedSessions(sessions, scope, date) {
  const { start, end } = publicationWindow(scope, date);
  return (sessions || []).filter((session) => {
    if (session.is_public || session.status === "open") return false;
    const key = phtDateKey(session.starts_at);
    return key >= start && key < end;
  });
}
