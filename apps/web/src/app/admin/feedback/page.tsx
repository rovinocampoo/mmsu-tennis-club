"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";
import {
  buildFeedbackCsv,
  feedbackExportFilename,
  filterSubmissions,
  summarizeSubmissions,
  FEEDBACK_CATEGORY_LABELS,
  FEEDBACK_STATUSES,
} from "@/lib/feedback-inbox.mjs";

type FeedbackSubmission = {
  id: string;
  category: string;
  message: string;
  name: string | null;
  email: string | null;
  status: "new" | "reviewed" | "archived";
  created_at: string;
  reviewed_at: string | null;
  reviewed_by: string | null;
};

type AuthStatus = "checking" | "authenticated" | "signed_out" | "unavailable";
type LoadStatus = "idle" | "loading" | "ready" | "error";

const supabase = getSupabaseBrowserClient();

const expiredSessionMessage =
  "Your admin session has expired. Please sign in again.";

const MESSAGE_PREVIEW_LENGTH = 220;

const statusLabels: Record<string, string> = {
  new: "New",
  reviewed: "Reviewed",
  archived: "Archived",
};

const statusActionLabels: Record<string, string> = {
  new: "Mark new",
  reviewed: "Mark reviewed",
  archived: "Archive",
};

const manilaDateKey = (value: string) =>
  new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Manila",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(value));

