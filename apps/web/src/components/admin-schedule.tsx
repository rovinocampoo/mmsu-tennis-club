"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";

type AdminFetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;
type Availability = {
  id: string;
  coach_id: string;
  coach_name: string;
  created_at: string;
  starts_at: string;
  ends_at: string;
  notes: string | null;
};
type Program = { id: string; name: string; description: string | null };
type Session = {
  id: string;
  program_id: string;
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  capacity: number | null;
  confirmed_booking_count: number;
  status: string;
  notes: string | null;
  is_public: boolean;
  assigned_coach_id: string | null;
  assigned_coach_name: string | null;
};
type SessionUpdate = { status?: string; notes?: string; is_public?: boolean };
type Period = "week" | "month";

const sessionDateTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});
const adminMonth = new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", month: "long", year: "numeric" });
const adminDateKey = (date: Date) => new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Manila",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
}).format(date);
const weekDayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function calendarDates(year: number, month: number) {
  const start = new Date(Date.UTC(year, month - 1, 1));
  start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(start);
    date.setUTCDate(start.getUTCDate() + index);
    const key = date.toISOString().slice(0, 10);
    return { key, day: date.getUTCDate(), inMonth: key.slice(0, 7) === `${year}-${String(month).padStart(2, "0")}` };
  });
}

const sessionTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  hour: "numeric",
  minute: "2-digit",
});

function isoFromPHT(day: string, time: string) {
  return new Date(`${day}T${time}:00+08:00`).toISOString();
}

