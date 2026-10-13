const DATE_KEY_PATTERN = /^\d{4}-\d{2}-\d{2}$/;
const TIME_PATTERN = /^(?:[01]\d|2[0-3]):[0-5]\d$/;

function parseDateKey(value) {
  if (typeof value !== "string" || !DATE_KEY_PATTERN.test(value)) return null;
  const date = new Date(`${value}T12:00:00Z`);
  return Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== value ? null : date;
}

export function addDateKey(dateKey, days) {
  const date = parseDateKey(dateKey);
  if (!date || !Number.isInteger(days)) return "";
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

export function getDateRangeError(startDate, endDate, maxDays = 62) {
  const start = parseDateKey(startDate);
  const end = parseDateKey(endDate);
  if (!start || !end) return "Choose a valid start and end date.";
  if (startDate > endDate) return "The end date must be on or after the start date.";
  const days = Math.floor((Date.parse(`${endDate}T00:00:00Z`) - Date.parse(`${startDate}T00:00:00Z`)) / 86400000) + 1;
  if (days > maxDays) return `Choose a date range of ${maxDays} days or fewer.`;
  return "";
}

export function getTimeRangeError(startTime, endTime) {
  if (!TIME_PATTERN.test(startTime) || !TIME_PATTERN.test(endTime)) {
    return "Choose a valid start and end time.";
  }
  const toMinutes = (value) => Number(value.slice(0, 2)) * 60 + Number(value.slice(3, 5));
  if (toMinutes(endTime) <= toMinutes(startTime)) {
    return "The end time must be later than the start time. Overnight sessions are not supported in recurring schedules.";
  }
  return "";
}

export function getOccurrenceDates({ startDate, endDate, frequency, weekdays = [], maxDays = 62 }) {
  if (getDateRangeError(startDate, endDate, maxDays)) return [];
  const pickedDays = new Set(weekdays);
  const result = [];
  const end = parseDateKey(endDate);
  let cursor = parseDateKey(startDate);
  while (cursor && cursor <= end) {
    const key = cursor.toISOString().slice(0, 10);
    const day = cursor.getUTCDay();
    const include = frequency === "daily"
      || (frequency === "weekdays" && day >= 1 && day <= 5)
      || (frequency === "selected" && pickedDays.has(day));
    if (include) result.push(key);
    cursor = new Date(cursor);
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }
  return result;
}

export function toPHTISO(dateKey, time) {
  if (!parseDateKey(dateKey) || !TIME_PATTERN.test(time)) return "";
  const date = new Date(`${dateKey}T${time}:00+08:00`);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

export function phtTimeInput(isoDateTime) {
  const date = new Date(isoDateTime);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Manila",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date);
}

export function findScheduleConflicts({ startsAt, endsAt, location, coachID = "", sessions = [] }) {
  const start = Date.parse(startsAt);
  const end = Date.parse(endsAt);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return [];
  const venue = String(location ?? "").trim().toLocaleLowerCase();

  return sessions.flatMap((session) => {
    if (session.status === "cancelled") return [];
    const existingStart = Date.parse(session.starts_at);
    const existingEnd = Date.parse(session.ends_at);
    if (!Number.isFinite(existingStart) || !Number.isFinite(existingEnd)) return [];
    if (start >= existingEnd || end <= existingStart) return [];

    const sameVenue = venue !== "" && venue === String(session.location ?? "").trim().toLocaleLowerCase();
    const sameCoach = Boolean(coachID) && session.assigned_coach_id === coachID;
    if (!sameVenue && !sameCoach) return [];

    const reasons = [];
    if (sameVenue) reasons.push("same venue");
    if (sameCoach) reasons.push("same assigned coach");
    return [{
      sessionId: session.id,
      programName: session.program_name,
      location: session.location,
      startsAt: session.starts_at,
      endsAt: session.ends_at,
      reason: reasons.join(" and "),
    }];
  });
}
