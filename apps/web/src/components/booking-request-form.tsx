"use client";

import { FormEvent, useReducer, useRef, useState } from "react";
import {
  bookingRequestReducer,
  initialBookingRequestState,
} from "./booking-request-form-state.mjs";

export default function BookingRequestForm({
  sessionId,
}: {
  sessionId: string;
}) {
  const [open, setOpen] = useState(false);
  const [request, dispatch] = useReducer(
    bookingRequestReducer,
    initialBookingRequestState,
  );
  const submitting = useRef(false);
  const [copyMessage, setCopyMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || request.phase !== "editing") return;

    const form = event.currentTarget;
    const data = new FormData(form);
    submitting.current = true;
    dispatch({ type: "start" });

    try {
      const response = await fetch("/api/backend/booking-requests", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          session_id: sessionId,
          full_name: data.get("full_name"),
          email: data.get("email"),
          phone: data.get("phone"),
        }),
      });
      const result = await response.json();
      if (!response.ok)
        throw new Error(
          result.error || "We could not send your request. Please try again.",
        );
      if (
        result.status !== "pending" ||
        typeof result.request_reference !== "string" ||
        result.request_reference.length === 0 ||
        typeof result.tracking_token !== "string" ||
        !/^[A-Za-z0-9_-]{43}$/.test(result.tracking_token)
      ) {
        throw new Error(
          "We could not confirm the request status. Please check with the club before trying again.",
        );
      }
      dispatch({
        type: "success",
        requestReference: result.request_reference,
        trackingToken: result.tracking_token,
      });
    } catch (cause) {
      dispatch({
        type: "failure",
        error:
          cause instanceof Error
            ? cause.message
            : "We could not send your request. Please try again.",
      });
    } finally {
      submitting.current = false;
    }
  }

  if (!open)
    return (
      <button
        className="request-open"
        type="button"
        onClick={() => setOpen(true)}
      >
        Request a spot
      </button>
    );
  const statusLink =
    request.phase === "accepted" && request.trackingToken
      ? (() => {
          const url = new URL("/booking-status", window.location.origin);
          url.hash = request.trackingToken;
          return url.toString();
        })()
      : null;
  return (
    <div className="request-box">
      <button
        className="request-close"
        type="button"
        onClick={() => setOpen(false)}
        disabled={request.phase === "submitting"}
        aria-label="Close booking request form"
      >
        ×
      </button>
      {request.phase === "accepted" ? (
        <div role="status" className="request-confirmation">
          <p className="form-success">
            <strong>Request received: Pending admin approval.</strong>
          </p>
          <p className="request-note">
            This is not a confirmed booking and does not reserve a spot.
          </p>
          <p className="request-note">
            Request reference: <strong>{request.requestReference}</strong>
          </p>
          {statusLink && (
            <div className="status-link-actions">
              <a className="request-submit" href={statusLink}>
                Check request status
              </a>
              <button
                className="request-copy"
                type="button"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(statusLink);
                    setCopyMessage("Private status link copied.");
                  } catch {
                    setCopyMessage("Could not copy the link. Open it and copy the address from your browser.");
                  }
                }}
              >
                Copy private status link
              </button>
              {copyMessage && (
                <p className="request-note" role="status" aria-live="polite">
                  {copyMessage}
                </p>
              )}
            </div>
          )}
          <button
            className="request-submit"
            type="button"
            onClick={() => setOpen(false)}
          >
            Close
          </button>
        </div>
      ) : (
        <form onSubmit={submit}>
          <label>
            Full name
            <input
              name="full_name"
              autoComplete="name"
              maxLength={200}
              required
              disabled={request.phase === "submitting"}
            />
          </label>
          <label>
            Email (optional)
            <input
              name="email"
              type="email"
              autoComplete="email"
              maxLength={254}
              disabled={request.phase === "submitting"}
            />
          </label>
          <label>
            Phone (optional)
            <input
              name="phone"
              type="tel"
              autoComplete="tel"
              maxLength={40}
              disabled={request.phase === "submitting"}
            />
          </label>
          <p className="request-note">
            Enter an email or phone number for any updates on your booking
            request. Requests are reviewed by a club admin and do not reserve a
            spot.
          </p>
          {request.phase === "editing" && request.error && (
            <p className="form-error" role="alert">
              {request.error}
            </p>
          )}
          <button
            className="request-submit"
            disabled={request.phase === "submitting"}
            type="submit"
          >
            {request.phase === "submitting" ? "Sending…" : "Send request"}
          </button>
        </form>
      )}
    </div>
  );
}
