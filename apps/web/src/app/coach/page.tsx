"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";
import {
  calendarGrid,
  periodRange,
  phtDateKey,
  rosterLabel,
  sessionsInRange,
  shiftPeriod,
  sortSessionsChronologically,
  summarizeSchedule,
} from "@/lib/coach-portal.mjs";
import { scrollToElement } from "@/lib/scroll-to.mjs";

type ScheduleSession = {
  id: string;
  program_name: string;
  program_description: string | null;
  starts_at: string;
  ends_at: string;
  location: string;
  status: string;
  is_public: boolean;
  assigned_coach_id: string | null;
  assigned_coach_name: string | null;
  participant_names: string[];
  confirmed_booking_count: number;
};

type CoachDirectoryEntry = {
  id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
  notes: string | null;
  is_active: boolean;
};

type AuthState = "checking" | "signed_out" | "authenticated";
type Tab = "dashboard" | "schedule" | "coaches";
type ScheduleView = "list" | "calendar";

const supabase = getSupabaseBrowserClient();
const weekDayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const coachMonth = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  month: "long",
  year: "numeric",
});
const coachLongDate = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "long",
  month: "long",
  day: "numeric",
  year: "numeric",
});
const phtDateTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});
const phtEndTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  hour: "numeric",
  minute: "2-digit",
});

async function responseData(response: Response) {
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) throw new Error("Your session has expired. Please sign in again.");
    if (response.status === 403) throw new Error("This account is not assigned to a coach.");
    throw new Error(typeof body?.error === "string" ? body.error : "The request failed.");
  }
  return body;
}

