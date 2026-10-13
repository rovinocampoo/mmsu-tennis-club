"use client";

import { useMemo, useState } from "react";
import BookingRequestForm from "@/components/booking-request-form";
import type { TrainingProgram, TrainingSession } from "@/lib/training-sessions";

type ScheduleView = "list" | "calendar";

const phtDateKey = (date: Date) => new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Manila",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
}).format(date);
const manilaDate = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "long",
  month: "long",
  day: "numeric",
  year: "numeric",
});
const compactDate = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "short",
  month: "short",
  day: "numeric",
});
const manilaTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  hour: "numeric",
  minute: "2-digit",
  hour12: true,
});
const calendarMonth = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  month: "long",
  year: "numeric",
});

function sessionAvailability(session: TrainingSession) {
  if (session.status !== "open") return { label: "Registration closed", kind: "closed" };
  if (session.remaining_capacity === 0) return { label: "Full", kind: "full" };
  if (session.capacity === null) return { label: "Open", kind: "open" };
  return { label: `${session.remaining_capacity ?? 0} spaces left`, kind: "open" };
}

function canRequestBooking(session: TrainingSession) {
  return session.status === "open" && (
    session.capacity === null ||
    (session.remaining_capacity !== null && session.remaining_capacity > 0)
  );
}

function sessionDateKey(session: TrainingSession) {
  return phtDateKey(new Date(session.starts_at));
}

