"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";
import styles from "./approved-clients.module.css";

type ApprovedClient = {
  booking_id: string;
  player_id: string;
  player_name: string;
  player_email: string | null;
  player_phone: string | null;
  session_id: string;
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  capacity: number | null;
  assigned_coach_id: string | null;
  assigned_coach_name: string | null;
  booking_created_at: string;
  participant_names: string[] | null;
};

type AuthStatus = "checking" | "authenticated" | "signed_out" | "unavailable";
type LoadStatus = "idle" | "loading" | "ready" | "error";
type ScheduleView = "list" | "calendar";

type CalendarDate = {
  key: string;
  day: number;
  inMonth: boolean;
};

const supabase = getSupabaseBrowserClient();

const expiredSessionMessage =
  "Your admin session has expired. Please sign in again.";

const weekDayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const calendarMonthFormatter = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  month: "long",
  year: "numeric",
});

const selectedDateFormatter = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "long",
  month: "long",
  day: "numeric",
  year: "numeric",
});

function manilaDateKey(value: string) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Manila",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(value));

  const part = (type: string) =>
    parts.find((item) => item.type === type)?.value ?? "";

  return `${part("year")}-${part("month")}-${part("day")}`;
}

function todayManilaDateKey() {
  return manilaDateKey(new Date().toISOString());
}

function calendarDates(year: number, month: number): CalendarDate[] {
  const firstDay = new Date(Date.UTC(year, month - 1, 1));

  firstDay.setUTCDate(firstDay.getUTCDate() - ((firstDay.getUTCDay() + 6) % 7));

  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(firstDay);
    date.setUTCDate(firstDay.getUTCDate() + index);

    const key = date.toISOString().slice(0, 10);

    return {
      key,
      day: date.getUTCDate(),
      inMonth: key.slice(0, 7) === `${year}-${String(month).padStart(2, "0")}`,
    };
  });
}

