"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";

type Availability = {
  id: string;
  coach_name: string;
  starts_at: string;
  ends_at: string;
  notes: string | null;
};

type AssignedSession = {
  id: string;
  program_name: string;
  program_description: string | null;
  starts_at: string;
  ends_at: string;
  location: string;
  status: string;
  is_public: boolean;
};

type AuthState = "checking" | "signed_out" | "authenticated";

const supabase = getSupabaseBrowserClient();
const phtDateKey = (date: Date) => new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Manila",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
}).format(date);
const phtDateTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

function periodRange(period: "week" | "month", anchor: string) {
  const [year, month, day] = anchor.split("-").map(Number);
  const start = new Date(Date.UTC(year, month - 1, day));
  if (period === "week") {
    start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
  } else {
    start.setUTCDate(1);
  }
  const end = new Date(start);
  if (period === "week") end.setUTCDate(end.getUTCDate() + 7);
  else end.setUTCMonth(end.getUTCMonth() + 1);
  return { start, end, startKey: start.toISOString().slice(0, 10), endKey: end.toISOString().slice(0, 10) };
}

function apiDateTime(day: string, time: string) {
  const date = new Date(`${day}T${time}:00+08:00`);
  return Number.isNaN(date.valueOf()) ? null : date;
}

async function responseData(response: Response) {
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) throw new Error("Your session has expired. Please sign in again.");
    if (response.status === 403) throw new Error("This account is not assigned to a coach.");
    throw new Error(typeof body?.error === "string" ? body.error : "The scheduling request failed.");
  }
  return body;
}

