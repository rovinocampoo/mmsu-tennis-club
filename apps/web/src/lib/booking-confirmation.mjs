const MANILA_TIME_ZONE = "Asia/Manila";
const manilaParts = new Intl.DateTimeFormat("en-CA", { timeZone: MANILA_TIME_ZONE, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" });
function dateParts(value) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) throw new TypeError("A valid session date is required.");
  return Object.fromEntries(manilaParts.formatToParts(date).map(function (part) { return [part.type, part.value]; }));
}
function assertApproved(status) {
  if (!status || status.status !== "approved") throw new TypeError("Only approved bookings can be added to a calendar.");
  if (!status.starts_at || !status.ends_at || !status.location) throw new TypeError("Complete booking details are required.");
}
function googleDate(value) {
  const p = dateParts(value);
  return p.year + p.month + p.day + "T" + p.hour + p.minute + p.second;
}
function icsDate(value) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) throw new TypeError("A valid session date is required.");
  return date.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
}
function icsEscape(value) {
  return String(value == null ? "" : value).replace(/\\/g, "\\\\").replace(/\r?\n/g, "\\n").replace(/,/g, "\\,").replace(/;/g, "\\;");
}
function requiredURL(value) {
  const url = new URL(value);
  if (url.protocol !== "https:" && url.protocol !== "http:") throw new TypeError("A valid status page URL is required.");
  return url.toString();
}

export function createGoogleCalendarURL(status) {
  assertApproved(status);
  const url = new URL("https://calendar.google.com/calendar/render");
  url.searchParams.set("action", "TEMPLATE");
  url.searchParams.set("text", "MMSU Tennis Club — " + status.program_name);
  url.searchParams.set("dates", googleDate(status.starts_at) + "/" + googleDate(status.ends_at));
  url.searchParams.set("ctz", MANILA_TIME_ZONE);
  url.searchParams.set("details", "Booking confirmed by MMSU Tennis Club.\nStatus: Approved\nTime zone: Philippine Time (Asia/Manila).");
  url.searchParams.set("location", status.location);
  return url.toString();
}

export function createCalendarFile(status) {
  assertApproved(status);
  const uidSeed = String(status.request_reference || "booking").replace(/[^A-Za-z0-9._-]/g, "-");
  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//MMSU Tennis Club//Booking Confirmation//EN",
    "CALSCALE:GREGORIAN",
    "METHOD:PUBLISH",
    "BEGIN:VEVENT",
    "UID:" + uidSeed + "@mmsu-tennis-club",
    "DTSTAMP:" + icsDate(new Date().toISOString()),
    "DTSTART:" + icsDate(status.starts_at),
    "DTEND:" + icsDate(status.ends_at),
    "SUMMARY:" + icsEscape("MMSU Tennis Club — " + status.program_name),
    "LOCATION:" + icsEscape(status.location),
    "DESCRIPTION:" + icsEscape("Confirmed booking from MMSU Tennis Club. Times are in Philippine Time (Asia/Manila)."),
    "STATUS:CONFIRMED",
    "END:VEVENT",
    "END:VCALENDAR",
    ""
  ];
  return lines.join("\r\n");
}

export function createBookingMessage(status, statusPageURL) {
  const pageURL = requiredURL(statusPageURL);
  const startsAt = new Date(status.starts_at);
  const endsAt = new Date(status.ends_at);
  if (!Number.isFinite(startsAt.getTime()) || !Number.isFinite(endsAt.getTime())) throw new TypeError("A valid session date is required.");
  const timeFmt = new Intl.DateTimeFormat("en-PH", { timeZone: MANILA_TIME_ZONE, hour: "numeric", minute: "2-digit", hour12: true });
  const dateFmt = new Intl.DateTimeFormat("en-PH", { timeZone: MANILA_TIME_ZONE, weekday: "short", month: "short", day: "numeric", year: "numeric" });
  const schedule = dateFmt.format(startsAt) + "\n" + timeFmt.format(startsAt) + "–" + timeFmt.format(endsAt) + " PHT\n" + status.location;

  if (status.status === "approved") {
    return [
      "🎾 MMSU TENNIS CLUB — BOOKING CONFIRMED",
      "",
      "Your " + status.program_name + " booking has been approved!",
      "",
      "📅 " + dateFmt.format(startsAt),
      "🕒 " + timeFmt.format(startsAt) + "–" + timeFmt.format(endsAt) + " PHT",
      "📍 " + status.location,
      "Reference: " + status.request_reference,
      "",
      "➕ Add to Google Calendar: " + createGoogleCalendarURL(status),
      "Booking details and latest updates: " + pageURL,
      "",
      "See you on court! 🎾"
    ].join("\n");
  }

  if (status.status === "rejected") {
    return [
      "🎾 MMSU TENNIS CLUB — BOOKING UPDATE",
      "",
      "Your request for " + status.program_name + " was not approved.",
      "Requested schedule:\n" + schedule,
      "Reference: " + status.request_reference,
      "",
      "View the latest update: " + pageURL,
      "Contact the club if you need help finding another session."
    ].join("\n");
  }

  if (status.status === "partially_decided") {
    return [
      "🎾 MMSU TENNIS CLUB — BOOKING UPDATE",
      "",
      "Some sessions in your request were approved and others were not.",
      "Review each session's decision here: " + pageURL,
      "Reference: " + status.request_reference,
    ].join("\n");
  }

  return [
    "🎾 MMSU TENNIS CLUB — REQUEST RECEIVED",
    "",
    "Your request for " + status.program_name + " is awaiting admin approval. This is not a confirmed booking.",
    "Requested schedule:\n" + schedule,
    "Reference: " + status.request_reference,
    "",
    "Check for updates here: " + pageURL
  ].join("\n");
}

export function createAdminApprovalMessage(request) {
  const sessions = Array.isArray(request?.sessions) && request.sessions.length > 0 ? request.sessions : [request];
  if (!request || sessions.some((session) => !session?.ends_at)) throw new TypeError("A confirmed session end time is required.");
  const name = String(request.full_name || "there").replace(/[\r\n]+/g, " ").trim() || "there";
  const dateFmt = new Intl.DateTimeFormat("en-PH", { timeZone: MANILA_TIME_ZONE, weekday: "short", month: "short", day: "numeric", year: "numeric" });
  const timeFmt = new Intl.DateTimeFormat("en-PH", { timeZone: MANILA_TIME_ZONE, hour: "numeric", minute: "2-digit", hour12: true });
  const schedule = sessions.map((session) => {
    const startsAt = new Date(session.starts_at);
    const endsAt = new Date(session.ends_at);
    if (!Number.isFinite(startsAt.getTime()) || !Number.isFinite(endsAt.getTime())) throw new TypeError("A valid session date is required.");
    return [
      "📅 " + dateFmt.format(startsAt),
      "🕒 " + timeFmt.format(startsAt) + "–" + timeFmt.format(endsAt) + " PHT",
      "📍 " + session.location
    ].join("\n");
  });
  return [
    "🎾 MMSU TENNIS CLUB — BOOKING CONFIRMED",
    "",
    "Hi " + name + ",",
    "Your tennis training booking has been approved!",
    "",
    schedule.join("\n\n"),
    "",
    "Visit your private booking status page to add this booking to your calendar and check for updates.",
    "",
    "See you on court! 🎾"
  ].join("\n");
}

export function getCalendarFilename(reference) {
  return "mmsu-tennis-booking-" + String(reference || "booking").replace(/[^A-Za-z0-9_-]/g, "-") + ".ics";
}