export default function CoachPage() {
  const [authState, setAuthState] = useState<AuthState>("checking");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [tab, setTab] = useState<Tab>("dashboard");
  const [period, setPeriod] = useState<"week" | "month">("week");
  const [anchor, setAnchor] = useState(() => phtDateKey(new Date()));
  const [view, setView] = useState<ScheduleView>("calendar");
  const [calendarMonth, setCalendarMonth] = useState(() => {
    const today = phtDateKey(new Date());
    return { year: Number(today.slice(0, 4)), month: Number(today.slice(5, 7)) };
  });
  const [selectedDay, setSelectedDay] = useState(() => phtDateKey(new Date()));
  const [sessions, setSessions] = useState<ScheduleSession[]>([]);
  const [coaches, setCoaches] = useState<CoachDirectoryEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [roleDenied, setRoleDenied] = useState(false);
  const requestGeneration = useRef(0);

  const range = useMemo(() => periodRange(period, anchor), [period, anchor]);
  const summary = useMemo(() => summarizeSchedule(sessions), [sessions]);
  const periodSessions = useMemo(() => sessionsInRange(sessions, range), [sessions, range]);
  const calendarMonthKey = `${calendarMonth.year}-${String(calendarMonth.month).padStart(2, "0")}`;
  const calendarCells = useMemo(
    () => calendarGrid(calendarMonth.year, calendarMonth.month),
    [calendarMonth],
  );
  const calendarSessions = useMemo(
    () => sessions.filter((session) => phtDateKey(session.starts_at).startsWith(calendarMonthKey)),
    [sessions, calendarMonthKey],
  );
  const selectedDaySessions = useMemo(
    () =>
      sortSessionsChronologically(
        calendarSessions.filter((session) => phtDateKey(session.starts_at) === selectedDay),
      ),
    [calendarSessions, selectedDay],
  );
  const upcoming = useMemo(
    () => sortSessionsChronologically(sessions.filter((session) => Date.parse(session.starts_at) >= Date.now())).slice(0, 5),
    [sessions],
  );

  const coachFetch = useCallback((input: RequestInfo | URL, init?: RequestInit) => {
    if (!supabase) throw new Error("Coach sign-in is not configured. Contact the site administrator.");
    return fetchWithAdminSession(supabase.auth, fetch, input, init);
  }, []);

  const handleSessionExpired = useCallback(() => {
    requestGeneration.current += 1;
    setAuthState("signed_out");
    setSessions([]);
    setCoaches([]);
    if (supabase) void supabase.auth.signOut().catch(() => undefined);
  }, []);

  const loadPortalData = useCallback(async () => {
    const generation = requestGeneration.current;
    setLoading(true);
    setError("");
    try {
      const [scheduleResponse, coachesResponse] = await Promise.all([
        coachFetch("/api/backend/coach/schedule"),
        coachFetch("/api/backend/coach/coaches"),
      ]);
      const scheduleData = await responseData(scheduleResponse);
      const coachesData = await responseData(coachesResponse);
      if (requestGeneration.current !== generation) return;
      setSessions(Array.isArray(scheduleData.sessions) ? scheduleData.sessions : []);
      setCoaches(Array.isArray(coachesData.coaches) ? coachesData.coaches : []);
      setRoleDenied(false);
    } catch (cause) {
      if (requestGeneration.current !== generation) return;
      const message = cause instanceof Error ? cause.message : "Could not load the coach portal.";
      setError(message);
      if (message.includes("not assigned to a coach")) setRoleDenied(true);
      if (message.includes("session has expired")) handleSessionExpired();
    } finally {
      if (requestGeneration.current === generation) setLoading(false);
    }
  }, [coachFetch, handleSessionExpired]);

  useEffect(() => {
    if (!supabase) {
      setAuthState("signed_out");
      return;
    }
    let active = true;
    let authEventObserved = false;
    const { data: { subscription } } = supabase.auth.onAuthStateChange((event, session) => {
      if (!active) return;
      authEventObserved = true;
      if (["INITIAL_SESSION", "SIGNED_IN", "TOKEN_REFRESHED", "USER_UPDATED"].includes(event)) {
        setAuthState(session?.access_token ? "authenticated" : "signed_out");
      }
      if (event === "SIGNED_OUT" || !session?.access_token) {
        requestGeneration.current += 1;
        setAuthState("signed_out");
        setSessions([]);
        setCoaches([]);
      }
    });
    void supabase.auth.getSession().then(({ data, error: sessionError }) => {
      if (!active || authEventObserved) return;
      if (sessionError) setError("Could not restore your session. Try again.");
      setAuthState(data.session?.access_token ? "authenticated" : "signed_out");
    }).catch(() => {
      if (active && !authEventObserved) {
        setError("Could not restore your session. Try again.");
        setAuthState("signed_out");
      }
    });
    return () => {
      active = false;
      requestGeneration.current += 1;
      subscription.unsubscribe();
    };
  }, []);

  useEffect(() => {
    if (authState === "authenticated") void loadPortalData();
  }, [authState, loadPortalData]);

  async function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!supabase) {
      setError("Coach sign-in is not configured. Contact the site administrator.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const { data, error: authError } = await supabase.auth.signInWithPassword({ email: email.trim(), password });
      if (authError || !data.session?.access_token) throw new Error("Email or password is incorrect, or sign-in is unavailable.");
      setPassword("");
      setAuthState("authenticated");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Sign-in is unavailable.");
    } finally {
      setBusy(false);
    }
  }

  function signOut() {
    requestGeneration.current += 1;
    setAuthState("signed_out");
    setSessions([]);
    setCoaches([]);
    setError("");
    setRoleDenied(false);
    setEmail("");
    setPassword("");
    setBusy(false);
    setLoading(false);
    if (supabase) void supabase.auth.signOut().catch(() => undefined);
  }

  function moveCoachMonth(direction: number) {
    const next = new Date(Date.UTC(calendarMonth.year, calendarMonth.month - 1 + direction, 1));
    const nextMonth = { year: next.getUTCFullYear(), month: next.getUTCMonth() + 1 };
    setCalendarMonth(nextMonth);
    setSelectedDay(`${nextMonth.year}-${String(nextMonth.month).padStart(2, "0")}-01`);
  }

  return (
    <main className="admin-page">
      <header className="training-header">
        <Link className="brand" href="/training"><span className="brand-mark" aria-hidden="true">M</span><span><strong>MMSU</strong><small>TENNIS CLUB</small></span></Link>
        <nav className="portal-nav" aria-label="Club navigation"><Link href="/training">Public schedule</Link><Link href="/admin">Admin</Link></nav>
      </header>
      <section className="admin-main coach-main">
        {authState === "checking" ? <p className="admin-state" role="status">Checking your session…</p>
          : authState === "signed_out" ? <div className="admin-login">
            <p className="section-kicker">COACH PORTAL</p><h1>Sign in to view the club schedule.</h1>
            <p className="admin-help">Coach accounts are provisioned by the club. The schedule and coach directory are read-only here.</p>
            <form onSubmit={signIn}>
              <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required /></label>
              <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="admin-button" disabled={busy} type="submit">{busy ? "Signing in…" : "Sign in"}</button>
            </form>
          </div> : <>
            <div className="admin-heading">
              <div><p className="section-kicker">COACH PORTAL</p><h1>Coach workspace</h1><p className="admin-help">See the club schedule, assigned coaches, and each session&apos;s confirmed participants. Records are managed by admins.</p></div>
              <button className="admin-button secondary" type="button" onClick={signOut}>Sign out</button>
            </div>
            {error && <p className="admin-state form-error" role="alert">{error}</p>}
            {roleDenied ? <p className="admin-state form-error">This account is not assigned to a coach. Contact a club admin if you need access.</p> : <>
              <div className="coach-tabs" role="tablist" aria-label="Coach portal sections">
                <button type="button" role="tab" aria-selected={tab === "dashboard"} className={tab === "dashboard" ? "active" : ""} onClick={() => setTab("dashboard")}>Dashboard</button>
                <button type="button" role="tab" aria-selected={tab === "schedule"} className={tab === "schedule" ? "active" : ""} onClick={() => setTab("schedule")}>Schedule</button>
                <button type="button" role="tab" aria-selected={tab === "coaches"} className={tab === "coaches" ? "active" : ""} onClick={() => setTab("coaches")}>Coaches</button>
              </div>

              {loading && sessions.length === 0 ? <p className="admin-state" role="status">Loading the club schedule…</p> : null}

              {tab === "dashboard" ? <section className="coach-panel" aria-label="Dashboard">
                <div className="coach-stat-grid">
                  <article className="coach-stat"><span>Total sessions</span><strong>{summary.total}</strong></article>
                  <article className="coach-stat"><span>Upcoming</span><strong>{summary.upcoming}</strong></article>
                  <article className="coach-stat"><span>Coach assigned</span><strong>{summary.assigned}</strong></article>
                  <article className="coach-stat"><span>Needs a coach</span><strong>{summary.unassigned}</strong></article>
                  <article className="coach-stat"><span>Confirmed participants</span><strong>{summary.participantCount}</strong></article>
                </div>
                <div className="schedule-section-heading"><div><p className="section-kicker">UP NEXT</p><h2>Next sessions</h2></div><button className="admin-button secondary" type="button" disabled={loading} onClick={() => { void loadPortalData(); }}>{loading ? "Loading…" : "Refresh"}</button></div>
                {upcoming.length === 0 ? <p className="admin-state">No upcoming sessions scheduled.</p>
                  : <div className="coach-session-list">{upcoming.map((session) => <SessionCard key={session.id} session={session} />)}</div>}
              </section> : null}

              {tab === "schedule" ? <section className="coach-panel" aria-label="Schedule">
                <div className="availability-toolbar">
                  <div className="schedule-tabs" role="group" aria-label="Schedule view">
                    <button type="button" className={view === "list" ? "active" : ""} aria-pressed={view === "list"} onClick={() => setView("list")}>List</button>
                    <button type="button" className={view === "calendar" ? "active" : ""} aria-pressed={view === "calendar"} onClick={() => setView("calendar")}>Calendar</button>
                  </div>
                  {view === "list" ? <>
                    <div className="schedule-tabs" role="group" aria-label="Schedule period">
                      <button type="button" className={period === "week" ? "active" : ""} aria-pressed={period === "week"} onClick={() => setPeriod("week")}>Week</button>
                      <button type="button" className={period === "month" ? "active" : ""} aria-pressed={period === "month"} onClick={() => setPeriod("month")}>Month</button>
                    </div>
                    <div className="period-navigation">
                      <button className="admin-button secondary" type="button" aria-label="Previous period" onClick={() => setAnchor((current) => shiftPeriod(period, current, -1) || current)}>‹</button>
                      <strong>{range?.label ?? ""}</strong>
                      <button className="admin-button secondary" type="button" aria-label="Next period" onClick={() => setAnchor((current) => shiftPeriod(period, current, 1) || current)}>›</button>
                    </div>
                  </> : <div className="period-navigation">
                    <button className="admin-button secondary" type="button" aria-label="Previous month" onClick={() => moveCoachMonth(-1)}>‹</button>
                    <strong>{coachMonth.format(new Date(Date.UTC(calendarMonth.year, calendarMonth.month - 1, 1)))}</strong>
                    <button className="admin-button secondary" type="button" aria-label="Next month" onClick={() => moveCoachMonth(1)}>›</button>
                  </div>}
                </div>
                {view === "calendar" ? <>
                  <div className="training-calendar admin-calendar" role="grid" aria-label={`Club schedule for ${coachMonth.format(new Date(Date.UTC(calendarMonth.year, calendarMonth.month - 1, 1)))}`}>
                    {weekDayLabels.map((weekday) => <div className="calendar-weekday" role="columnheader" key={weekday}>{weekday}</div>)}
                    {calendarCells.map((day) => {
                      const daySessions = calendarSessions.filter((session) => phtDateKey(session.starts_at) === day.key);
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
                              scrollToElement("coach-selected-date");
                            });
                          }}
                        >
                          <span className="calendar-day-number">{day.day}</span>
                          {daySessions.length > 0 && <span className="calendar-day-count">{daySessions.length} {daySessions.length === 1 ? "session" : "sessions"}</span>}
                          <span className="calendar-day-dots" aria-hidden="true">
                            {daySessions.slice(0, 3).map((session) => <i className={`dot-${session.status === "cancelled" ? "closed" : session.is_public ? "open" : "full"}`} key={session.id} />)}
                          </span>
                        </button>
                      );
                    })}
                  </div>
                  <div className="admin-day-agenda" id="coach-selected-date" style={{ scrollMarginTop: "24px" }}>
                    <h3>{selectedDay ? coachLongDate.format(new Date(`${selectedDay}T12:00:00+08:00`)) : "Choose a date"}</h3>
                    {selectedDaySessions.length === 0 ? <p className="admin-state">No sessions scheduled on this date.</p>
                      : <div className="coach-session-list">{selectedDaySessions.map((session) => <SessionCard key={session.id} session={session} />)}</div>}
                  </div>
                </> : periodSessions.length === 0 ? <p className="admin-state">No sessions scheduled for this {period}.</p>
                  : <div className="coach-session-list">{periodSessions.map((session) => <SessionCard key={session.id} session={session} />)}</div>}
              </section> : null}

              {tab === "coaches" ? <section className="coach-panel" aria-label="Coach directory">
                <div className="schedule-section-heading"><div><p className="section-kicker">COACH DIRECTORY</p><h2>Club coaches</h2></div></div>
                {coaches.length === 0 ? <p className="admin-state">No coaches are listed yet.</p>
                  : <div className="coach-directory-grid">{coaches.map((coach) => <article className="coach-directory-card" key={coach.id}>
                    <strong>{coach.full_name}</strong>
                    <p>{coach.email || "No email on file"}</p>
                    {coach.phone && <small>{coach.phone}</small>}
                    {coach.notes && <p className="coach-directory-notes">{coach.notes}</p>}
                  </article>)}</div>}
              </section> : null}
            </>}
          </>}
      </section>
    </main>
  );
}

function SessionCard({ session }: { session: ScheduleSession }) {
  const roster = rosterLabel(session);
  return (
    <article className="availability-card coach-session-card">
      <div>
        <strong>{session.program_name}</strong>
        <p>{phtDateTime.format(new Date(session.starts_at))} – {phtEndTime.format(new Date(session.ends_at))} · {session.location}</p>
        {session.program_description && <small>{session.program_description}</small>}
        <div className="session-status-row">
          <span className={`session-status session-status-${session.status}`}>{session.status}</span>
          <span className={`session-status ${session.is_public ? "session-status-published" : "session-status-hidden"}`}>{session.is_public ? "Published" : "Hidden from customers"}</span>
          <span className="session-status">{session.assigned_coach_name ? `Coach ${session.assigned_coach_name}` : "No coach assigned"}</span>
          <span className="session-status">{session.confirmed_booking_count} confirmed</span>
        </div>
        <p className="coach-roster"><strong>Participants:</strong> {roster}</p>
      </div>
    </article>
  );
}
