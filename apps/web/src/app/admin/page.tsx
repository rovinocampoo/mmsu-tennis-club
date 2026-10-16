"use client";

import Link from "next/link";
import {
  FormEvent,
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
} from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import { fetchWithAdminSession } from "@/lib/admin-auth-session.mjs";
import {
  adminDataReducer,
  adminSessionReducer,
  initialAdminDataState,
  initialAdminSessionState,
} from "@/lib/admin-session-state.mjs";
import AdminSchedule from "@/components/admin-schedule";

type RequestRow = {
  id: string;
  session_id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
  created_at: string;
  starts_at: string;
  location: string;
  session_status: string;
};
type Player = {
  id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
};
type ApprovalNotification = {
  requestId: string;
  notification_status: string;
  notification_message: string;
  notification_retryable: boolean;
};

const supabase = getSupabaseBrowserClient();
const expiredSessionMessage =
  "Your admin session has expired. Please sign in again.";
const authUnavailableMessage =
  "Admin authentication is temporarily unavailable. Try again shortly.";
const accessDeniedMessage =
  "This Supabase account is not authorized to review club bookings.";

async function readAdminResponse(response: Response, fallback: string) {
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) throw new Error(expiredSessionMessage);
    if (response.status === 403) throw new Error(accessDeniedMessage);
    const serverMessage = typeof data?.error === "string" ? data.error : "";
    if (
      response.status === 503 &&
      serverMessage.toLowerCase().includes("authentication")
    ) {
      throw new Error(authUnavailableMessage);
    }
    throw new Error(serverMessage || fallback);
  }
  return data;
}