function formatSubmittedAt(value: string) {
  return new Intl.DateTimeFormat("en-PH", {
    timeZone: "Asia/Manila",
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function countLabel(count: number, singular: string, plural?: string) {
  return `${count} ${count === 1 ? singular : (plural ?? `${singular}s`)}`;
}

function FeedbackCard({
  record,
  busy,
  onStatusChange,
}: {
  record: FeedbackSubmission;
  busy: boolean;
  onStatusChange: (id: string, status: FeedbackSubmission["status"]) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const isLong = record.message.length > MESSAGE_PREVIEW_LENGTH;
  const displayMessage =
    isLong && !expanded
      ? `${record.message.slice(0, MESSAGE_PREVIEW_LENGTH).trimEnd()}…`
      : record.message;
  const contact = record.email?.trim();

  return (
    <article className="admin-card feedback-card">
      <div className="feedback-card-heading">
        <span
          className={`feedback-category feedback-category-${record.category}`}
        >
          {FEEDBACK_CATEGORY_LABELS[record.category] ?? record.category}
        </span>
        <span className={`feedback-status feedback-status-${record.status}`}>
          {statusLabels[record.status] ?? record.status}
        </span>
      </div>

      <p className="feedback-message">{displayMessage}</p>

      {isLong && (
        <button
          type="button"
          className="feedback-expand"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "Show less" : "Show full message"}
        </button>
      )}

      <div className="feedback-meta">
        <span>
          <strong>{record.name?.trim() || "Anonymous"}</strong>
        </span>
        {contact && <span className="feedback-contact">{contact}</span>}
        <span>Received {formatSubmittedAt(record.created_at)}</span>
        {record.reviewed_at && (
          <span>Reviewed {formatSubmittedAt(record.reviewed_at)}</span>
        )}
      </div>

      <div
        className="admin-actions feedback-actions"
        role="group"
        aria-label="Set review status"
      >
        {FEEDBACK_STATUSES.map((status) => (
          <button
            key={status}
            type="button"
            className={`admin-button secondary${record.status === status ? " active" : ""}`}
            aria-pressed={record.status === status}
            disabled={busy || record.status === status}
            onClick={() =>
              onStatusChange(record.id, status as FeedbackSubmission["status"])
            }
          >
            {statusActionLabels[status]}
          </button>
        ))}
      </div>
    </article>
  );
}

export default function AdminFeedbackPage() {
  const [authStatus, setAuthStatus] = useState<AuthStatus>("checking");
  const [sessionRetry, setSessionRetry] = useState(0);
  const [submissions, setSubmissions] = useState<FeedbackSubmission[]>([]);
  const [loadStatus, setLoadStatus] = useState<LoadStatus>("idle");
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");

  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [categoryFilter, setCategoryFilter] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [includeEmail, setIncludeEmail] = useState(false);

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
        setSubmissions([]);
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

  const loadSubmissions = useCallback(async () => {
    if (!supabase) return;

    setLoadStatus("loading");
    setError("");

    try {
      const response = await fetchWithAdminSession(
        supabase.auth,
        fetch,
        "/api/backend/admin/feedback-submissions",
      );

      const data = await response.json().catch(() => ({}));

      if (response.status === 401) {
        void supabase.auth.signOut().catch(() => undefined);
        setAuthStatus("signed_out");
        setSubmissions([]);
        throw new Error(expiredSessionMessage);
      }

      if (response.status === 403) {
        throw new Error(
          "This account is not authorized to view feedback submissions.",
        );
      }

      if (!response.ok) {
        throw new Error(
          typeof data?.error === "string"
            ? data.error
            : "Could not load feedback submissions.",
        );
      }

      setSubmissions(Array.isArray(data.submissions) ? data.submissions : []);
      setLoadStatus("ready");
    } catch (cause) {
      const message =
        cause instanceof Error
          ? cause.message
          : "Could not load feedback submissions.";

      if (message === expiredSessionMessage) {
        setAuthStatus("signed_out");
        setSubmissions([]);
      }

      setError(message);
      setLoadStatus("error");
    }
  }, []);

  useEffect(() => {
    if (authStatus === "authenticated") {
      void loadSubmissions();
    }
  }, [authStatus, loadSubmissions]);

  async function updateStatus(
    id: string,
    status: FeedbackSubmission["status"],
  ) {
    if (!supabase || busyId) return;

    setBusyId(id);
    setError("");

    try {
      const response = await fetchWithAdminSession(
        supabase.auth,
        fetch,
        `/api/backend/admin/feedback-submissions/${id}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ status }),
        },
      );

      const data = await response.json().catch(() => ({}));

      if (response.status === 401) {
        void supabase.auth.signOut().catch(() => undefined);
        setAuthStatus("signed_out");
        setSubmissions([]);
        throw new Error(expiredSessionMessage);
      }

      if (!response.ok) {
        throw new Error(
          typeof data?.error === "string"
            ? data.error
            : "Could not update the submission status.",
        );
      }

      setSubmissions((current) =>
        current.map((record) =>
          record.id === id
            ? {
                ...record,
                status,
                reviewed_at:
                  status === "new" ? null : new Date().toISOString(),
              }
            : record,
        ),
      );
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not update the submission status.",
      );
    } finally {
      setBusyId("");
    }
  }

  const summary = useMemo(
    () => summarizeSubmissions(submissions),
    [submissions],
  );

  const filters = useMemo(
    () => ({ query, status: statusFilter, category: categoryFilter, dateFrom, dateTo }),
    [query, statusFilter, categoryFilter, dateFrom, dateTo],
  );

  const filteredSubmissions = useMemo(
    () =>
      filterSubmissions(submissions, filters, (submission) =>
        manilaDateKey(submission.created_at),
      ),
    [submissions, filters],
  );

  const hasActiveFilters = Boolean(
    query || statusFilter || categoryFilter || dateFrom || dateTo,
  );

  const activeFilterSummary = useMemo(() => {
    const parts: string[] = [];
    if (query.trim()) parts.push(`search “${query.trim()}”`);
    if (statusFilter) parts.push(`status: ${statusLabels[statusFilter]}`);
    if (categoryFilter)
      parts.push(
        `category: ${FEEDBACK_CATEGORY_LABELS[categoryFilter] ?? categoryFilter}`,
      );
    if (dateFrom) parts.push(`from ${dateFrom}`);
    if (dateTo) parts.push(`to ${dateTo}`);
    return parts.length > 0 ? parts.join(", ") : "All submissions";
  }, [query, statusFilter, categoryFilter, dateFrom, dateTo]);

  function resetFilters() {
    setQuery("");
    setStatusFilter("");
    setCategoryFilter("");
    setDateFrom("");
    setDateTo("");
  }

  function toggleStatusFilter(status: string) {
    setStatusFilter((current) => (current === status ? "" : status));
  }

  function downloadCsv(records: FeedbackSubmission[]) {
    const csv = buildFeedbackCsv(records, { includeEmail });
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = feedbackExportFilename(new Date());
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  }

  function signOut() {
    setSubmissions([]);
    setAuthStatus("signed_out");

    if (supabase) {
      void supabase.auth.signOut().catch(() => undefined);
    }
  }

  return (
    <main className="admin-page">
      <header className="training-header no-print">
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
          <Link href="/admin/approved-clients">Approved clients</Link>
          <Link href="/admin/coaches">Coaches</Link>
          <Link href="/training">Public schedule</Link>
        </nav>
      </header>

      <section className="admin-main">
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
              Sign in to the admin workspace before reviewing suggestions and
              feedback.
            </p>
            <Link className="admin-button admin-button-link" href="/admin">
              Go to admin sign in
            </Link>
          </section>
        ) : (
          <div className="feedback-screen">
            <div className="admin-heading">
              <div>
                <p className="section-kicker">CLUB INBOX</p>
                <h1>Suggestions &amp; feedback</h1>
                <p className="admin-help">
                  Review submissions from members and visitors. Only authorized
                  admins can read these messages. Times are shown in Philippine
                  time.
                </p>
              </div>

              <div className="admin-heading-actions">
                <button
                  className="admin-button secondary"
                  type="button"
                  disabled={loadStatus === "loading"}
                  onClick={() => void loadSubmissions()}
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

            {error && (
              <section className="admin-state form-error" role="alert">
                <p>{error}</p>
              </section>
            )}

            {loadStatus === "loading" && submissions.length === 0 && (
              <div className="admin-state" role="status" aria-live="polite">
                Loading submissions…
              </div>
            )}

            {loadStatus === "ready" && (
              <>
                <section
                  className="feedback-summary"
                  aria-label="Submission totals"
                >
                  <button
                    type="button"
                    className={`feedback-stat${statusFilter === "new" ? " active" : ""}`}
                    aria-pressed={statusFilter === "new"}
                    onClick={() => toggleStatusFilter("new")}
                  >
                    <span className="feedback-stat-value">{summary.new}</span>
                    <span className="feedback-stat-label">New</span>
                  </button>
                  <button
                    type="button"
                    className={`feedback-stat${statusFilter === "reviewed" ? " active" : ""}`}
                    aria-pressed={statusFilter === "reviewed"}
                    onClick={() => toggleStatusFilter("reviewed")}
                  >
                    <span className="feedback-stat-value">
                      {summary.reviewed}
                    </span>
                    <span className="feedback-stat-label">Reviewed</span>
                  </button>
                  <button
                    type="button"
                    className={`feedback-stat${statusFilter === "archived" ? " active" : ""}`}
                    aria-pressed={statusFilter === "archived"}
                    onClick={() => toggleStatusFilter("archived")}
                  >
                    <span className="feedback-stat-value">
                      {summary.archived}
                    </span>
                    <span className="feedback-stat-label">Archived</span>
                  </button>
                  <button
                    type="button"
                    className={`feedback-stat${statusFilter === "" ? " active" : ""}`}
                    aria-pressed={statusFilter === ""}
                    onClick={() => setStatusFilter("")}
                  >
                    <span className="feedback-stat-value">{summary.total}</span>
                    <span className="feedback-stat-label">Total</span>
                  </button>
                </section>

                <section
                  className="admin-card feedback-toolbar"
                  aria-labelledby="feedback-filters-heading"
                >
                  <div className="schedule-section-heading">
                    <div>
                      <p className="section-kicker">FILTER</p>
                      <h2 id="feedback-filters-heading">Find a submission</h2>
                    </div>
                    <button
                      className="admin-button secondary"
                      type="button"
                      disabled={!hasActiveFilters}
                      onClick={resetFilters}
                    >
                      Clear filters
                    </button>
                  </div>

                  <div className="feedback-filters">
                    <label className="feedback-filter-search">
                      Search
                      <input
                        type="search"
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder="Message or submitter name"
                      />
                    </label>
                    <label>
                      Status
                      <select
                        value={statusFilter}
                        onChange={(event) =>
                          setStatusFilter(event.target.value)
                        }
                      >
                        <option value="">All statuses</option>
                        {FEEDBACK_STATUSES.map((status) => (
                          <option key={status} value={status}>
                            {statusLabels[status] ?? status}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Category
                      <select
                        value={categoryFilter}
                        onChange={(event) =>
                          setCategoryFilter(event.target.value)
                        }
                      >
                        <option value="">All categories</option>
                        {Object.entries(FEEDBACK_CATEGORY_LABELS).map(
                          ([value, label]) => (
                            <option key={value} value={value}>
                              {label}
                            </option>
                          ),
                        )}
                      </select>
                    </label>
                    <label>
                      Submitted from
                      <input
                        type="date"
                        value={dateFrom}
                        max={dateTo || undefined}
                        onChange={(event) => setDateFrom(event.target.value)}
                      />
                    </label>
                    <label>
                      Submitted to
                      <input
                        type="date"
                        value={dateTo}
                        min={dateFrom || undefined}
                        onChange={(event) => setDateTo(event.target.value)}
                      />
                    </label>
                  </div>

                  <div className="feedback-toolbar-actions">
                    <p className="admin-help" role="status">
                      Showing {countLabel(filteredSubmissions.length, "submission")}
                      {hasActiveFilters ? ` of ${summary.total}` : ""}.
                    </p>
                    <div className="feedback-export">
                      <label className="feedback-include-email">
                        <input
                          type="checkbox"
                          checked={includeEmail}
                          onChange={(event) =>
                            setIncludeEmail(event.target.checked)
                          }
                        />
                        Include submitter email
                      </label>
                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={filteredSubmissions.length === 0}
                        onClick={() => downloadCsv(filteredSubmissions)}
                      >
                        Export filtered CSV ({filteredSubmissions.length})
                      </button>
                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={submissions.length === 0}
                        onClick={() => downloadCsv(submissions)}
                      >
                        Export all ({submissions.length})
                      </button>
                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={filteredSubmissions.length === 0}
                        onClick={() => window.print()}
                      >
                        Print report
                      </button>
                    </div>
                  </div>
                </section>

                {filteredSubmissions.length === 0 ? (
                  <div className="admin-state feedback-no-results">
                    {submissions.length === 0
                      ? "No suggestions or feedback have been submitted yet."
                      : "No submissions match the current filters. Clear or adjust the filters to see more."}
                  </div>
                ) : (
                  <div className="admin-list feedback-list">
                    {filteredSubmissions.map((record) => (
                      <FeedbackCard
                        key={record.id}
                        record={record}
                        busy={busyId === record.id}
                        onStatusChange={updateStatus}
                      />
                    ))}
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {authStatus === "authenticated" && loadStatus === "ready" && (
          <section className="feedback-print-report" aria-hidden="true">
            <h1>MMSU Tennis Club — Suggestions &amp; Feedback</h1>
            <p>
              Generated{" "}
              {new Intl.DateTimeFormat("en-PH", {
                timeZone: "Asia/Manila",
                dateStyle: "medium",
                timeStyle: "short",
              }).format(new Date())}{" "}
              (Asia/Manila).
            </p>
            <p>Filters: {activeFilterSummary}</p>
            <p>
              {countLabel(filteredSubmissions.length, "submission")} included.
              Contact details are excluded from this report.
            </p>
            {filteredSubmissions.map((record) => (
              <article key={record.id} className="feedback-print-item">
                <h2>
                  {FEEDBACK_CATEGORY_LABELS[record.category] ?? record.category}
                  {" · "}
                  {statusLabels[record.status] ?? record.status}
                </h2>
                <p className="feedback-print-meta">
                  {record.name?.trim() || "Anonymous"} ·{" "}
                  {formatSubmittedAt(record.created_at)}
                </p>
                <p className="feedback-print-message">{record.message}</p>
              </article>
            ))}
          </section>
        )}
      </section>
    </main>
  );
}