function formatSessionDate(value: string) {
  return new Intl.DateTimeFormat("en-PH", {
    timeZone: "Asia/Manila",
    weekday: "long",
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(value));
}

function formatSessionTime(value: string) {
  return new Intl.DateTimeFormat("en-PH", {
    timeZone: "Asia/Manila",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatBookedAt(value: string) {
  return new Intl.DateTimeFormat("en-PH", {
    timeZone: "Asia/Manila",
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function countLabel(count: number, singular: string, plural?: string) {
  return `${count} ${count === 1 ? singular : (plural ?? `${singular}s`)}`;
}

function BookingCard({ record }: { record: ApprovedClient }) {
  const contactDetails = [record.player_email, record.player_phone].filter(
    (value): value is string => Boolean(value?.trim()),
  );

  return (
    <article className={`admin-card ${styles.bookingCard}`}>
      <div className={styles.bookingCardHeading}>
        <div className={styles.playerIdentity}>
          <p className="section-kicker">PLAYER</p>
          <h3>{record.player_name}</h3>
        </div>

        <span className="approved-booking-pill">Confirmed</span>
      </div>

      <p className={styles.contactDetails}>
        {contactDetails.length > 0
          ? contactDetails.join(" · ")
          : "No contact details on record"}
      </p>

      <div className={`approved-client-fields ${styles.bookingFields}`}>
        <div>
          <span>Training program</span>
          <strong>{record.program_name}</strong>
        </div>

        <div>
          <span>Session date and time</span>
          <strong>
            {formatSessionDate(record.starts_at)}
            <br />
            {formatSessionTime(record.starts_at)} –{" "}
            {formatSessionTime(record.ends_at)} PHT
          </strong>
        </div>

        <div>
          <span>Venue</span>
          <strong>{record.location || "Not specified"}</strong>
        </div>

        <div>
          <span>Assigned coach</span>
          <strong>{record.assigned_coach_name || "Not assigned"}</strong>
        </div>

        <div>
          <span>Participants</span>
          <strong>
            {record.participant_names && record.participant_names.length > 0
              ? record.participant_names.join(", ")
              : record.player_name}
          </strong>
        </div>

        <div>
          <span>Booking recorded</span>
          <strong>{formatBookedAt(record.booking_created_at)}</strong>
        </div>

        <div>
          <span>Booking reference</span>
          <strong className={styles.bookingReference}>
            {record.booking_id}
          </strong>
        </div>
      </div>
    </article>
  );
}

export default function ApprovedClientsPage() {
  const [authStatus, setAuthStatus] = useState<AuthStatus>("checking");
  const [sessionRetry, setSessionRetry] = useState(0);
  const [records, setRecords] = useState<ApprovedClient[]>([]);
  const [loadStatus, setLoadStatus] = useState<LoadStatus>("idle");
  const [error, setError] = useState("");

  const [search, setSearch] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [programFilter, setProgramFilter] = useState("");
  const [sessionFilter, setSessionFilter] = useState("");
  const [coachFilter, setCoachFilter] = useState("");

  const [showPastBookings, setShowPastBookings] = useState(false);
  const [view, setView] = useState<ScheduleView>("calendar");

  const [selectedDate, setSelectedDate] = useState(todayManilaDateKey);
  const [calendarMonth, setCalendarMonth] = useState(() => {
    const today = todayManilaDateKey();

    return {
      year: Number(today.slice(0, 4)),
      month: Number(today.slice(5, 7)),
    };
  });

  const todayKey = todayManilaDateKey();

  useEffect(() => {
    if (!supabase) {
      setAuthStatus("signed_out");
      return;
    }

    let active = true;
    let authEventObserved = false;

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((_event, session) => {
      if (!active) return;

      authEventObserved = true;

      if (session?.access_token) {
        setAuthStatus("authenticated");
        setError("");
      } else {
        setAuthStatus("signed_out");
        setRecords([]);
      }
    });

    void supabase.auth
      .getSession()
      .then(({ data, error: sessionError }) => {
        if (!active || authEventObserved) return;

        if (sessionError) {
          setError("Could not restore your admin session. Try again.");
          setAuthStatus("unavailable");
          return;
        }

        setAuthStatus(
          data.session?.access_token ? "authenticated" : "signed_out",
        );
      })
      .catch(() => {
        if (!active || authEventObserved) return;

        setError("Could not restore your admin session. Try again.");
        setAuthStatus("unavailable");
      });

    return () => {
      active = false;
      subscription.unsubscribe();
    };
  }, [sessionRetry]);

  const loadClients = useCallback(async () => {
    if (!supabase) return;

    setLoadStatus("loading");
    setError("");

    try {
      const response = await fetchWithAdminSession(
        supabase.auth,
        fetch,
        "/api/backend/admin/approved-clients",
      );

      const data = await response.json().catch(() => ({}));

      if (response.status === 401) {
        void supabase.auth.signOut().catch(() => undefined);
        setAuthStatus("signed_out");
        setRecords([]);
        throw new Error(expiredSessionMessage);
      }

      if (response.status === 403) {
        throw new Error(
          "This account is not authorized to access approved clients.",
        );
      }

      if (!response.ok) {
        throw new Error(
          typeof data?.error === "string"
            ? data.error
            : "Could not load approved clients.",
        );
      }

      setRecords(Array.isArray(data.clients) ? data.clients : []);
      setLoadStatus("ready");
    } catch (cause) {
      const message =
        cause instanceof Error
          ? cause.message
          : "Could not load approved clients.";

      if (message === expiredSessionMessage) {
        setAuthStatus("signed_out");
        setRecords([]);
      }

      setError(message);
      setLoadStatus("error");
    }
  }, []);

  useEffect(() => {
    if (authStatus === "authenticated") {
      void loadClients();
    }
  }, [authStatus, loadClients]);

  // Hide past sessions by default. Records remain in memory and in the database.
  const visibleRecords = useMemo(() => {
    if (showPastBookings) return records;

    return records.filter(
      (record) => manilaDateKey(record.starts_at) >= todayKey,
    );
  }, [records, showPastBookings, todayKey]);

  const hiddenPastBookingCount = records.length - visibleRecords.length;

  const programs = useMemo(
    () =>
      [...new Set(visibleRecords.map((record) => record.program_name))].sort(),
    [visibleRecords],
  );

  const sessions = useMemo(() => {
    const byId = new Map<string, ApprovedClient>();

    for (const record of visibleRecords) {
      if (!byId.has(record.session_id)) {
        byId.set(record.session_id, record);
      }
    }

    return [...byId.values()].sort((a, b) =>
      a.starts_at.localeCompare(b.starts_at),
    );
  }, [visibleRecords]);

  const coaches = useMemo(() => {
    const byId = new Map<string, string>();

    for (const record of visibleRecords) {
      if (record.assigned_coach_id) {
        byId.set(
          record.assigned_coach_id,
          record.assigned_coach_name || "Assigned coach",
        );
      }
    }

    return [...byId.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [visibleRecords]);

  const hasUnassignedCoach = visibleRecords.some(
    (record) => !record.assigned_coach_id,
  );

  const filteredRecords = useMemo(() => {
    const query = search.trim().toLowerCase();

    return visibleRecords
      .filter((record) => {
        const searchableText = [
          record.player_name,
          record.player_email,
          record.player_phone,
          record.program_name,
          record.location,
          record.assigned_coach_name,
          manilaDateKey(record.starts_at),
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();

        const sessionDate = manilaDateKey(record.starts_at);

        return (
          (!query || searchableText.includes(query)) &&
          (!dateFrom || sessionDate >= dateFrom) &&
          (!dateTo || sessionDate <= dateTo) &&
          (!programFilter || record.program_name === programFilter) &&
          (!sessionFilter || record.session_id === sessionFilter) &&
          (!coachFilter ||
            (coachFilter === "unassigned"
              ? !record.assigned_coach_id
              : record.assigned_coach_id === coachFilter))
        );
      })
      .sort((a, b) => a.starts_at.localeCompare(b.starts_at));
  }, [
    visibleRecords,
    search,
    dateFrom,
    dateTo,
    programFilter,
    sessionFilter,
    coachFilter,
  ]);

  const uniqueClientCount = useMemo(
    () => new Set(filteredRecords.map((record) => record.player_id)).size,
    [filteredRecords],
  );

  const recordsByDate = useMemo(() => {
    const grouped = new Map<string, ApprovedClient[]>();

    for (const record of filteredRecords) {
      const key = manilaDateKey(record.starts_at);
      const existing = grouped.get(key) ?? [];

      existing.push(record);
      grouped.set(key, existing);
    }

    return grouped;
  }, [filteredRecords]);

  const dates = useMemo(
    () => calendarDates(calendarMonth.year, calendarMonth.month),
    [calendarMonth],
  );

  const monthLabel = calendarMonthFormatter.format(
    new Date(Date.UTC(calendarMonth.year, calendarMonth.month - 1, 15, 12)),
  );

  const selectedDateLabel = selectedDateFormatter.format(
    new Date(`${selectedDate}T12:00:00+08:00`),
  );

  const selectedDateRecords = recordsByDate.get(selectedDate) ?? [];

  const hasResultsToDisplay = loadStatus === "ready" || records.length > 0;

  function resetFilters() {
    setSearch("");
    setDateFrom("");
    setDateTo("");
    setProgramFilter("");
    setSessionFilter("");
    setCoachFilter("");
  }

  function moveCalendarMonth(direction: number) {
    const next = new Date(
      Date.UTC(calendarMonth.year, calendarMonth.month - 1 + direction, 1, 12),
    );

    setCalendarMonth({
      year: next.getUTCFullYear(),
      month: next.getUTCMonth() + 1,
    });

    setSelectedDate(next.toISOString().slice(0, 10));
  }

  function selectCalendarDate(day: CalendarDate) {
    setSelectedDate(day.key);

    if (!day.inMonth) {
      const [year, month] = day.key.split("-").map(Number);
      setCalendarMonth({ year, month });
    }
  }

  function goToToday() {
    const today = todayManilaDateKey();
    const [year, month] = today.split("-").map(Number);

    setCalendarMonth({ year, month });
    setSelectedDate(today);
  }

  function signOut() {
    setRecords([]);
    setAuthStatus("signed_out");

    if (supabase) {
      void supabase.auth.signOut().catch(() => undefined);
    }
  }

  return (
    <main className={`admin-page ${styles.page}`}>
      <header className="training-header">
        <Link className="brand" href="/training">
          <span className="brand-mark" aria-hidden="true">
            M
          </span>
          <span>
            <strong>MMSU</strong>
            <small>TENNIS CLUB</small>
          </span>
        </Link>

        <nav className="portal-nav" aria-label="Club navigation">
          <Link href="/admin">Admin workspace</Link>
          <Link href="/admin/coaches">Coaches</Link>
          <Link href="/admin/feedback">Feedback</Link>
          <Link href="/training">Public schedule</Link>
        </nav>
      </header>

      <section className={`admin-main ${styles.main}`}>
        {authStatus === "checking" ? (
          <div className="admin-state" role="status">
            Checking your admin session…
          </div>
        ) : authStatus === "unavailable" ? (
          <section className="admin-state form-error" role="alert">
            <p>{error || "Could not check your admin session."}</p>
            <button
              className="admin-button"
              type="button"
              onClick={() => {
                setAuthStatus("checking");
                setSessionRetry((value) => value + 1);
              }}
            >
              Retry session check
            </button>
          </section>
        ) : authStatus === "signed_out" ? (
          <section className="admin-login">
            <p className="section-kicker">CLUB ADMIN</p>
            <h1>Sign in required</h1>
            <p>
              Sign in to the admin workspace before viewing confirmed client
              bookings.
            </p>
            <Link className="admin-button admin-button-link" href="/admin">
              Go to admin sign in
            </Link>
          </section>
        ) : (
          <>
            <div className={`admin-heading ${styles.pageHeading}`}>
              <div>
                <p className="section-kicker">CLUB RECORDS</p>
                <h1>Approved clients</h1>
                <p className={`admin-help ${styles.description}`}>
                  View confirmed training bookings and the players attending
                  each session. Past sessions are hidden by default, but remain
                  available for reference. All times are shown in Philippine
                  time.
                </p>
              </div>

              <div className={`admin-heading-actions ${styles.headingActions}`}>
                <button
                  className="admin-button secondary"
                  type="button"
                  disabled={loadStatus === "loading"}
                  onClick={() => void loadClients()}
                >
                  {loadStatus === "loading" ? "Refreshing…" : "Refresh"}
                </button>

                <button
                  className="admin-button secondary"
                  type="button"
                  onClick={signOut}
                >
                  Sign out
                </button>
              </div>
            </div>

            {error && loadStatus === "error" && (
              <section className="admin-state form-error" role="alert">
                <p>{error}</p>

                {records.length > 0 && (
                  <p>
                    Previously loaded bookings remain visible. Refresh to try
                    loading the latest data again.
                  </p>
                )}

                <button
                  className="admin-button secondary"
                  type="button"
                  onClick={() => void loadClients()}
                >
                  Retry
                </button>
              </section>
            )}

            {loadStatus === "loading" && records.length === 0 && (
              <div className="admin-state" role="status" aria-live="polite">
                Loading confirmed bookings…
              </div>
            )}

            {hasResultsToDisplay && (
              <>
                <section
                  className={`approved-stats ${styles.stats}`}
                  aria-label="Filtered booking totals"
                >
                  <div className={`approved-stat ${styles.stat}`}>
                    <span className={styles.statLabel}>Confirmed bookings</span>
                    <strong className={styles.statValue}>
                      {filteredRecords.length}
                    </strong>
                    <small className={styles.statFoot}>
                      Matching booking records
                    </small>
                  </div>

                  <div className={`approved-stat ${styles.stat}`}>
                    <span className={styles.statLabel}>Unique clients</span>
                    <strong className={styles.statValue}>
                      {uniqueClientCount}
                    </strong>
                    <small className={styles.statFoot}>
                      Distinct players in the current results
                    </small>
                  </div>
                </section>

                <section
                  className={`admin-card approved-filter-card ${styles.filterCard}`}
                  aria-labelledby="booking-filters-heading"
                >
                  <div className="schedule-section-heading">
                    <div>
                      <p className="section-kicker">FIND A BOOKING</p>
                      <h2 id="booking-filters-heading">Search and filter</h2>
                    </div>

                    <button
                      className="admin-button secondary"
                      type="button"
                      onClick={resetFilters}
                      disabled={
                        !search &&
                        !dateFrom &&
                        !dateTo &&
                        !programFilter &&
                        !sessionFilter &&
                        !coachFilter
                      }
                    >
                      Clear filters
                    </button>
                  </div>

                  <div
                    className={`schedule-form approved-filters ${styles.filterGrid}`}
                  >
                    <label
                      className={`approved-filter-search ${styles.searchField}`}
                    >
                      Search players and bookings
                      <input
                        type="search"
                        value={search}
                        onChange={(event) => setSearch(event.target.value)}
                        placeholder="Name, email, phone, venue or coach"
                      />
                    </label>

                    <label>
                      Session date from
                      <input
                        type="date"
                        value={dateFrom}
                        max={dateTo || undefined}
                        onChange={(event) => setDateFrom(event.target.value)}
                      />
                    </label>

                    <label>
                      Session date to
                      <input
                        type="date"
                        value={dateTo}
                        min={dateFrom || undefined}
                        onChange={(event) => setDateTo(event.target.value)}
                      />
                    </label>

                    <label>
                      Training program
                      <select
                        value={programFilter}
                        onChange={(event) =>
                          setProgramFilter(event.target.value)
                        }
                      >
                        <option value="">All programs</option>
                        {programs.map((program) => (
                          <option key={program} value={program}>
                            {program}
                          </option>
                        ))}
                      </select>
                    </label>

                    <label>
                      Training session
                      <select
                        value={sessionFilter}
                        onChange={(event) =>
                          setSessionFilter(event.target.value)
                        }
                      >
                        <option value="">All sessions</option>
                        {sessions.map((session) => (
                          <option
                            key={session.session_id}
                            value={session.session_id}
                          >
                            {formatSessionDate(session.starts_at)} ·{" "}
                            {session.program_name} · {session.location}
                          </option>
                        ))}
                      </select>
                    </label>

                    <label>
                      Assigned coach
                      <select
                        value={coachFilter}
                        onChange={(event) => setCoachFilter(event.target.value)}
                      >
                        <option value="">All coaches</option>
                        {coaches.map(([id, name]) => (
                          <option key={id} value={id}>
                            {name}
                          </option>
                        ))}
                        {hasUnassignedCoach && (
                          <option value="unassigned">Not assigned</option>
                        )}
                      </select>
                    </label>
                  </div>

                  <div className={styles.archiveControl}>
                    <label>
                      <input
                        type="checkbox"
                        checked={showPastBookings}
                        onChange={(event) =>
                          setShowPastBookings(event.target.checked)
                        }
                      />
                      <span>Show past bookings</span>
                    </label>

                    <p>
                      {!showPastBookings && hiddenPastBookingCount > 0
                        ? `${countLabel(hiddenPastBookingCount, "past booking")} hidden. `
                        : ""}
                      Past bookings are hidden from this view, not deleted.
                    </p>
                  </div>

                  <p className={`admin-help ${styles.filterSummary}`}>
                    Showing {filteredRecords.length} of {visibleRecords.length}{" "}
                    visible confirmed{" "}
                    {visibleRecords.length === 1 ? "booking" : "bookings"}.
                  </p>
                </section>

                <section
                  className={styles.resultsSection}
                  aria-labelledby="bookings-heading"
                >
                  <div className={styles.resultsHeader}>
                    <div>
                      <p className="section-kicker">CONFIRMED BOOKINGS</p>
                      <h2 id="bookings-heading">Bookings</h2>
                      <p className={styles.resultsDescription}>
                        {countLabel(filteredRecords.length, "booking")} across{" "}
                        {countLabel(uniqueClientCount, "unique client")}.
                      </p>
                    </div>

                    <div
                      className={`schedule-view-switch ${styles.viewSwitch}`}
                      role="group"
                      aria-label="Booking display"
                    >
                      <button
                        type="button"
                        className={view === "list" ? "active" : ""}
                        aria-pressed={view === "list"}
                        onClick={() => setView("list")}
                      >
                        List
                      </button>

                      <button
                        type="button"
                        className={view === "calendar" ? "active" : ""}
                        aria-pressed={view === "calendar"}
                        onClick={() => setView("calendar")}
                      >
                        Calendar
                      </button>
                    </div>
                  </div>

                  {view === "list" ? (
                    filteredRecords.length === 0 ? (
                      <div className="admin-state">
                        {records.length === 0
                          ? "No confirmed bookings have been recorded yet."
                          : visibleRecords.length === 0 && !showPastBookings
                            ? "There are no current or upcoming bookings. Enable Show past bookings to view historical records."
                            : "No bookings match your current filters. Try changing your search."}
                      </div>
                    ) : (
                      <div
                        className={`admin-list approved-client-list ${styles.bookingList}`}
                      >
                        {filteredRecords.map((record) => (
                          <BookingCard
                            key={record.booking_id}
                            record={record}
                          />
                        ))}
                      </div>
                    )
                  ) : (
                    <div className={styles.calendarLayout}>
                      <section
                        className={styles.calendarPanel}
                        aria-label="Confirmed booking calendar"
                      >
                        <div className="calendar-toolbar">
                          <button
                            type="button"
                            aria-label="Previous month"
                            onClick={() => moveCalendarMonth(-1)}
                          >
                            ‹
                          </button>

                          <h3>{monthLabel}</h3>

                          <button
                            type="button"
                            aria-label="Next month"
                            onClick={() => moveCalendarMonth(1)}
                          >
                            ›
                          </button>
                        </div>

                        <div
                          className={`training-calendar admin-calendar ${styles.bookingCalendar}`}
                          role="grid"
                          aria-label={`Confirmed bookings for ${monthLabel}`}
                        >
                          {weekDayLabels.map((day, index) => (
                            <div
                              className="calendar-weekday"
                              role="columnheader"
                              key={`${day}-${index}`}
                            >
                              {day}
                            </div>
                          ))}

                          {dates.map((day) => {
                            const dayRecords = recordsByDate.get(day.key) ?? [];
                            const isToday = day.key === todayKey;
                            const isSelected = selectedDate === day.key;

                            const dayClassNames = [
                              "calendar-day",
                              !day.inMonth ? "outside-month" : "",
                              dayRecords.length > 0 ? "has-sessions" : "",
                              isSelected ? "selected" : "",
                              isToday ? styles.todayCell : "",
                            ]
                              .filter(Boolean)
                              .join(" ");

                            return (
                              <button
                                className={dayClassNames}
                                type="button"
                                role="gridcell"
                                aria-selected={isSelected}
                                aria-label={`${day.key}, ${countLabel(
                                  dayRecords.length,
                                  "confirmed booking",
                                )}${isToday ? ", today" : ""}`}
                                key={day.key}
                                onClick={() => selectCalendarDate(day)}
                              >
                                <span className="calendar-day-number">
                                  {day.day}
                                </span>

                                {dayRecords.length > 0 && (
                                  <>
                                    <span className="calendar-day-count">
                                      {dayRecords.length}{" "}
                                      {dayRecords.length === 1
                                        ? "booking"
                                        : "bookings"}
                                    </span>

                                    <span
                                      className="calendar-day-dots"
                                      aria-hidden="true"
                                    >
                                      {dayRecords.slice(0, 3).map((record) => (
                                        <i
                                          className="dot-open"
                                          key={record.booking_id}
                                        />
                                      ))}
                                    </span>
                                  </>
                                )}
                              </button>
                            );
                          })}
                        </div>

                        <div className={styles.calendarFooter}>
                          <span>
                            <i aria-hidden="true" />
                            Confirmed booking
                          </span>

                          <button
                            className="admin-button secondary"
                            type="button"
                            onClick={goToToday}
                          >
                            Today
                          </button>
                        </div>
                      </section>

                      <section
                        className={`admin-day-agenda ${styles.dayAgenda}`}
                        aria-labelledby="selected-date-heading"
                      >
                        <div className={styles.agendaHeader}>
                          <div>
                            <p className="section-kicker">SELECTED DATE</p>
                            <h3 id="selected-date-heading">
                              {selectedDateLabel}
                            </h3>
                            <p className={styles.resultsDescription}>
                              Confirmed bookings scheduled for this day.
                            </p>
                          </div>

                          <span className={styles.agendaCount}>
                            {selectedDateRecords.length}
                          </span>
                        </div>

                        {selectedDateRecords.length === 0 ? (
                          <div className="admin-state">
                            {records.length === 0
                              ? "No confirmed bookings have been recorded yet."
                              : visibleRecords.length === 0 && !showPastBookings
                                ? "There are no current or upcoming bookings. Enable Show past bookings to view historical records."
                                : filteredRecords.length === 0
                                  ? "No bookings match your current filters."
                                  : "No confirmed bookings on this date. Select another day to view its bookings."}
                          </div>
                        ) : (
                          <div
                            className={`admin-list approved-client-list ${styles.bookingList}`}
                          >
                            {selectedDateRecords.map((record) => (
                              <BookingCard
                                key={record.booking_id}
                                record={record}
                              />
                            ))}
                          </div>
                        )}
                      </section>
                    </div>
                  )}
                </section>
              </>
            )}
          </>
        )}
      </section>
    </main>
  );
}