export default function AdminPage() {
  const [sessionState, dispatchSession] = useReducer(
    adminSessionReducer,
    initialAdminSessionState,
  );
  const [adminDataState, dispatchAdminData] = useReducer(
    adminDataReducer,
    initialAdminDataState,
  );
  const [sessionRetry, setSessionRetry] = useState(0);
  const [verificationRetryCount, setVerificationRetryCount] = useState(0);
  const [authUnavailable, setAuthUnavailable] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [requests, setRequests] = useState<RequestRow[]>([]);
  const [activeView, setActiveView] = useState<"bookings" | "schedule">(
    "bookings",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [approvalNotification, setApprovalNotification] =
    useState<ApprovalNotification | null>(null);
  const [notificationBusy, setNotificationBusy] = useState(false);
  const requestLoadInFlight = useRef(false);

  const adminFetch = useCallback(
    (input: RequestInfo | URL, init: RequestInit = {}) => {
      if (!supabase)
        throw new Error(
          "Admin sign-in is not configured. Contact the site administrator.",
        );
      let showingRetry = false;
      return fetchWithAdminSession(supabase.auth, fetch, input, init, {
        onVerificationRetry: (retrying) => {
          if (retrying && !showingRetry) {
            showingRetry = true;
            setVerificationRetryCount((count) => count + 1);
          } else if (!retrying && showingRetry) {
            showingRetry = false;
            setVerificationRetryCount((count) => Math.max(0, count - 1));
          }
        },
      }).then((response) => {
        if (response.status !== 503) setAuthUnavailable(false);
        return response;
      });
    },
    [],
  );

  const handleAdminError = useCallback((cause: unknown) => {
    const message =
      cause instanceof Error
        ? cause.message
        : "Could not complete the admin request.";
    setError(message);
    if (message === expiredSessionMessage) {
      setAuthUnavailable(false);
      dispatchSession({ type: "auth_event", hasSession: false });
      setRequests([]);
      dispatchAdminData({ type: "reset" });
      if (supabase) void supabase.auth.signOut().catch(() => undefined);
    } else if (message === accessDeniedMessage) {
      setAuthUnavailable(false);
      dispatchAdminData({ type: "denied" });
    } else if (message === authUnavailableMessage) {
      setAuthUnavailable(true);
    }
  }, []);

  const loadRequests = useCallback(async () => {
    if (requestLoadInFlight.current) return;
    requestLoadInFlight.current = true;
    dispatchAdminData({ type: "loading" });
    setAuthUnavailable(false);
    setError("");
    try {
      const response = await adminFetch("/api/backend/admin/booking-requests");
      const data = await readAdminResponse(
        response,
        "Could not load booking requests.",
      );
      setRequests(data.requests || []);
      dispatchAdminData({ type: "ready" });
    } catch (cause) {
      const message =
        cause instanceof Error
          ? cause.message
          : "Could not load booking requests.";
      if (message === expiredSessionMessage || message === accessDeniedMessage)
        handleAdminError(cause);
      else {
        setError(message);
        if (message === authUnavailableMessage) setAuthUnavailable(true);
        dispatchAdminData({ type: "unavailable" });
      }
    } finally {
      requestLoadInFlight.current = false;
    }
  }, [adminFetch, handleAdminError]);

  useEffect(() => {
    if (!supabase) {
      dispatchSession({ type: "snapshot", hasSession: false });
      return;
    }
    let active = true;
    let authEventObserved = false;
    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((event, session) => {
      if (!active) return;
      authEventObserved = true;
      if (
        event === "INITIAL_SESSION" ||
        event === "SIGNED_IN" ||
        event === "TOKEN_REFRESHED" ||
        event === "USER_UPDATED"
      ) {
        dispatchSession({
          type: "auth_event",
          hasSession: Boolean(session?.access_token),
        });
        if (session?.access_token) {
          setError("");
          setAuthUnavailable(false);
          if (event === "SIGNED_IN") dispatchAdminData({ type: "reset" });
        }
      }
      if (event === "SIGNED_OUT" || !session?.access_token) {
        dispatchSession({ type: "auth_event", hasSession: false });
        setRequests([]);
        dispatchAdminData({ type: "reset" });
      }
    });
    void supabase.auth
      .getSession()
      .then(({ data, error: sessionError }) => {
        if (!active || authEventObserved) return;
        if (sessionError) {
          setError("Could not restore your admin session. Try again.");
          dispatchSession({ type: "unavailable" });
        } else
          dispatchSession({
            type: "snapshot",
            hasSession: Boolean(data.session?.access_token),
          });
      })
      .catch(() => {
        if (active && !authEventObserved) {
          setError("Could not restore your admin session. Try again.");
          dispatchSession({ type: "unavailable" });
        }
      });
    return () => {
      active = false;
      subscription.unsubscribe();
    };
  }, [sessionRetry]);

  useEffect(() => {
    if (sessionState.status !== "authenticated") return;
    loadRequests().catch(handleAdminError);
  }, [sessionState.status, loadRequests, handleAdminError]);

  async function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    if (!supabase) {
      setError(
        "Admin sign-in is not configured. Contact the site administrator.",
      );
      setBusy(false);
      return;
    }
    try {
      const { data, error: authError } = await supabase.auth.signInWithPassword(
        { email: email.trim(), password },
      );
      if (authError || !data.session?.access_token)
        throw new Error(
          "Email or password is incorrect, or sign-in is unavailable.",
        );
      dispatchSession({ type: "auth_event", hasSession: true });
      dispatchAdminData({ type: "reset" });
      setPassword("");
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Sign-in is unavailable.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function sendPasswordRecovery() {
    setBusy(true);
    setError("");
    setNotice("");
    const cleanEmail = email.trim();
    if (!cleanEmail || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(cleanEmail)) {
      setError("Enter a valid email address first.");
      setBusy(false);
      return;
    }
    if (!supabase) {
      setError(
        "Password recovery is not configured. Contact the site administrator.",
      );
      setBusy(false);
      return;
    }
    try {
      const { error: authError } = await supabase.auth.resetPasswordForEmail(
        cleanEmail,
        {
          redirectTo: new URL(
            "/admin/update-password",
            window.location.origin,
          ).toString(),
        },
      );
      if (authError)
        throw new Error(
          "The recovery email could not be sent. Check the email address and try again.",
        );
      setNotice(
        "If an account exists for that email, a password recovery link will arrive shortly.",
      );
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Password recovery is unavailable.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function decide(
    id: string,
    action: "approve" | "reject",
    playerId?: string,
    createNew = false,
  ) {
    if (sessionState.status !== "authenticated") return;
    setBusy(true);
    setError("");
    setNotice("");
    setAuthUnavailable(false);
    try {
      const response = await adminFetch(
        `/api/backend/admin/booking-requests/${id}/${action}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body:
            action === "approve"
              ? JSON.stringify({
                  player_id: playerId || "",
                  create_new: createNew,
                })
              : "{}",
        },
      );
      const data = await readAdminResponse(
        response,
        "The request could not be updated.",
      );
      if (action === "approve") {
        setApprovalNotification({ requestId: id, ...data });
        setNotice("Booking approved and confirmed.");
      } else {
        setApprovalNotification(null);
        setNotice("Request rejected. No approval email was sent.");
      }
      await loadRequests();
    } catch (cause) {
      handleAdminError(cause);
    } finally {
      setBusy(false);
    }
  }

  async function retryApprovalNotification() {
    if (sessionState.status !== "authenticated" || !approvalNotification)
      return;
    setNotificationBusy(true);
    setError("");
    setAuthUnavailable(false);
    try {
      const response = await adminFetch(
        `/api/backend/admin/booking-requests/${approvalNotification.requestId}/notification/retry`,
        {
          method: "POST",
        },
      );
      const data = await readAdminResponse(
        response,
        "The approval email could not be retried.",
      );
      setApprovalNotification({
        requestId: approvalNotification.requestId,
        ...data,
      });
    } catch (cause) {
      if (
        !(cause instanceof Error) ||
        cause.message !== expiredSessionMessage
      ) {
        setApprovalNotification((current) =>
          current
            ? {
                ...current,
                notification_status: "failed",
                notification_message:
                  "The retry could not be completed. Please contact the requester manually.",
                notification_retryable: true,
              }
            : current,
        );
      }
      handleAdminError(cause);
    } finally {
      setNotificationBusy(false);
    }
  }

  function retrySessionCheck() {
    setError("");
    dispatchSession({ type: "retry" });
    setSessionRetry((attempt) => attempt + 1);
  }

  function signOut() {
    dispatchSession({ type: "auth_event", hasSession: false });
    dispatchAdminData({ type: "reset" });
    setActiveView("bookings");
    setRequests([]);
    setError("");
    if (supabase) void supabase.auth.signOut().catch(() => undefined);
  }

  return (
    <main className="admin-page">
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
          <Link href="/training">Public schedule</Link>
          {sessionState.status === "authenticated" && (
            <Link href="/admin/approved-clients">Approved clients</Link>
          )}
        </nav>{" "}
      </header>
      <section className="admin-main">
        {sessionState.status === "checking" ? (
          <div className="admin-state" role="status" aria-live="polite">
            Checking your admin session…
          </div>
        ) : sessionState.status === "unavailable" ? (
          <div className="admin-login">
            <p className="section-kicker">CLUB ADMIN</p>
            <h1>Checking your admin session</h1>
            <p className="admin-state" role="status">
              {error || "Could not restore your admin session yet."}
            </p>
            <button
              className="admin-button"
              type="button"
              onClick={retrySessionCheck}
            >
              Retry session check
            </button>
          </div>
        ) : sessionState.status === "signed_out" ? (
          <div className="admin-login">
            <p className="section-kicker">CLUB ADMIN</p>
            <h1>Sign in to review requests.</h1>
            <p className="admin-help">
              Admin accounts are provisioned by the project owner. There is no
              public admin registration.
            </p>
            <form onSubmit={signIn}>
              <label>
                Email
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  autoComplete="username"
                  required
                />
              </label>
              <label>
                Password
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="current-password"
                  required
                />
              </label>
              {error && (
                <p className="form-error" role="alert">
                  {error}
                </p>
              )}
              {notice && (
                <p className="form-success" role="status">
                  {notice}
                </p>
              )}
              <button className="admin-button" disabled={busy} type="submit">
                {busy ? "Signing in…" : "Sign in"}
              </button>
            </form>
            <div className="admin-actions">
              <button
                className="admin-button secondary"
                disabled={busy}
                type="button"
                onClick={sendPasswordRecovery}
              >
                {busy ? "Sending…" : "Forgot password?"}
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="admin-heading">
              <div>
                <p className="section-kicker">
                  {activeView === "bookings"
                    ? "MANUAL REVIEW"
                    : "SCHEDULE PLANNING"}
                </p>
                <h1>
                  {activeView === "bookings"
                    ? "Booking requests"
                    : "Club schedule"}
                </h1>
                <p className="admin-help">
                  {activeView === "bookings"
                    ? "Approving confirms a spot. Pending requests do not reserve capacity. Match each request to the right player or explicitly create a new player."
                    : "Review coach availability, then separately create and publish actual training sessions."}
                </p>
              </div>
              <div className="admin-heading-actions">
                <button
                  className="admin-button secondary"
                  type="button"
                  onClick={signOut}
                >
                  Sign out
                </button>
              </div>
            </div>
            {verificationRetryCount > 0 && (
              <p className="admin-state" role="status" aria-live="polite">
                Verifying your admin session…
              </p>
            )}
            <div
              className="admin-tabs"
              role="tablist"
              aria-label="Admin workspace"
            >
              <button
                className={activeView === "bookings" ? "active" : ""}
                type="button"
                role="tab"
                aria-selected={activeView === "bookings"}
                onClick={() => setActiveView("bookings")}
              >
                Booking requests
              </button>
              <button
                className={activeView === "schedule" ? "active" : ""}
                type="button"
                role="tab"
                aria-selected={activeView === "schedule"}
                onClick={() => setActiveView("schedule")}
              >
                Schedule management
              </button>
            </div>
            {activeView === "schedule" ? (
              <AdminSchedule
                adminFetch={adminFetch}
                onAuthorizationError={handleAdminError}
              />
            ) : (
              <>
                {authUnavailable && (
                  <section className="admin-state form-error" role="alert">
                    <p>
                      {authUnavailableMessage} Your session is still active. You
                      can retry.
                    </p>
                    <button
                      className="admin-button secondary"
                      type="button"
                      disabled={busy || notificationBusy}
                      onClick={() => {
                        void loadRequests();
                      }}
                    >
                      Retry
                    </button>
                  </section>
                )}
                {error && !authUnavailable && (
                  <p className="admin-state form-error" role="alert">
                    {error}
                  </p>
                )}
                {notice && (
                  <p className="admin-state form-success" role="status">
                    {notice}
                  </p>
                )}
                {approvalNotification && (
                  <section
                    className={`admin-state ${approvalNotification.notification_status === "failed" ? "form-error" : "form-success"}`}
                    role="status"
                  >
                    <strong>
                      Approval email:{" "}
                      {approvalNotification.notification_status === "sent"
                        ? "accepted by Resend"
                        : approvalNotification.notification_status.replaceAll(
                            "_",
                            " ",
                          )}
                      .
                    </strong>
                    <p>{approvalNotification.notification_message}</p>
                    {approvalNotification.notification_retryable && (
                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={notificationBusy || busy}
                        onClick={retryApprovalNotification}
                      >
                        {notificationBusy
                          ? "Retrying…"
                          : "Retry approval email"}
                      </button>
                    )}
                  </section>
                )}
                {adminDataState.status === "denied" ? (
                  <p className="admin-state form-error" role="alert">
                    {accessDeniedMessage} Sign out and use an authorized club
                    admin account.
                  </p>
                ) : adminDataState.status === "loading" &&
                  requests.length === 0 ? (
                  <p className="admin-state" role="status">
                    Loading booking requests…
                  </p>
                ) : adminDataState.status === "unavailable" &&
                  requests.length === 0 ? (
                  <div className="admin-state">
                    <p>Booking requests are temporarily unavailable.</p>
                    <button
                      className="admin-button secondary"
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        void loadRequests();
                      }}
                    >
                      Retry
                    </button>
                  </div>
                ) : requests.length === 0 ? (
                  <p className="admin-state">No pending requests.</p>
                ) : (
                  <div className="admin-list">
                    {requests.map((row) => (
                      <RequestCard
                        key={row.id}
                        row={row}
                        busy={busy}
                        adminFetch={adminFetch}
                        onSessionError={handleAdminError}
                        onDecide={decide}
                      />
                    ))}
                  </div>
                )}
              </>
            )}
          </>
        )}
      </section>
    </main>
  );
}

function RequestCard({
  row,
  busy,
  adminFetch,
  onSessionError,
  onDecide,
}: {
  row: RequestRow;
  busy: boolean;
  adminFetch: (
    input: RequestInfo | URL,
    init?: RequestInit,
  ) => Promise<Response>;
  onSessionError: (cause: unknown) => void;
  onDecide: (
    id: string,
    action: "approve" | "reject",
    playerId?: string,
    createNew?: boolean,
  ) => void;
}) {
  const [query, setQuery] = useState(row.email || row.phone || row.full_name);
  const [players, setPlayers] = useState<Player[]>([]);
  const [playerId, setPlayerId] = useState("");
  const [createNew, setCreateNew] = useState(false);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState("");

  async function search() {
    setSearching(true);
    setError("");
    try {
      const response = await adminFetch(
        `/api/backend/admin/players?q=${encodeURIComponent(query)}`,
      );
      const data = await readAdminResponse(response, "Player search failed.");
      setPlayers(data.players || []);
      setPlayerId("");
      setCreateNew(false);
    } catch (cause) {
      if (
        cause instanceof Error &&
        [
          expiredSessionMessage,
          accessDeniedMessage,
          authUnavailableMessage,
        ].includes(cause.message)
      )
        onSessionError(cause);
      else
        setError(
          cause instanceof Error ? cause.message : "Player search failed.",
        );
    } finally {
      setSearching(false);
    }
  }

  return (
    <article className="admin-card">
      <h2>{row.full_name}</h2>
      <p>
        {[row.email, row.phone].filter(Boolean).join(" · ") ||
          "No contact details"}
        <br />
        Session:{" "}
        {new Date(row.starts_at).toLocaleString("en-PH", {
          timeZone: "Asia/Manila",
        })}{" "}
        · {row.location} · {row.session_status}
      </p>
      <label>
        Search existing players
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          maxLength={100}
        />
      </label>
      <div className="admin-actions">
        <button
          className="admin-button secondary"
          type="button"
          disabled={searching || query.trim().length < 2}
          onClick={search}
        >
          {searching ? "Searching…" : "Search players"}
        </button>
      </div>
      {players.length > 0 && (
        <div
          className="player-results"
          role="radiogroup"
          aria-label="Matching players"
        >
          {players.map((player) => (
            <label className="player-choice" key={player.id}>
              <input
                type="radio"
                name={`player-${row.id}`}
                checked={playerId === player.id}
                onChange={() => {
                  setPlayerId(player.id);
                  setCreateNew(false);
                }}
              />
              <span>
                <strong>{player.full_name}</strong>
                <br />
                {[player.email, player.phone].filter(Boolean).join(" · ")}
              </span>
            </label>
          ))}
        </div>
      )}
      {players.length === 0 && (
        <p className="admin-help">
          No player selected. Search and match an existing player, or explicitly
          create a new record from this request.
        </p>
      )}
      <label className="player-choice">
        <input
          type="checkbox"
          checked={createNew}
          onChange={(event) => {
            setCreateNew(event.target.checked);
            setPlayerId("");
          }}
        />
        Create a new player using the submitted contact details
      </label>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="admin-actions">
        <button
          className="admin-button"
          disabled={busy || (!playerId && !createNew)}
          onClick={() => onDecide(row.id, "approve", playerId, createNew)}
        >
          Approve and confirm
        </button>
        <button
          className="admin-button secondary"
          disabled={busy}
          onClick={() => onDecide(row.id, "reject")}
        >
          Reject
        </button>
      </div>
    </article>
  );
}
