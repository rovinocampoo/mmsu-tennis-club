import assert from "node:assert/strict";
import test from "node:test";
import { createAdminApprovalMessage, createBookingMessage, createCalendarFile, createGoogleCalendarURL, getCalendarFilename } from "../src/lib/booking-confirmation.mjs";

const approved = { request_reference: "MMSU-26-104", status: "approved", program_name: "Tennis Fundamentals", starts_at: "2026-10-20T21:30:00Z", ends_at: "2026-10-20T23:00:00Z", location: "MMSU Tennis Court", next_steps: ["Arrive on time."] };
const statusURL = "https://mmsu-tennis-club.vercel.app/booking-status#opaqueToken";

test("Google Calendar link uses Manila wall-clock time and never exposes the private status token", () => {
  const url = new URL(createGoogleCalendarURL(approved));
  assert.equal(url.origin, "https://calendar.google.com");
  assert.equal(url.searchParams.get("ctz"), "Asia/Manila");
  assert.equal(url.searchParams.get("dates"), "20261021T053000/20261021T070000");
  assert.match(url.searchParams.get("text"), /Tennis Fundamentals/);
  assert.match(url.searchParams.get("details"), /Booking confirmed/);
  assert.equal(url.searchParams.get("location"), "MMSU Tennis Court");
  assert.doesNotMatch(url.toString(), /opaqueToken/);
});

test("calendar file is a confirmed event with UTC timestamps and CRLF line endings", () => {
  const file = createCalendarFile(approved);
  assert.match(file, /BEGIN:VCALENDAR\r\n/);
  assert.match(file, /BEGIN:VEVENT\r\n/);
  assert.match(file, /DTSTART:20261020T213000Z/);
  assert.match(file, /DTEND:20261020T230000Z/);
  assert.match(file, /STATUS:CONFIRMED/);
  assert.match(file, /END:VCALENDAR\r\n$/);
});

test("pending and rejected requester messages do not offer unapproved calendar events", () => {
  const pending = createBookingMessage({ ...approved, status: "pending" }, statusURL);
  assert.match(pending, /awaiting admin approval/);
  assert.match(pending, /not a confirmed booking/);
  assert.doesNotMatch(pending, /Add to Google Calendar/);
  assert.match(pending, /booking-status#opaqueToken/);
  const confirmed = createBookingMessage(approved, statusURL);
  assert.match(confirmed, /BOOKING CONFIRMED/);
  assert.match(confirmed, /Add to Google Calendar: https:\/\/calendar\.google\.com/);
  assert.match(confirmed, /booking-status#opaqueToken/);
  const calendarLink = confirmed.split("Add to Google Calendar: ")[1].split("\n")[0];
  assert.doesNotMatch(calendarLink, /opaqueToken/);
  const rejected = createBookingMessage({ ...approved, status: "rejected" }, statusURL);
  assert.match(rejected, /was not approved/);
  assert.doesNotMatch(rejected, /Add to Google Calendar/);
});

test("admin approval copy contains schedule details and an add-to-calendar link", () => {
  const message = createAdminApprovalMessage({ full_name: "Rovin Ocampo", starts_at: approved.starts_at, ends_at: approved.ends_at, location: approved.location });
  assert.match(message, /BOOKING CONFIRMED/);
  assert.match(message, /Hi Rovin Ocampo/);
  assert.match(message, /Oct 21, 2026/);
  assert.match(message, /5:30 AM–7:00 AM PHT/);
  assert.match(message, /Visit your private booking status page to add this booking to your calendar and check for updates/);
  assert.doesNotMatch(message, /calendar\.google\.com\/calendar\/render/);
  assert.match(message, /MMSU Tennis Court/);
  assert.doesNotMatch(message, /https:\/\/calendar\.google\.com/);
  assert.doesNotMatch(message, /opaqueToken/);
});

test("admin approval copy lists every session approved in one action", () => {
  const message = createAdminApprovalMessage({
    full_name: "Rovin Ocampo",
    sessions: [
      { starts_at: "2026-10-20T21:30:00Z", ends_at: "2026-10-20T23:00:00Z", location: "MMSU Tennis Court 1" },
      { starts_at: "2026-10-27T23:00:00Z", ends_at: "2026-10-28T00:00:00Z", location: "MMSU Tennis Court 2" },
    ],
  });
  assert.match(message, /^🎾 MMSU TENNIS CLUB — BOOKING CONFIRMED/);
  assert.match(message, /Hi Rovin Ocampo,/);
  assert.match(message, /Wed, Oct 21, 2026[\s\S]*5:30 AM–7:00 AM PHT[\s\S]*MMSU Tennis Court 1/);
  assert.match(message, /Wed, Oct 28, 2026[\s\S]*7:00 AM–8:00 AM PHT[\s\S]*MMSU Tennis Court 2/);
});

test("calendar actions require approval and filenames are sanitized", () => {
  assert.throws(() => createGoogleCalendarURL({ ...approved, status: "pending" }), /Only approved/);
  assert.throws(() => createCalendarFile({ ...approved, status: "pending" }), /Only approved/);
  assert.equal(getCalendarFilename("MMSU/26 104"), "mmsu-tennis-booking-MMSU-26-104.ics");
});

test("admin confirmation message tells requester to visit status page instead of exposing long calendar URL", () => {
  const message = createAdminApprovalMessage({ full_name: "Player Example", starts_at: approved.starts_at, ends_at: approved.ends_at, location: approved.location });
  assert.match(message, /Visit your private booking status page to add this booking to your calendar/);
  assert.doesNotMatch(message, /https:\/\/calendar\.google\.com/);
  assert.doesNotMatch(message, /https:\/\/mmsu-tennis-club\.vercel\.app\/booking-status/);
});

test("multi-date calendar exports include only individually approved sessions", async () => {
  const { createGoogleCalendarURLs } = await import("../src/lib/booking-confirmation.mjs");
  const multi = { ...approved, status: "partially_decided", requested_sessions: [
    { session_id: "session-a", program_name: "Tennis Fundamentals", starts_at: "2026-10-20T21:30:00Z", ends_at: "2026-10-20T23:00:00Z", location: "Court 1", status: "approved" },
    { session_id: "session-b", program_name: "Tennis Fundamentals", starts_at: "2026-10-27T21:30:00Z", ends_at: "2026-10-27T23:00:00Z", location: "Court 2", status: "pending" },
    { session_id: "session-c", program_name: "Tennis Fundamentals", starts_at: "2026-11-03T21:30:00Z", ends_at: "2026-11-03T23:00:00Z", location: "Court 3", status: "approved" },
    { session_id: "session-d", program_name: "Tennis Fundamentals", starts_at: "2026-11-10T21:30:00Z", ends_at: "2026-11-10T23:00:00Z", location: "Court 4", status: "rejected" },
  ] };
  const links = createGoogleCalendarURLs(multi);
  assert.equal(links.length, 2);
  assert.deepEqual(links.map((entry) => entry.session_id), ["session-a", "session-c"]);
  assert.equal(new URL(links[0].url).searchParams.get("location"), "Court 1");
  assert.equal(new URL(links[1].url).searchParams.get("location"), "Court 3");
  const file = createCalendarFile(multi);
  assert.equal((file.match(/BEGIN:VEVENT/g) || []).length, 2);
  assert.match(file, /LOCATION:Court 1/);
  assert.match(file, /LOCATION:Court 3/);
  assert.doesNotMatch(file, /Court 2|Court 4/);
  assert.throws(() => createCalendarFile({ ...multi, requested_sessions: multi.requested_sessions.map((s) => ({ ...s, status: "pending" })) }), /No approved sessions/);
});
