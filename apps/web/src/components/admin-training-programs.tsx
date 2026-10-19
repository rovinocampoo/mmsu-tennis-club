"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";

type Program = {
  id: string;
  name: string;
  description: string | null;
  is_active: boolean;
};

type AdminFetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

async function readResponse(response: Response) {
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message = typeof data?.error === "string" ? data.error : "Training program request failed.";
    throw Object.assign(new Error(message), { status: response.status });
  }
  return data;
}

export default function AdminTrainingPrograms({
  adminFetch,
  onAuthorizationError,
}: {
  adminFetch: AdminFetch;
  onAuthorizationError: (cause: unknown) => void;
}) {
  const [programs, setPrograms] = useState<Program[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [editing, setEditing] = useState<Program | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");

  const loadPrograms = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const response = await adminFetch("/api/backend/admin/training-program-catalog");
      const data = await readResponse(response);
      setPrograms(Array.isArray(data.programs) ? data.programs : []);
    } catch (cause) {
      if (cause instanceof Error && [401, 403].includes((cause as Error & { status?: number }).status || 0)) {
        onAuthorizationError(cause);
      }
      setLoadError(cause instanceof Error ? cause.message : "Could not load training programs.");
    } finally {
      setLoading(false);
    }
  }, [adminFetch, onAuthorizationError]);

  useEffect(() => {
    void loadPrograms();
  }, [loadPrograms]);

  async function createProgram(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await readResponse(await adminFetch("/api/backend/admin/training-program-catalog", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, description }),
      }));
      setName("");
      setDescription("");
      setNotice("Training program created.");
      await loadPrograms();
    } catch (cause) {
      if (cause instanceof Error && [401, 403].includes((cause as Error & { status?: number }).status || 0)) onAuthorizationError(cause);
      setError(cause instanceof Error ? cause.message : "Could not create training program.");
    } finally {
      setBusy(false);
    }
  }

  function beginEdit(program: Program) {
    setEditing(program);
    setEditName(program.name);
    setEditDescription(program.description || "");
    setError("");
    setNotice("");
  }

  async function saveProgram(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await readResponse(await adminFetch(`/api/backend/admin/training-program-catalog/${editing.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: editName, description: editDescription, is_active: editing.is_active }),
      }));
      setEditing(null);
      setNotice("Training program updated.");
      await loadPrograms();
    } catch (cause) {
      if (cause instanceof Error && [401, 403].includes((cause as Error & { status?: number }).status || 0)) onAuthorizationError(cause);
      setError(cause instanceof Error ? cause.message : "Could not update training program.");
    } finally {
      setBusy(false);
    }
  }

  async function setProgramActive(program: Program, isActive: boolean) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await readResponse(await adminFetch(`/api/backend/admin/training-program-catalog/${program.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: program.name, description: program.description || "", is_active: isActive }),
      }));
      setNotice(isActive ? "Training program activated." : "Training program deactivated. Existing bookings and sessions remain in place.");
      await loadPrograms();
    } catch (cause) {
      if (cause instanceof Error && [401, 403].includes((cause as Error & { status?: number }).status || 0)) onAuthorizationError(cause);
      setError(cause instanceof Error ? cause.message : "Could not change training program status.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="program-management" aria-labelledby="program-management-title">
      <div className="availability-panel">
        <div className="schedule-section-heading">
          <div>
            <p className="section-kicker">PROGRAM CATALOG</p>
            <h2 id="program-management-title">Training programs</h2>
          </div>
          <span className="program-total">{programs.length} {programs.length === 1 ? "program" : "programs"}</span>
        </div>
        <p className="admin-help">Create and maintain the programs offered by the club. Deactivation keeps existing sessions and booking history intact.</p>

        {notice && <p className="admin-state form-success" role="status" aria-live="polite">{notice}</p>}
        {error && <p className="admin-state form-error" role="alert">{error}</p>}

        <form className="program-create-form" onSubmit={createProgram}>
          <h3>Add a program</h3>
          <label>
            Program name <span aria-hidden="true">*</span>
            <input required value={name} onChange={(event) => setName(event.target.value)} maxLength={200} disabled={busy} />
          </label>
          <label>
            Description <span className="program-optional">Optional</span>
            <textarea value={description} onChange={(event) => setDescription(event.target.value)} rows={3} maxLength={2000} disabled={busy} />
          </label>
          <button className="admin-button" type="submit" disabled={busy || !name.trim()}>{busy ? "Saving…" : "Create program"}</button>
        </form>

        {loading ? (
          <p className="admin-state" role="status">Loading training programs…</p>
        ) : loadError ? (
          <div className="admin-state form-error" role="alert">
            <p>{loadError}</p>
            <button className="admin-button secondary" type="button" disabled={busy} onClick={() => void loadPrograms()}>Retry</button>
          </div>
        ) : programs.length === 0 ? (
          <div className="admin-state"><strong>No training programs yet.</strong><p>Create the first program using the form above.</p></div>
        ) : (
          <div className="program-management-list">
            {programs.map((program) => (
              <article className="program-management-card" key={program.id}>
                {editing?.id === program.id ? (
                  <form onSubmit={saveProgram}>
                    <label>Program name<input required value={editName} onChange={(event) => setEditName(event.target.value)} maxLength={200} disabled={busy} /></label>
                    <label>Description <span className="program-optional">Optional</span><textarea value={editDescription} onChange={(event) => setEditDescription(event.target.value)} rows={3} maxLength={2000} disabled={busy} /></label>
                    <div className="admin-actions">
                      <button className="admin-button" type="submit" disabled={busy || !editName.trim()}>{busy ? "Saving…" : "Save changes"}</button>
                      <button className="admin-button secondary" type="button" disabled={busy} onClick={() => setEditing(null)}>Cancel</button>
                    </div>
                  </form>
                ) : (
                  <>
                    <div className="program-management-heading">
                      <div><h3>{program.name}</h3><span className={`program-status ${program.is_active ? "active" : "inactive"}`}>{program.is_active ? "Active" : "Inactive"}</span></div>
                    </div>
                    <p className="program-management-description">{program.description || "No description provided."}</p>
                    <div className="admin-actions">
                      <button className="admin-button secondary" type="button" disabled={busy} onClick={() => beginEdit(program)}>Edit</button>
                      <button className={`admin-button ${program.is_active ? "danger" : "secondary"}`} type="button" disabled={busy} onClick={() => void setProgramActive(program, !program.is_active)}>{program.is_active ? "Deactivate" : "Activate"}</button>
                    </div>
                  </>
                )}
              </article>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
