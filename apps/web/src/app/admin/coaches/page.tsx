"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";
import ClubMobileMenu from "@/components/club-mobile-menu";

type Coach = {
  id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
  notes: string | null;
  is_active: boolean;
  assigned_session_count: number;
};

type AuthStatus = "checking" | "authenticated" | "signed_out";

const supabase = getSupabaseBrowserClient();
const expiredSessionMessage = "Your admin session has expired. Please sign in again.";

export default function AdminCoachesPage() {
  const [authStatus, setAuthStatus] = useState<AuthStatus>("checking");
  const [coaches, setCoaches] = useState<Coach[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [editingID, setEditingID] = useState("");

  useEffect(() => {
    if (!supabase) {
      setAuthStatus("signed_out");
      return;
    }
    let active = true;
    let authEventObserved = false;
    const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
      if (!active) return;
      authEventObserved = true;
      if (session?.access_token) {
        setAuthStatus("authenticated");
        setError("");
      } else {
        setAuthStatus("signed_out");
        setCoaches([]);
      }
    });
    void supabase.auth.getSession().then(({ data, error: sessionError }) => {
      if (!active || authEventObserved) return;
      if (sessionError) setError("Could not restore your admin session. Try again.");
      setAuthStatus(data.session?.access_token ? "authenticated" : "signed_out");
    }).catch(() => {
      if (!active || authEventObserved) return;
      setError("Could not restore your admin session. Try again.");
      setAuthStatus("signed_out");
    });
    return () => {
      active = false;
      subscription.unsubscribe();
    };
  }, []);

  const adminFetch = useCallback(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (!supabase) throw new Error("Admin sign-in is not configured.");
    const response = await fetchWithAdminSession(supabase.auth, fetch, input, init);
    if (response.status === 401) {
      void supabase.auth.signOut().catch(() => undefined);
      setAuthStatus("signed_out");
      throw new Error(expiredSessionMessage);
    }
    return response;
  }, []);

  const loadCoaches = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await adminFetch("/api/backend/admin/coaches");
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(typeof data?.error === "string" ? data.error : "Could not load coaches.");
      }
      setCoaches(Array.isArray(data.coaches) ? data.coaches : []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load coaches.");
    } finally {
      setLoading(false);
    }
  }, [adminFetch]);

  useEffect(() => {
    if (authStatus === "authenticated") void loadCoaches();
  }, [authStatus, loadCoaches]);

  async function createCoach(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await adminFetch("/api/backend/admin/coaches", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          full_name: values.get("full_name"),
          email: values.get("email"),
          phone: values.get("phone"),
          notes: values.get("notes"),
        }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(typeof data?.error === "string" ? data.error : "Could not add the coach.");
      form.reset();
      setNotice("Coach added to the directory.");
      await loadCoaches();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not add the coach.");
    } finally {
      setBusy(false);
    }
  }

  async function saveCoach(coach: Coach, values: { fullName: string; email: string; phone: string; notes: string; isActive: boolean }) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await adminFetch(`/api/backend/admin/coaches/${coach.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          full_name: values.fullName,
          email: values.email,
          phone: values.phone,
          notes: values.notes,
          is_active: values.isActive,
        }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(typeof data?.error === "string" ? data.error : "Could not save the coach.");
      setNotice(`Saved ${coach.full_name}.`);
      setEditingID("");
      await loadCoaches();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not save the coach.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="admin-page">
      <header className="training-header">
        <Link className="brand" href="/" aria-label="MMSU Tennis Club home"><span className="brand-mark" aria-hidden="true">M</span><span><strong>MMSU</strong><small>TENNIS CLUB</small></span></Link>
        <nav className="portal-nav" aria-label="Club navigation">
          <Link href="/admin">Admin workspace</Link>
          <Link href="/admin/approved-clients">Approved clients</Link>
          <Link href="/admin/feedback">Feedback</Link>
        </nav>
        <ClubMobileMenu />
      </header>
      <section className="admin-main">
        {authStatus === "checking" ? <p className="admin-state" role="status">Checking your session…</p>
          : authStatus === "signed_out" ? <div className="admin-login">
            <p className="section-kicker">ADMIN ONLY</p><h1>Sign in to manage coaches.</h1>
            <p className="admin-help">Coach records are managed here and used when assigning coaches to training sessions.</p>
            <p className="admin-help">Use the admin workspace to sign in.</p>
            <div className="admin-actions"><Link className="admin-button admin-button-link" href="/admin">Go to admin sign-in</Link></div>
          </div> : <>
            <div className="admin-heading">
              <div><p className="section-kicker">COACH DIRECTORY</p><h1>Coaches</h1><p className="admin-help">Add and update coaches. Deactivate a coach to keep their history without listing them for new assignments.</p></div>
            </div>
            {error && <p className="admin-state form-error" role="alert">{error}</p>}
            {notice && <p className="admin-state form-success" role="status">{notice}</p>}
            <section className="availability-panel">
              <h2>Add a coach</h2>
              <form className="schedule-form" onSubmit={createCoach}>
                <label>Full name<input name="full_name" maxLength={120} required /></label>
                <label>Email<input name="email" type="email" maxLength={254} /></label>
                <label>Phone<input name="phone" maxLength={40} /></label>
                <label className="schedule-form-notes">Notes (optional)<input name="notes" maxLength={2000} /></label>
                <button className="admin-button" type="submit" disabled={busy}>{busy ? "Saving…" : "Add coach"}</button>
              </form>
            </section>
            <section className="availability-panel">
              <div className="schedule-section-heading"><h2>Directory</h2><button className="admin-button secondary" type="button" disabled={loading || busy} onClick={() => { void loadCoaches(); }}>{loading ? "Loading…" : "Refresh"}</button></div>
              {coaches.length === 0 ? <p className="admin-state">{loading ? "Loading coaches…" : "No coaches yet. Add the first coach above."}</p>
                : <div className="coach-session-list">{coaches.map((coach) => (
                  <CoachCard
                    key={coach.id}
                    coach={coach}
                    editing={editingID === coach.id}
                    busy={busy}
                    onEdit={() => setEditingID(coach.id)}
                    onCancel={() => setEditingID("")}
                    onSave={(values) => { void saveCoach(coach, values); }}
                    onToggle={() => { void saveCoach(coach, { fullName: coach.full_name, email: coach.email ?? "", phone: coach.phone ?? "", notes: coach.notes ?? "", isActive: !coach.is_active }); }}
                  />
                ))}</div>}
            </section>
          </>}
      </section>
    </main>
  );
}

function CoachCard({
  coach,
  editing,
  busy,
  onEdit,
  onCancel,
  onSave,
  onToggle,
}: {
  coach: Coach;
  editing: boolean;
  busy: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onSave: (values: { fullName: string; email: string; phone: string; notes: string; isActive: boolean }) => void;
  onToggle: () => void;
}) {
  const [fullName, setFullName] = useState(coach.full_name);
  const [email, setEmail] = useState(coach.email ?? "");
  const [phone, setPhone] = useState(coach.phone ?? "");
  const [notes, setNotes] = useState(coach.notes ?? "");
  const [isActive, setIsActive] = useState(coach.is_active);

  useEffect(() => {
    setFullName(coach.full_name);
    setEmail(coach.email ?? "");
    setPhone(coach.phone ?? "");
    setNotes(coach.notes ?? "");
    setIsActive(coach.is_active);
  }, [coach]);

  return (
    <article className="availability-card coach-admin-card">
      {editing ? <form
        className="schedule-form coach-admin-form"
        onSubmit={(event) => {
          event.preventDefault();
          onSave({ fullName, email, phone, notes, isActive });
        }}
      >
        <label>Full name<input value={fullName} onChange={(event) => setFullName(event.target.value)} maxLength={120} required /></label>
        <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} maxLength={254} /></label>
        <label>Phone<input value={phone} onChange={(event) => setPhone(event.target.value)} maxLength={40} /></label>
        <label className="schedule-form-notes">Notes<input value={notes} onChange={(event) => setNotes(event.target.value)} maxLength={2000} /></label>
        <label className="schedule-publish"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /> Active</label>
        <div className="coach-admin-actions">
          <button className="admin-button" type="submit" disabled={busy}>Save</button>
          <button className="admin-button secondary" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
        </div>
      </form> : <div>
        <div className="coach-admin-heading">
          <strong>{coach.full_name}</strong>
          <span className={`session-status ${coach.is_active ? "session-status-open" : "session-status-hidden"}`}>{coach.is_active ? "Active" : "Inactive"}</span>
        </div>
        <p>{[coach.email, coach.phone].filter(Boolean).join(" · ") || "No contact details on file"}</p>
        {coach.notes && <small>{coach.notes}</small>}
        <p className="coach-admin-meta">{coach.assigned_session_count} assigned session{coach.assigned_session_count === 1 ? "" : "s"}</p>
        <div className="coach-admin-actions">
          <button className="admin-button secondary" type="button" disabled={busy} onClick={onEdit}>Edit</button>
          <button className="admin-button secondary" type="button" disabled={busy} onClick={onToggle}>{coach.is_active ? "Deactivate" : "Reactivate"}</button>
        </div>
      </div>}
    </article>
  );
}
