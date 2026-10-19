"use client";

import { useMemo, useState } from "react";
import BookingRequestForm from "@/components/booking-request-form";
import styles from "./training-slot-selection.module.css";
import {
  sessionAvailability,
  unavailableReason,
} from "@/lib/session-availability.mjs";
import { scrollToElement } from "@/lib/scroll-to.mjs";
import type { TrainingProgram, TrainingSession } from "@/lib/training-sessions";

type ScheduleView = "list" | "calendar";
type PrimaryTab = "sessions" | "programs";

const phtDateKey = (date: Date) =>
  new Intl.DateTimeFormat("en-CA", {
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

function canRequestBooking(session: TrainingSession) {
  return (
    session.status === "open" &&
    (session.capacity === null ||
      (session.remaining_capacity !== null && session.remaining_capacity > 0))
  );
}

function sessionDateKey(session: TrainingSession) {
  return phtDateKey(new Date(session.starts_at));
}

function SessionCard({
  session,
  selected,
  onToggle,
}: {
  session: TrainingSession;
  selected: boolean;
  onToggle: (sessionID: string) => void;
}) {
  const availability = sessionAvailability(session);
  return (
    <article className="session-card" key={session.id} data-reveal="up">
      <div className="session-date">
        <span>
          {compactDate.format(new Date(session.starts_at)).split(" ")[0]}
        </span>
        <strong>
          {new Intl.DateTimeFormat("en-PH", {
            timeZone: "Asia/Manila",
            day: "2-digit",
          }).format(new Date(session.starts_at))}
        </strong>
        <small>
          {new Intl.DateTimeFormat("en-PH", {
            timeZone: "Asia/Manila",
            month: "short",
          }).format(new Date(session.starts_at))}
        </small>
      </div>
      <div className="session-main">
        <div className="session-title-row">
          <h3>{session.program_name}</h3>
          <span className={`availability availability-${availability.kind}`}>
            {availability.label}
          </span>
        </div>
        {session.program_description && (
          <p className="session-description">{session.program_description}</p>
        )}
        <div className="session-details">
          <span>
            <span className="detail-icon" aria-hidden="true">
              ◷
            </span>
            {manilaDate.format(new Date(session.starts_at))}
          </span>
          <span>
            <span className="detail-icon" aria-hidden="true">
              ◴
            </span>
            {manilaTime.format(new Date(session.starts_at))} –{" "}
            {manilaTime.format(new Date(session.ends_at))}
          </span>
          <span>
            <span className="detail-icon" aria-hidden="true">
              ⌖
            </span>
            {session.location}
          </span>
        </div>
        <p className="session-policy">
          Requests are reviewed by the club. A request does not reserve a spot.
        </p>
      </div>
      <div className="session-action">
        {canRequestBooking(session) ? (
          <label className={styles.slotSelect}>
            <input
              type="checkbox"
              checked={selected}
              onChange={() => onToggle(session.id)}
            />
            <span>{selected ? "Slot selected" : "Select this slot"}</span>
          </label>
        ) : (
          <span
            className={`session-unavailable session-unavailable-${availability.kind}`}
            role="status"
          >
            <span className="session-unavailable-icon" aria-hidden="true">
              {availability.kind === "full" ? "✕" : "⌛"}
            </span>
            {unavailableReason(availability.kind)}
          </span>
        )}
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
  const [view, setView] = useState<ScheduleView>("calendar");
  const [primaryTab, setPrimaryTab] = useState<PrimaryTab>("sessions");
  const [selectedSessionIDs, setSelectedSessionIDs] = useState<string[]>([]);
  const [month, setMonth] = useState(() => {
    const [year, monthNumber] = initialMonth.split("-").map(Number);
    return { year, month: monthNumber };
  });
  const [selectedDay, setSelectedDay] = useState(() => phtDateKey(new Date()));

  const programCards = useMemo(
    () =>
      programs.map((program) => ({
        ...program,
        sessionCount: sessions.filter(
          (session) => session.program_name === program.name,
        ).length,
      })),
    [programs, sessions],
  );

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
  const monthEndKey = new Date(Date.UTC(month.year, month.month, 0))
    .toISOString()
    .slice(0, 10);
  const firstCell = new Date(monthStart);
  firstCell.setUTCDate(
    firstCell.getUTCDate() - ((firstCell.getUTCDay() + 6) % 7),
  );
  const days = Array.from({ length: 42 }, (_, index) => {
    const day = new Date(firstCell);
    day.setUTCDate(day.getUTCDate() + index);
    const key = day.toISOString().slice(0, 10);
    return {
      key,
      day: day.getUTCDate(),
      inMonth: key >= monthStartKey && key <= monthEndKey,
    };
  });
  const selectedDaySessions = (sessionsByDate.get(selectedDay) || []).sort(
    (a, b) => a.starts_at.localeCompare(b.starts_at),
  );

  const selectedSessions = useMemo(
    () =>
      selectedSessionIDs
        .map((id) => sessions.find((session) => session.id === id))
        .filter((session): session is TrainingSession => Boolean(session))
        .sort((a, b) => a.starts_at.localeCompare(b.starts_at)),
    [selectedSessionIDs, sessions],
  );

  const selectedSlotError = useMemo(() => {
    if (selectedSessions.length !== selectedSessionIDs.length) {
      return "One of the selected slots is no longer available. Refresh the page and select your times again.";
    }
    if (selectedSessions.length > 8) {
      return "Select no more than eight consecutive slots in one request.";
    }
    if (selectedSessions.some((session) => !canRequestBooking(session))) {
      return "One or more selected slots are no longer available. Remove them or refresh the schedule.";
    }
    if (selectedSessions.length < 2) return null;
    const first = selectedSessions[0];
    for (let index = 1; index < selectedSessions.length; index += 1) {
      const previous = selectedSessions[index - 1];
      const current = selectedSessions[index];
      if (
        sessionDateKey(current) !== sessionDateKey(first) ||
        current.program_name !== first.program_name ||
        current.location.trim().toLocaleLowerCase() !== first.location.trim().toLocaleLowerCase()
      ) {
        return "Choose adjacent slots from the same program, date, and location.";
      }
      if (Date.parse(previous.ends_at) !== Date.parse(current.starts_at)) {
        return "Selected times must be consecutive with no gaps or overlaps.";
      }
    }
    return null;
  }, [selectedSessionIDs.length, selectedSessions]);

  function toggleSessionSelection(sessionID: string) {
    setSelectedSessionIDs((current) => {
      if (current.includes(sessionID)) {
        return current.filter((id) => id !== sessionID);
      }
      if (current.length >= 8) return current;
      return [...current, sessionID];
    });
  }

  function moveMonth(direction: number) {
    const next = new Date(Date.UTC(month.year, month.month - 1 + direction, 1));
    const nextMonth = {
      year: next.getUTCFullYear(),
      month: next.getUTCMonth() + 1,
    };
    setMonth(nextMonth);
    const nextMonthStart = next.toISOString().slice(0, 10);
    setSelectedDay(nextMonthStart);
  }

  function chooseProgram(name: string) {
    const nextSession = sessions.find(
      (session) => session.program_name === name,
    );
    if (!nextSession) return;
    const date = new Date(nextSession.starts_at);
    setMonth({
      year: Number(phtDateKey(date).slice(0, 4)),
      month: Number(phtDateKey(date).slice(5, 7)),
    });
    setSelectedDay(sessionDateKey(nextSession));
    setView("calendar");
    setPrimaryTab("sessions");
    // Wait for React to reveal the sessions panel before scrolling to it.
    window.requestAnimationFrame(() => {
      scrollToElement("sessions");
    });
  }

  return (
    <>
      <div className="training-explorer" data-reveal="up">
        <p className="training-explorer-kicker">EXPLORE THE CLUB</p>
        <div
          className="training-view-tabs"
          role="group"
          aria-label="Training content"
        >
          <button
            className={`training-view-tab${primaryTab === "sessions" ? " active" : ""}`}
            type="button"
            aria-pressed={primaryTab === "sessions"}
            onClick={() => setPrimaryTab("sessions")}
          >
            <span>Upcoming sessions</span>
            <span
              className="training-view-count"
              aria-label={`${sessionsUnavailable ? "Unknown" : sessions.length} upcoming sessions`}
            >
              {sessionsUnavailable ? "—" : sessions.length}
            </span>
          </button>
          <button
            className={`training-view-tab${primaryTab === "programs" ? " active" : ""}`}
            type="button"
            aria-pressed={primaryTab === "programs"}
            onClick={() => setPrimaryTab("programs")}
          >
            <span>Training programs</span>
            <span
              className="training-view-count"
              aria-label={`${programsUnavailable ? "Unknown" : programs.length} active programs`}
            >
              {programsUnavailable ? "—" : programs.length}
            </span>
          </button>
        </div>
        <p className="training-explorer-hint">
          {primaryTab === "sessions"
            ? "Find a scheduled session and request a spot. Every request is reviewed by the club."
            : "Explore the club’s training options. Session dates and booking details are listed separately."}
        </p>
      </div>
      <section
        className="published-sessions training-tab-panel"
        id="sessions"
        aria-labelledby="schedule-heading"
        hidden={primaryTab !== "sessions"}
        tabIndex={-1}
      >
        <div className="schedule-heading" data-reveal="up">
          <div>
            <p className="section-kicker">FIND A TIME THAT WORKS</p>
            <h2 id="schedule-heading">Published sessions</h2>
          </div>
          <p className="timezone-note">All times are Philippine Time (PHT)</p>
        </div>
        <div className="schedule-toolbar" data-reveal="up">
          <p>
            {sessions.length} {sessions.length === 1 ? "session" : "sessions"}{" "}
            available to explore
          </p>
          <div
            className="schedule-view-switch"
            role="group"
            aria-label="Schedule view"
          >
            <button
              type="button"
              aria-pressed={view === "list"}
              className={view === "list" ? "active" : ""}
              onClick={() => setView("list")}
            >
              List
            </button>
            <button
              type="button"
              aria-pressed={view === "calendar"}
              className={view === "calendar" ? "active" : ""}
              onClick={() => setView("calendar")}
            >
              Calendar
            </button>
          </div>
        </div>
        {sessionsUnavailable && (
          <div className="state-panel error-panel" role="alert">
            <span className="state-icon" aria-hidden="true">
              !
            </span>
            <div>
              <h3>Schedule temporarily unavailable</h3>
              <p>
                We couldn’t load published sessions right now. Please try again
                in a little while.
              </p>
            </div>
          </div>
        )}
        {view === "list" ? (
          sessions.length === 0 ? (
            <p className="admin-state">
              {sessionsUnavailable
                ? "Published sessions could not be loaded."
                : "There are no upcoming sessions published right now."}
            </p>
          ) : (
            <div className="session-list">
              {sessions.map((session) => (
                <SessionCard
                  key={session.id}
                  session={session}
                  selected={selectedSessionIDs.includes(session.id)}
                  onToggle={toggleSessionSelection}
                />
              ))}
            </div>
          )
        ) : (
          <>
            <div className="calendar-toolbar">
              <button
                type="button"
                aria-label="Previous month"
                onClick={() => moveMonth(-1)}
              >
                ‹
              </button>
              <h3>{calendarMonth.format(monthStart)}</h3>
              <button
                type="button"
                aria-label="Next month"
                onClick={() => moveMonth(1)}
              >
                ›
              </button>
            </div>
            <div
              className="training-calendar"
              role="grid"
              aria-label={`Training sessions for ${calendarMonth.format(monthStart)}`}
             data-reveal="up">
              {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map(
                (weekday) => (
                  <div
                    className="calendar-weekday"
                    role="columnheader"
                    key={weekday}
                  >
                    {weekday}
                  </div>
                ),
              )}
              {days.map((day) => {
                const daySessions = sessionsByDate.get(day.key) || [];
                return (
                  <button
                    className={`calendar-day${day.inMonth ? "" : " outside-month"}${selectedDay === day.key ? " selected" : ""}${daySessions.length ? " has-sessions" : ""}`}
                    type="button"
                    role="gridcell"
                    aria-selected={selectedDay === day.key}
                    aria-label={`${day.key}${daySessions.length ? `, ${daySessions.length} sessions` : ""}`}
                    key={day.key}
                    onClick={() => {
                      setSelectedDay(day.key);

                      window.requestAnimationFrame(() => {
                        scrollToElement("selected-date-agenda");
                      });
                    }}
                  >
                    <span className="calendar-day-number">{day.day}</span>
                    {daySessions.length > 0 && (
                      <span className="calendar-day-count">
                        {daySessions.length}{" "}
                        {daySessions.length === 1 ? "session" : "sessions"}
                      </span>
                    )}
                    <span className="calendar-day-dots" aria-hidden="true">
                      {daySessions.slice(0, 3).map((session) => (
                        <i
                          className={`dot-${sessionAvailability(session).kind}`}
                          key={session.id}
                        />
                      ))}
                    </span>
                  </button>
                );
              })}
            </div>
            <div
              className="calendar-day-agenda"
              id="selected-date-agenda"
              style={{ scrollMarginTop: "24px" }}
             data-reveal="up">
              <div className="agenda-heading">
                <div>
                  <p className="section-kicker">SELECTED DATE</p>
                  <h3>
                    {selectedDay
                      ? manilaDate.format(
                          new Date(`${selectedDay}T12:00:00+08:00`),
                        )
                      : "Choose a date"}
                  </h3>
                </div>
              </div>
              {selectedDaySessions.length === 0 ? (
                <p className="admin-state">
                  No published sessions on this date. Choose a date with a
                  session marker to explore options.
                </p>
              ) : (
                <div className="session-list">
                  {selectedDaySessions.map((session) => (
                    <SessionCard
                      key={session.id}
                      session={session}
                      selected={selectedSessionIDs.includes(session.id)}
                      onToggle={toggleSessionSelection}
                    />
                  ))}
                </div>
              )}
            </div>
          </>
        )}
        {selectedSessionIDs.length > 0 && (
          <div className={styles.bookingPanel} aria-live="polite">
            <div>
              <p className="section-kicker">YOUR SELECTED TIME</p>
              <h3>
                {selectedSessions.length} slot{selectedSessions.length === 1 ? "" : "s"} selected
              </h3>
              {selectedSessions.length > 0 && (
                <p>
                  {manilaDate.format(new Date(selectedSessions[0].starts_at))}
                  {" · "}
                  {manilaTime.format(new Date(selectedSessions[0].starts_at))}
                  {" – "}
                  {manilaTime.format(new Date(selectedSessions[selectedSessions.length - 1].ends_at))}
                </p>
              )}
              {selectedSlotError && (
                <p className={styles.selectionError} role="alert">{selectedSlotError}</p>
              )}
              <button
                type="button"
                className="admin-button secondary"
                onClick={() => setSelectedSessionIDs([])}
              >
                Clear selection
              </button>
            </div>
            {!selectedSlotError && selectedSessions.length > 0 && (
              <BookingRequestForm
                key={selectedSessions.map((session) => session.id).join(":")}
                sessionIds={selectedSessions.map((session) => session.id)}
              />
            )}
          </div>
        )}
      </section>
      <section
        className="programs-section training-tab-panel"
        id="programs"
        aria-labelledby="programs-title"
        hidden={primaryTab !== "programs"}
        tabIndex={-1}
      >
        <div className="schedule-heading compact-heading">
          <div>
            <p className="section-kicker">CLUB OFFERINGS</p>
            <h2 id="programs-title">Training programs</h2>
          </div>
          <p className="timezone-note">
            Programs describe what we teach; published sessions show when you
            can book.
          </p>
        </div>
        {programsUnavailable && (
          <p className="admin-state form-error" role="alert">
            Training program information is temporarily unavailable. Please try
            again later.
          </p>
        )}
        {sessionsUnavailable && !programsUnavailable && (
          <p className="program-schedule-notice" role="status">
            Upcoming session counts are temporarily unavailable. You can still
            explore the program descriptions.
          </p>
        )}
        {programCards.length === 0 && !programsUnavailable ? (
          <p className="admin-state">
            No active training programs are listed right now.
          </p>
        ) : null}
        <div className="program-catalog">
          {programCards.map((program) => (
            <details className="program-catalog-item" key={program.id} data-reveal="up">
              <summary className="program-catalog-summary">
                <span className="program-catalog-icon" aria-hidden="true">
                  ↗
                </span>
                <span className="program-catalog-copy">
                  <span className="program-catalog-name">{program.name}</span>
                  <span className="program-catalog-prompt">
                    View program details
                  </span>
                </span>
                {!sessionsUnavailable && program.sessionCount > 0 && (
                  <span className="program-catalog-count">
                    {program.sessionCount} upcoming{" "}
                    {program.sessionCount === 1 ? "session" : "sessions"}
                  </span>
                )}
                <span className="program-catalog-chevron" aria-hidden="true">
                  ⌄
                </span>
              </summary>
              <div className="program-catalog-details">
                <p>
                  {program.description ||
                    "Contact the club for more information about this program."}
                </p>
                {sessionsUnavailable ? (
                  <span className="program-detail-note">
                    Session availability could not be loaded.
                  </span>
                ) : program.sessionCount === 0 ? (
                  <span className="program-detail-note">
                    No upcoming sessions are scheduled yet.
                  </span>
                ) : (
                  <button
                    className="text-action"
                    type="button"
                    onClick={() => chooseProgram(program.name)}
                  >
                    Explore{" "}
                    {program.sessionCount === 1 ? "session" : "sessions"}{" "}
                    <span aria-hidden="true">→</span>
                  </button>
                )}
              </div>
            </details>
          ))}
        </div>
      </section>
    </>
  );
}
