const DATE_KEY_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

export function phtDateKey(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Manila",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

export function periodRange(period, anchor) {
  if (!DATE_KEY_PATTERN.test(String(anchor ?? ""))) return null;
  const [year, month, day] = anchor.split("-").map(Number);
  const start = new Date(Date.UTC(year, month - 1, day));
  if (Number.isNaN(start.getTime())) return null;
  if (period === "month") {
    start.setUTCDate(1);
  } else {
    start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
  }
  const end = new Date(start);
  if (period === "month") end.setUTCMonth(end.getUTCMonth() + 1);
  else end.setUTCDate(end.getUTCDate() + 7);
  const startKey = start.toISOString().slice(0, 10);
  const endKey = end.toISOString().slice(0, 10);
  const label = period === "month"
    ? start.toLocaleDateString("en-PH", { timeZone: "UTC", month: "long", year: "numeric" })
    : `${start.toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric" })} – ${new Date(end.getTime() - 86400000).toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric", year: "numeric" })}`;
  return { start, end, startKey, endKey, label };
}

export function shiftPeriod(period, anchor, direction) {
  const range = periodRange(period, anchor);
  if (!range) return "";
  const next = new Date(range.start);
  if (period === "month") next.setUTCMonth(next.getUTCMonth() + direction);
  else next.setUTCDate(next.getUTCDate() + 7 * direction);
  return next.toISOString().slice(0, 10);
}

export function calendarGrid(year, month) {
  if (
    !Number.isInteger(year) ||
    !Number.isInteger(month) ||
    month < 1 ||
    month > 12
  ) {
    return [];
  }
  const start = new Date(Date.UTC(year, month - 1, 1));
  start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
  const monthKey = `${year}-${String(month).padStart(2, "0")}`;
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(start);
    date.setUTCDate(start.getUTCDate() + index);
    const key = date.toISOString().slice(0, 10);
    return {
      key,
      day: date.getUTCDate(),
      inMonth: key.slice(0, 7) === monthKey,
    };
  });
}

export function sessionInRange(session, range) {
  if (!range) return true;
  const key = phtDateKey(session?.starts_at);
  if (!key) return false;
  return key >= range.startKey && key < range.endKey;
}

export function sessionsInRange(sessions, range) {
  const list = Array.isArray(sessions) ? sessions : [];
  return sortSessionsChronologically(list.filter((session) => sessionInRange(session, range)));
}

export function sortSessionsChronologically(sessions) {
  const list = Array.isArray(sessions) ? [...sessions] : [];
  return list.sort((a, b) => {
    const left = Date.parse(a?.starts_at ?? "");
    const right = Date.parse(b?.starts_at ?? "");
    if (Number.isNaN(left) && Number.isNaN(right)) return 0;
    if (Number.isNaN(left)) return 1;
    if (Number.isNaN(right)) return -1;
    return left - right;
  });
}

export function summarizeSchedule(sessions, now = new Date()) {
  const list = Array.isArray(sessions) ? sessions : [];
  const nowMs = now instanceof Date ? now.getTime() : Date.parse(now);
  let upcoming = 0;
  let assigned = 0;
  let unassigned = 0;
  let participantCount = 0;
  for (const session of list) {
    const start = Date.parse(session?.starts_at ?? "");
    if (Number.isFinite(start) && start >= nowMs) upcoming += 1;
    if (session?.assigned_coach_id) assigned += 1;
    else unassigned += 1;
    if (Array.isArray(session?.participant_names)) participantCount += session.participant_names.length;
  }
  return { total: list.length, upcoming, assigned, unassigned, participantCount };
}

export function rosterLabel(session) {
  const names = Array.isArray(session?.participant_names) ? session.participant_names.filter(Boolean) : [];
  if (names.length === 0) return "No confirmed participants yet";
  return names.join(", ");
}