export default function AdminSchedule({
  adminFetch,
  onAuthorizationError,
}: {
  adminFetch: AdminFetch;
  onAuthorizationError: (cause: unknown) => void;
}) {
  const [availability, setAvailability] = useState<Availability[]>([]);
  const [programs, setPrograms] = useState<Program[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [period, setPeriod] = useState<Period>("week");
  const [anchor, setAnchor] = useState(() => adminDateKey(new Date()));
  const [selectedDate, setSelectedDate] = useState(() => adminDateKey(new Date()));
  const [scheduleView, setScheduleView] = useState<"list" | "calendar">("list");
  const [month, setMonth] = useState(() => {
    const now = new Date();
    return { year: Number(adminDateKey(now).slice(0, 4)), month: Number(adminDateKey(now).slice(5, 7)) };
  });

  const periodRange = useMemo(() => {
    const [year, monthNumber, day] = anchor.split("-").map(Number);
    const start = new Date(Date.UTC(year, monthNumber - 1, day));
    if (period === "week") start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
    else start.setUTCDate(1);
    const end = new Date(start);
    if (period === "week") end.setUTCDate(end.getUTCDate() + 7);
    else end.setUTCMonth(end.getUTCMonth() + 1);
    return { start, end, startKey: start.toISOString().slice(0, 10), endKey: end.toISOString().slice(0, 10) };
  }, [anchor, period]);
  const visibleAvailability = availability.filter((item) => {
    const day = adminDateKey(new Date(item.starts_at));
    return day >= periodRange.startKey && day < periodRange.endKey;
  });
  const coaches = useMemo(() => {
    const latestByID = new Map<string, Availability>();
    for (const item of [...availability].sort((a, b) => a.created_at.localeCompare(b.created_at))) {
      latestByID.set(item.coach_id, item);
    }
    return [...latestByID.values()].sort((a, b) => a.coach_name.localeCompare(b.coach_name));
  }, [availability]);
  const monthSessions = sessions.filter((item) => {
    const day = adminDateKey(new Date(item.starts_at));
    return day.startsWith(`${month.year}-${String(month.month).padStart(2, "0")}`);
  });
  const monthAvailability = availability.filter((item) => {
    const day = adminDateKey(new Date(item.starts_at));
    return day.startsWith(`${month.year}-${String(month.month).padStart(2, "0")}`);
  });
  const selectedSessions = monthSessions.filter((item) => adminDateKey(new Date(item.starts_at)) === selectedDate);
  const selectedAvailability = monthAvailability.filter((item) => adminDateKey(new Date(item.starts_at)) === selectedDate);

  function moveAvailabilityPeriod(direction: number) {
    const next = new Date(periodRange.start);
    if (period === "week") next.setUTCDate(next.getUTCDate() + 7 * direction);
    else next.setUTCMonth(next.getUTCMonth() + direction);
    setAnchor(next.toISOString().slice(0, 10));
  }

  function moveScheduleMonth(direction: number) {
    const next = new Date(Date.UTC(month.year, month.month - 1 + direction, 1));
    const nextMonth = { year: next.getUTCFullYear(), month: next.getUTCMonth() + 1 };
    setMonth(nextMonth);
    setSelectedDate(next.toISOString().slice(0, 10));
  }

  const readResponse = useCallback(async (response: Response, fallback: string) => {
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
      if (response.status === 401) {
        const cause = new Error("Your admin session has expired. Please sign in again.");
        onAuthorizationError(cause);
        throw cause;
      }
      if (response.status === 403) {
        const cause = new Error("This Supabase account is not authorized to review club bookings.");
        onAuthorizationError(cause);
        throw cause;
      }
      throw new Error(typeof body?.error === "string" ? body.error : fallback);
    }
    return body;
  }, [onAuthorizationError]);

  const loadSchedule = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [availabilityResponse, programsResponse, sessionsResponse] = await Promise.all([
        adminFetch("/api/backend/admin/coach-availability"),
        adminFetch("/api/backend/admin/training-programs"),
        adminFetch("/api/backend/admin/training-sessions"),
      ]);
      const [availabilityData, programsData, sessionsData] = await Promise.all([
        readResponse(availabilityResponse, "Could not load coach availability."),
        readResponse(programsResponse, "Could not load training programs."),
        readResponse(sessionsResponse, "Could not load training sessions."),
      ]);
      setAvailability(availabilityData.availability || []);
      setPrograms(programsData.programs || []);
      setSessions(sessionsData.sessions || []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load schedule management.");
    } finally {
      setLoading(false);
    }
  }, [adminFetch, readResponse]);

  useEffect(() => {
    void loadSchedule();
  }, [loadSchedule]);

  async function createSession(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const capacityValue = String(values.get("capacity") || "").trim();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await adminFetch("/api/backend/admin/training-sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          program_id: values.get("program_id"),
          starts_at: isoFromPHT(String(values.get("day")), String(values.get("starts_at"))),
          ends_at: isoFromPHT(String(values.get("day")), String(values.get("ends_at"))),
          location: values.get("location"),
          capacity: capacityValue ? Number(capacityValue) : null,
          notes: values.get("notes"),
          is_public: values.get("is_public") === "on",
        }),
      });
      await readResponse(response, "Could not create the training session.");
      form.reset();
      setNotice("Training session created. Coach availability remains separate and was not published.");
      await loadSchedule();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not create the training session.");
    } finally {
      setBusy(false);
    }
  }

  async function updateSession(id: string, update: SessionUpdate) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await adminFetch(`/api/backend/admin/training-sessions/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(update),
      });
      await readResponse(response, "Could not update the training session.");
      setNotice("Training session updated.");
      await loadSchedule();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update the training session.");
    } finally {
      setBusy(false);
    }
  }

  async function assignCoach(sessionID: string, coachID: string | null) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await adminFetch(`/api/backend/admin/training-sessions/${sessionID}/coach`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ coach_id: coachID }),
      });
      await readResponse(response, "Could not assign a coach to the training session.");
      setNotice(coachID ? "Coach assigned to the training session." : "Coach assignment removed.");
      await loadSchedule();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not assign a coach to the training session.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="schedule-management">
      {error && <p className="admin-state form-error" role="alert">{error}</p>}
      {notice && <p className="admin-state form-success" role="status">{notice}</p>}
      <section className="availability-panel">
        <div className="schedule-section-heading"><div><p className="section-kicker">COACH INPUT</p><h2>Submitted availability</h2></div><button className="admin-button secondary" type="button" disabled={loading || busy} onClick={() => { void loadSchedule(); }}>{loading ? "Loading…" : "Refresh"}</button></div>
        <p className="admin-help">Availability is a planning reference only. It never creates or publishes a customer booking session.</p>
        <div className="availability-toolbar">
          <div className="schedule-tabs" role="group" aria-label="Availability review period">
            <button type="button" className={period === "week" ? "active" : ""} aria-pressed={period === "week"} onClick={() => setPeriod("week")}>Week</button>
            <button type="button" className={period === "month" ? "active" : ""} aria-pressed={period === "month"} onClick={() => setPeriod("month")}>Month</button>
          </div>
          <div className="period-navigation"><button className="admin-button secondary" type="button" aria-label="Previous period" onClick={() => moveAvailabilityPeriod(-1)}>‹</button><strong>{periodRange.start.toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric" })} – {new Date(periodRange.end.getTime() - 86400000).toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric", year: "numeric" })}</strong><button className="admin-button secondary" type="button" aria-label="Next period" onClick={() => moveAvailabilityPeriod(1)}>›</button></div>
        </div>
        {visibleAvailability.length === 0 && !loading ? <p className="admin-state">{availability.length === 0 ? "No coach availability has been submitted." : `No availability submitted for this ${period}.`}</p>
          : <div className="availability-list">{visibleAvailability.map((item) => <article className="availability-card" key={item.id}>
            <div><strong>{item.coach_name}</strong><p>{sessionDateTime.format(new Date(item.starts_at))} – {sessionTime.format(new Date(item.ends_at))}</p>{item.notes && <small>{item.notes}</small>}</div>
          </article>)}</div>}
      </section>

      <section className="availability-panel">
        <div className="schedule-section-heading"><div><p className="section-kicker">ACTUAL CUSTOMER SESSIONS</p><h2>Create a training session</h2></div></div>
        {programs.length === 0 && !loading ? <p className="admin-state">No active training programs are available.</p>
          : <form className="schedule-form admin-session-form" onSubmit={createSession}>
            <label>Program<select name="program_id" required defaultValue=""><option value="" disabled>Select a program</option>{programs.map((program) => <option key={program.id} value={program.id}>{program.name}</option>)}</select></label>
            <label>Date<input name="day" type="date" required /></label>
            <label>Start time<input name="starts_at" type="time" required /></label>
            <label>End time<input name="ends_at" type="time" required /></label>
            <label>Venue<input name="location" maxLength={200} required /></label>
            <label>Capacity<input name="capacity" type="number" min="1" step="1" /></label>
            <label className="schedule-form-notes">Notes (optional)<input name="notes" maxLength={2000} /></label>
            <label className="schedule-publish"><input name="is_public" type="checkbox" defaultChecked /> Publish to customer schedule</label>
            <button className="admin-button" type="submit" disabled={busy || programs.length === 0}>{busy ? "Saving…" : "Create session"}</button>
          </form>}
      </section>

      <section className="availability-panel">
        <div className="schedule-section-heading"><div><p className="section-kicker">EXCEPTIONS & PUBLICATION</p><h2>Training sessions</h2></div><div className="schedule-view-switch" role="group" aria-label="Admin schedule view"><button type="button" aria-pressed={scheduleView === "list"} className={scheduleView === "list" ? "active" : ""} onClick={() => setScheduleView("list")}>List</button><button type="button" aria-pressed={scheduleView === "calendar"} className={scheduleView === "calendar" ? "active" : ""} onClick={() => setScheduleView("calendar")}>Calendar</button></div></div>
        {sessions.length === 0 && !loading ? <p className="admin-state">No training sessions have been created.</p>
          : scheduleView === "calendar" ? <>
            <div className="calendar-toolbar">
              <button type="button" aria-label="Previous month" onClick={() => moveScheduleMonth(-1)}>‹</button>
              <h3>{adminMonth.format(new Date(Date.UTC(month.year, month.month - 1, 1)))}</h3>
              <button type="button" aria-label="Next month" onClick={() => moveScheduleMonth(1)}>›</button>
            </div>
            <div className="training-calendar admin-calendar" role="grid" aria-label={`Training and coach availability for ${adminMonth.format(new Date(Date.UTC(month.year, month.month - 1, 1)))}`}>
              {weekDayLabels.map((day) => <div className="calendar-weekday" role="columnheader" key={day}>{day}</div>)}
              {calendarDates(month.year, month.month).map((day) => {
                const daySessions = monthSessions.filter((item) => adminDateKey(new Date(item.starts_at)) === day.key);
                const dayAvailability = monthAvailability.filter((item) => adminDateKey(new Date(item.starts_at)) === day.key);
                return <button className={`calendar-day${day.inMonth ? "" : " outside-month"}${selectedDate === day.key ? " selected" : ""}${daySessions.length || dayAvailability.length ? " has-sessions" : ""}`} type="button" role="gridcell" aria-selected={selectedDate === day.key} aria-label={`${day.key}, ${daySessions.length} sessions, ${dayAvailability.length} coach availability submissions`} key={day.key} onClick={() => setSelectedDate(day.key)}>
                  <span className="calendar-day-number">{day.day}</span>
                  {(daySessions.length > 0 || dayAvailability.length > 0) && <span className="calendar-day-count">{daySessions.length ? `${daySessions.length} sessions` : ""}{daySessions.length && dayAvailability.length ? " · " : ""}{dayAvailability.length ? `${dayAvailability.length} available` : ""}</span>}
                  <span className="calendar-day-dots" aria-hidden="true">{daySessions.slice(0, 2).map((item) => <i className={`dot-${item.status === "cancelled" ? "closed" : item.is_public ? "open" : "full"}`} key={`session-${item.id}`} />)}{dayAvailability.slice(0, 2).map((item) => <i className="dot-availability" key={`availability-${item.id}`} />)}</span>
                </button>;
              })}
            </div>
            <div className="admin-day-agenda">
              <h3>{selectedDate ? new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", weekday: "long", month: "long", day: "numeric" }).format(new Date(`${selectedDate}T12:00:00+08:00`)) : "Selected date"}</h3>
              {selectedSessions.length === 0 && selectedAvailability.length === 0 ? <p className="admin-state">No sessions or availability on this date.</p> : <>
                {selectedAvailability.map((item) => <article className="availability-card agenda-card" key={item.id}><div><strong>Coach availability · {item.coach_name}</strong><p>{sessionDateTime.format(new Date(item.starts_at))} – {sessionTime.format(new Date(item.ends_at))}</p>{item.notes && <small>{item.notes}</small>}</div></article>)}
                {selectedSessions.map((item) => <article className="availability-card agenda-card" key={item.id}><div><strong>{item.program_name}</strong><p>{sessionDateTime.format(new Date(item.starts_at))} – {sessionTime.format(new Date(item.ends_at))} · {item.location}</p><small>{item.status} · {item.is_public ? "published" : "hidden"} · {item.confirmed_booking_count} confirmed · {item.assigned_coach_name ? `Coach ${item.assigned_coach_name}` : "Unassigned"}</small></div></article>)}
              </>}
            </div>
          </> : <div className="schedule-session-list">{sessions.map((session) => <article className="schedule-session-card" key={session.id}>
            <div className="schedule-session-summary">
              <div><h3>{session.program_name}</h3><p>{sessionDateTime.format(new Date(session.starts_at))} – {sessionTime.format(new Date(session.ends_at))} · {session.location}</p><div className="session-status-row"><span className={`session-status session-status-${session.status}`}>{session.status === "open" ? "Booking open" : session.status === "cancelled" ? "Cancelled" : "Booking closed"}</span><span className={`session-status ${session.is_public ? "session-status-published" : "session-status-hidden"}`}>{session.is_public ? "Published" : "Hidden"}</span><span className="session-booking-count">{session.confirmed_booking_count} confirmed{session.capacity === null ? "" : ` of ${session.capacity}`}</span><span className="session-booking-count">{session.assigned_coach_name ? `Coach ${session.assigned_coach_name}` : "No coach assigned"}</span></div>{session.notes && <p className="session-description">{session.notes}</p>}</div>
              <div className="schedule-session-actions">
                <label className="session-coach-assignment">Assigned coach<select value={session.assigned_coach_id || ""} disabled={busy || (coaches.length === 0 && !session.assigned_coach_id)} onChange={(event) => { void assignCoach(session.id, event.target.value || null); }}><option value="">Unassigned</option>{session.assigned_coach_id && !coaches.some((coach) => coach.coach_id === session.assigned_coach_id) && <option value={session.assigned_coach_id}>{session.assigned_coach_name || "Previously assigned coach"}</option>}{coaches.map((coach) => <option key={coach.coach_id} value={coach.coach_id}>{coach.coach_name}</option>)}</select></label>
                {coaches.length === 0 && <span className="admin-help">A coach must submit availability before being assigned.</span>}
                {session.status !== "cancelled" && <button className="admin-button secondary" type="button" disabled={busy} onClick={() => { void updateSession(session.id, { status: session.status === "open" ? "closed" : "open" }); }}>{session.status === "open" ? "Close booking" : "Reopen booking"}</button>}
                {session.status !== "cancelled" && <button className="admin-button danger" type="button" disabled={busy} onClick={() => { if (window.confirm(`Cancel ${session.program_name}? It will no longer accept booking requests.`)) void updateSession(session.id, { status: "cancelled" }); }}>Cancel session</button>}
                <button className="admin-button secondary" type="button" disabled={busy} onClick={() => { void updateSession(session.id, { is_public: !session.is_public }); }}>{session.is_public ? "Unpublish" : "Publish"}</button>
              </div>
            </div>
            <form className="schedule-note-form" onSubmit={(event) => {
              event.preventDefault();
              const notes = new FormData(event.currentTarget).get("notes");
              void updateSession(session.id, { notes: String(notes || "") });
            }}>
              <label>Admin note<input name="notes" maxLength={2000} defaultValue={session.notes || ""} /></label>
              <button className="admin-button secondary" type="submit" disabled={busy}>Save note</button>
            </form>
          </article>)}</div>}
      </section>
    </div>
  );
}