function SessionCard({ session }: { session: TrainingSession }) {
  const availability = sessionAvailability(session);
  return (
    <article className="session-card" key={session.id}>
      <div className="session-date">
        <span>{compactDate.format(new Date(session.starts_at)).split(" ")[0]}</span>
        <strong>{new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", day: "2-digit" }).format(new Date(session.starts_at))}</strong>
        <small>{new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", month: "short" }).format(new Date(session.starts_at))}</small>
      </div>
      <div className="session-main">
        <div className="session-title-row">
          <h3>{session.program_name}</h3>
          <span className={`availability availability-${availability.kind}`}>{availability.label}</span>
        </div>
        {session.program_description && <p className="session-description">{session.program_description}</p>}
        <div className="session-details">
          <span><span className="detail-icon" aria-hidden="true">◷</span>{manilaDate.format(new Date(session.starts_at))}</span>
          <span><span className="detail-icon" aria-hidden="true">◴</span>{manilaTime.format(new Date(session.starts_at))} – {manilaTime.format(new Date(session.ends_at))}</span>
          <span><span className="detail-icon" aria-hidden="true">⌖</span>{session.location}</span>
        </div>
        <p className="session-policy">Requests are reviewed by the club. A request does not reserve a spot.</p>
      </div>
      <div className="session-action">
        {canRequestBooking(session)
          ? <BookingRequestForm sessionId={session.id} />
          : <span className="session-unavailable">{availability.kind === "full" ? "No spots left" : "Not accepting requests"}</span>}
      </div>
    </article>
  );
}

export default function TrainingSchedule({
  sessions,
  programs,
  programsUnavailable,
  sessionsUnavailable,
  initialMonth,
}: {
  sessions: TrainingSession[];
  programs: TrainingProgram[];
  programsUnavailable: boolean;
  sessionsUnavailable: boolean;
  initialMonth: string;
}) {
  const [view, setView] = useState<ScheduleView>("list");
  const [month, setMonth] = useState(() => {
    const [year, monthNumber] = initialMonth.split("-").map(Number);
    return { year, month: monthNumber };
  });
  const [selectedDay, setSelectedDay] = useState(() => phtDateKey(new Date()));

  const programCards = useMemo(() => programs.map((program) => ({
    ...program,
    sessionCount: sessions.filter((session) => session.program_name === program.name).length,
  })), [programs, sessions]);

  const sessionsByDate = useMemo(() => {
    const grouped = new Map<string, TrainingSession[]>();
    for (const session of sessions) {
      const key = sessionDateKey(session);
      grouped.set(key, [...(grouped.get(key) || []), session]);
    }
    return grouped;
  }, [sessions]);

  const monthStart = new Date(Date.UTC(month.year, month.month - 1, 1));
  const monthStartKey = monthStart.toISOString().slice(0, 10);
  const monthEndKey = new Date(Date.UTC(month.year, month.month, 0)).toISOString().slice(0, 10);
  const firstCell = new Date(monthStart);
  firstCell.setUTCDate(firstCell.getUTCDate() - ((firstCell.getUTCDay() + 6) % 7));
  const days = Array.from({ length: 42 }, (_, index) => {
    const day = new Date(firstCell);
    day.setUTCDate(day.getUTCDate() + index);
    const key = day.toISOString().slice(0, 10);
    return { key, day: day.getUTCDate(), inMonth: key >= monthStartKey && key <= monthEndKey };
  });
  const selectedDaySessions = (sessionsByDate.get(selectedDay) || []).sort((a, b) => a.starts_at.localeCompare(b.starts_at));

  function moveMonth(direction: number) {
    const next = new Date(Date.UTC(month.year, month.month - 1 + direction, 1));
    const nextMonth = { year: next.getUTCFullYear(), month: next.getUTCMonth() + 1 };
    setMonth(nextMonth);
    const nextMonthStart = next.toISOString().slice(0, 10);
    setSelectedDay(nextMonthStart);
  }

  function chooseProgram(name: string) {
    const nextSession = sessions.find((session) => session.program_name === name);
    if (!nextSession) return;
    const date = new Date(nextSession.starts_at);
    setMonth({ year: Number(phtDateKey(date).slice(0, 4)), month: Number(phtDateKey(date).slice(5, 7)) });
    setSelectedDay(sessionDateKey(nextSession));
    setView("calendar");
    document.getElementById("sessions")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  return (
    <>
      <section className="programs-section" aria-labelledby="programs-title">
        <div className="schedule-heading compact-heading">
          <div><p className="section-kicker">ACTIVE PROGRAMS</p><h2 id="programs-title">Training programs</h2></div>
          <p className="timezone-note">Offerings shown here come from upcoming published sessions.</p>
        </div>
        {programsUnavailable && <p className="admin-state form-error" role="alert">Training program information is temporarily unavailable.</p>}
        {programCards.length === 0 && !programsUnavailable ? <p className="admin-state">No active training programs are listed right now.</p> : null}
        <div className="program-grid">
          {programCards.map((program) => (
            <article className="program-card" key={program.name}>
              <span className="program-icon" aria-hidden="true">↗</span>
              <p className="program-session-count">{program.sessionCount === 0 ? "No upcoming sessions" : `${program.sessionCount} upcoming ${program.sessionCount === 1 ? "session" : "sessions"}`}</p>
              <h3>{program.name}</h3>
              <p>{program.description || "Program details have not been provided."}</p>
              <button className="text-action" type="button" disabled={program.sessionCount === 0 || sessionsUnavailable} onClick={() => chooseProgram(program.name)}>{sessionsUnavailable ? "Schedule unavailable" : program.sessionCount === 0 ? "No published sessions" : <>Explore sessions <span aria-hidden="true">→</span></>}</button>
            </article>
          ))}
        </div>
      </section>

      <section className="published-sessions" id="sessions" aria-labelledby="schedule-heading">
        <div className="schedule-heading">
          <div><p className="section-kicker">FIND A TIME THAT WORKS</p><h2 id="schedule-heading">Published sessions</h2></div>
          <p className="timezone-note">All times are Philippine Time (PHT)</p>
        </div>
        <div className="schedule-toolbar">
          <p>{sessions.length} {sessions.length === 1 ? "session" : "sessions"} available to explore</p>
          <div className="schedule-view-switch" role="group" aria-label="Schedule view">
            <button type="button" aria-pressed={view === "list"} className={view === "list" ? "active" : ""} onClick={() => setView("list")}>List</button>
            <button type="button" aria-pressed={view === "calendar"} className={view === "calendar" ? "active" : ""} onClick={() => setView("calendar")}>Calendar</button>
          </div>
        </div>
        {sessionsUnavailable && <div className="state-panel error-panel" role="alert"><span className="state-icon" aria-hidden="true">!</span><div><h3>Schedule temporarily unavailable</h3><p>We couldn’t load published sessions right now. Please try again in a little while.</p></div></div>}
        {view === "list" ? sessions.length === 0 ? <p className="admin-state">{sessionsUnavailable ? "Published sessions could not be loaded." : "There are no upcoming sessions published right now."}</p> : <div className="session-list">
          {sessions.map((session) => <SessionCard key={session.id} session={session} />)}
        </div> : <>
          <div className="calendar-toolbar">
            <button type="button" aria-label="Previous month" onClick={() => moveMonth(-1)}>‹</button>
            <h3>{calendarMonth.format(monthStart)}</h3>
            <button type="button" aria-label="Next month" onClick={() => moveMonth(1)}>›</button>
          </div>
          <div className="training-calendar" role="grid" aria-label={`Training sessions for ${calendarMonth.format(monthStart)}`}>
            {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((weekday) => <div className="calendar-weekday" role="columnheader" key={weekday}>{weekday}</div>)}
            {days.map((day) => {
              const daySessions = sessionsByDate.get(day.key) || [];
              return <button
                className={`calendar-day${day.inMonth ? "" : " outside-month"}${selectedDay === day.key ? " selected" : ""}${daySessions.length ? " has-sessions" : ""}`}
                type="button"
                role="gridcell"
                aria-selected={selectedDay === day.key}
                aria-label={`${day.key}${daySessions.length ? `, ${daySessions.length} sessions` : ""}`}
                key={day.key}
                onClick={() => setSelectedDay(day.key)}
              >
                <span className="calendar-day-number">{day.day}</span>
                {daySessions.length > 0 && <span className="calendar-day-count">{daySessions.length} {daySessions.length === 1 ? "session" : "sessions"}</span>}
                <span className="calendar-day-dots" aria-hidden="true">{daySessions.slice(0, 3).map((session) => <i className={`dot-${sessionAvailability(session).kind}`} key={session.id} />)}</span>
              </button>;
            })}
          </div>
          <div className="calendar-day-agenda">
            <div className="agenda-heading"><div><p className="section-kicker">SELECTED DATE</p><h3>{selectedDay ? manilaDate.format(new Date(`${selectedDay}T12:00:00+08:00`)) : "Choose a date"}</h3></div></div>
            {selectedDaySessions.length === 0 ? <p className="admin-state">No published sessions on this date. Choose a date with a session marker to explore options.</p>
              : <div className="session-list">{selectedDaySessions.map((session) => <SessionCard key={session.id} session={session} />)}</div>}
          </div>
        </>}
      </section>
    </>
  );
}