export default function CoachPage() {
  const [authState, setAuthState] = useState<AuthState>("checking");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [period, setPeriod] = useState<"week" | "month">("week");
  const [anchor, setAnchor] = useState(() => phtDateKey(new Date()));
  const [availability, setAvailability] = useState<Availability[]>([]);
  const [assignedSessions, setAssignedSessions] = useState<AssignedSession[]>([]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [roleDenied, setRoleDenied] = useState(false);
  const requestGeneration = useRef(0);
  const range = periodRange(period, anchor);
  const periodAvailability = availability.filter((item) => {
    const key = phtDateKey(new Date(item.starts_at));
    return key >= range.startKey && key < range.endKey;
  });

  const coachFetch = useCallback((input: RequestInfo | URL, init?: RequestInit) => {
    if (!supabase) throw new Error("Coach sign-in is not configured. Contact the site administrator.");
    return fetchWithAdminSession(supabase.auth, fetch, input, init);
  }, []);

  const loadAvailability = useCallback(async () => {
    const generation = requestGeneration.current;
    setLoading(true);
    setError("");
    try {
      const response = await coachFetch("/api/backend/coach/availability");
      const data = await responseData(response);
      if (requestGeneration.current !== generation) return;
      setAvailability(Array.isArray(data.availability) ? data.availability : []);
      setRoleDenied(false);
    } catch (cause) {
      if (requestGeneration.current !== generation) return;
      const message = cause instanceof Error ? cause.message : "Could not load your availability.";
      setError(message);
      if (message.includes("not assigned to a coach")) setRoleDenied(true);
      if (message.includes("session has expired")) {
        setAuthState("signed_out");
        setAvailability([]);
        if (supabase) void supabase.auth.signOut().catch(() => undefined);
      }
    } finally {
      if (requestGeneration.current === generation) setLoading(false);
    }
  }, [coachFetch]);

  const loadAssignedSessions = useCallback(async () => {
    const generation = requestGeneration.current;
    try {
      const response = await coachFetch("/api/backend/coach/sessions");
      const data = await responseData(response);
      if (requestGeneration.current !== generation) return;
      setAssignedSessions(Array.isArray(data.sessions) ? data.sessions : []);
    } catch (cause) {
      if (requestGeneration.current !== generation) return;
      const message = cause instanceof Error ? cause.message : "Could not load assigned sessions.";
      setError(message);
      if (message.includes("not assigned to a coach")) setRoleDenied(true);
      if (message.includes("session has expired")) {
        setAuthState("signed_out");
        setAvailability([]);
        setAssignedSessions([]);
        if (supabase) void supabase.auth.signOut().catch(() => undefined);
      }
    }
  }, [coachFetch]);

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
        setAvailability([]);
        setAssignedSessions([]);
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
    if (authState === "authenticated") {
      void loadAvailability();
      void loadAssignedSessions();
    }
  }, [authState, loadAvailability, loadAssignedSessions]);

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

  async function addAvailability(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const startsAt = apiDateTime(String(values.get("day") || ""), String(values.get("starts_at") || ""));
    const endsAt = apiDateTime(String(values.get("day") || ""), String(values.get("ends_at") || ""));
    if (!startsAt || !endsAt || startsAt >= endsAt) {
      setError("End time must be after start time.");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await coachFetch("/api/backend/coach/availability", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          coach_name: values.get("coach_name"),
          starts_at: startsAt.toISOString(),
          ends_at: endsAt.toISOString(),
          notes: values.get("notes"),
        }),
      });
      await responseData(response);
      form.reset();
      setNotice("Availability saved. Admins will use it when creating actual sessions.");
      await loadAvailability();
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not save availability.";
      setError(message);
      if (message.includes("session has expired")) signOut();
    } finally {
      setBusy(false);
    }
  }

  async function removeAvailability(id: string) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await responseData(await coachFetch(`/api/backend/coach/availability/${id}`, { method: "DELETE" }));
      setAvailability((current) => current.filter((item) => item.id !== id));
      setNotice("Availability removed.");
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not remove availability.";
      setError(message);
      if (message.includes("session has expired")) signOut();
    } finally {
      setBusy(false);
    }
  }

  function movePeriod(direction: number) {
    const start = range.start;
    if (period === "week") start.setUTCDate(start.getUTCDate() + 7 * direction);
    else start.setUTCMonth(start.getUTCMonth() + direction);
    setAnchor(start.toISOString().slice(0, 10));
  }

  function signOut() {
    requestGeneration.current += 1;
    setAuthState("signed_out");
    setAvailability([]);
    setAssignedSessions([]);
    setError("");
    setRoleDenied(false);
    if (supabase) void supabase.auth.signOut().catch(() => undefined);
  }

  const rangeLabel = period === "week"
    ? `${range.start.toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric" })} – ${new Date(range.end.getTime() - 86400000).toLocaleDateString("en-PH", { timeZone: "UTC", month: "short", day: "numeric", year: "numeric" })}`
    : range.start.toLocaleDateString("en-PH", { timeZone: "UTC", month: "long", year: "numeric" });

  return (
    <main className="admin-page">
      <header className="training-header">
        <Link className="brand" href="/training"><span className="brand-mark" aria-hidden="true">M</span><span><strong>MMSU</strong><small>TENNIS CLUB</small></span></Link>
        <nav className="portal-nav" aria-label="Club navigation"><Link href="/training">Public schedule</Link><Link href="/admin">Admin</Link></nav>
      </header>
      <section className="admin-main coach-main">
        {authState === "checking" ? <p className="admin-state" role="status">Checking your session…</p>
          : authState === "signed_out" ? <div className="admin-login">
            <p className="section-kicker">COACH PORTAL</p><h1>Sign in to share your availability.</h1>
            <p className="admin-help">Coach accounts are provisioned by the club. Availability is not a published training session.</p>
            <form onSubmit={signIn}>
              <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required /></label>
              <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="admin-button" disabled={busy} type="submit">{busy ? "Signing in…" : "Sign in"}</button>
            </form>
          </div> : <>
            <div className="admin-heading">
              <div><p className="section-kicker">COACH PORTAL</p><h1>Your availability</h1><p className="admin-help">Add the times you can coach. Admins review availability separately and create actual sessions for customers to book.</p></div>
              <button className="admin-button secondary" type="button" onClick={signOut}>Sign out</button>
            </div>
            {error && <p className="admin-state form-error" role="alert">{error}</p>}
            {notice && <p className="admin-state form-success" role="status">{notice}</p>}
            {roleDenied ? <p className="admin-state form-error">This account is not assigned to a coach. Contact a club admin if you need access.</p> : <>
              <section className="assigned-sessions-note">
                <div className="schedule-section-heading"><div><p className="section-kicker">ADMIN ASSIGNMENTS</p><h2>Assigned sessions</h2></div><button className="admin-button secondary" type="button" disabled={loading || busy} onClick={() => { void loadAssignedSessions(); }}>Refresh</button></div>
                {assignedSessions.length === 0 ? <p className="admin-state">No sessions have been assigned to you yet. Availability submissions are planning input and do not create sessions.</p>
                  : <div className="availability-list">{assignedSessions.map((session) => <article className="availability-card assigned-session-card" key={session.id}>
                    <div><strong>{session.program_name}</strong><p>{phtDateTime.format(new Date(session.starts_at))} – {new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", hour: "numeric", minute: "2-digit" }).format(new Date(session.ends_at))} · {session.location}</p>{session.program_description && <small>{session.program_description}</small>}<div className="session-status-row"><span className={`session-status session-status-${session.status}`}>{session.status}</span><span className={`session-status ${session.is_public ? "session-status-published" : "session-status-hidden"}`}>{session.is_public ? "Published" : "Hidden from customers"}</span></div></div>
                  </article>)}</div>}
              </section>
              <div className="availability-toolbar">
                <div className="schedule-tabs" role="group" aria-label="Availability period">
                  <button type="button" className={period === "week" ? "active" : ""} aria-pressed={period === "week"} onClick={() => setPeriod("week")}>Week</button>
                  <button type="button" className={period === "month" ? "active" : ""} aria-pressed={period === "month"} onClick={() => setPeriod("month")}>Month</button>
                </div>
                <div className="period-navigation"><button className="admin-button secondary" type="button" aria-label="Previous period" onClick={() => movePeriod(-1)}>‹</button><strong>{rangeLabel}</strong><button className="admin-button secondary" type="button" aria-label="Next period" onClick={() => movePeriod(1)}>›</button></div>
              </div>
              <section className="availability-panel">
                <h2>Add a time</h2>
                <form className="schedule-form" key={`${period}-${range.startKey}`} onSubmit={addAvailability}>
                  <label>Coach name<input name="coach_name" autoComplete="name" maxLength={100} required /></label>
                  <label>Date<input name="day" type="date" min={range.startKey} max={new Date(range.end.getTime() - 86400000).toISOString().slice(0, 10)} defaultValue={range.startKey} required /></label>
                  <label>Start time<input name="starts_at" type="time" required /></label>
                  <label>End time<input name="ends_at" type="time" required /></label>
                  <label className="schedule-form-notes">Notes (optional)<input name="notes" maxLength={2000} /></label>
                  <button className="admin-button" type="submit" disabled={busy}>{busy ? "Saving…" : "Add availability"}</button>
                </form>
              </section>
              <section className="availability-panel">
                <div className="schedule-section-heading"><h2>Submitted availability</h2><button className="admin-button secondary" type="button" disabled={loading || busy} onClick={() => { void loadAvailability(); }}>{loading ? "Loading…" : "Refresh"}</button></div>
                {loading && availability.length === 0 ? <p className="admin-state">Loading availability…</p>
                  : periodAvailability.length === 0 ? <p className="admin-state">No availability submitted for this {period}.</p>
                    : <div className="availability-list">{periodAvailability.map((item) => <article className="availability-card" key={item.id}>
                      <div><strong>{item.coach_name}</strong><p>{phtDateTime.format(new Date(item.starts_at))} – {new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", hour: "numeric", minute: "2-digit" }).format(new Date(item.ends_at))}</p>{item.notes && <small>{item.notes}</small>}</div>
                      <button className="admin-button secondary" type="button" disabled={busy} onClick={() => { void removeAvailability(item.id); }}>Remove</button>
                    </article>)}</div>}
              </section>
            </>}
          </>}
      </section>
    </main>
  );
}
