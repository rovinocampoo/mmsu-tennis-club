"use client";

import { FormEvent, useMemo, useReducer, useRef, useState } from "react";
import {
  bookingRequestReducer,
  initialBookingRequestState,
} from "./booking-request-form-state.mjs";
import {
  addParticipant,
  createParticipant,
  joinParticipantNames,
  participantError,
  participantNames,
  PARTICIPANT_MAX_COUNT,
  removeParticipant,
} from "./booking-participants.mjs";

export default function BookingRequestForm({
  sessionIds,
  sessionId,
}: {
  sessionIds?: string[];
  sessionId?: string;
}) {
  const requestedSessionIDs = sessionIds ?? (sessionId ? [sessionId] : []);
  const [open, setOpen] = useState(false);
  const [request, dispatch] = useReducer(
    bookingRequestReducer,
    initialBookingRequestState,
  );
  const [participants, setParticipants] = useState(() => [
    createParticipant("p0"),
  ]);
  const [attempted, setAttempted] = useState(false);
  const submitting = useRef(false);
  const [copyMessage, setCopyMessage] = useState("");

  const participantMessage = useMemo(
    () => participantError(participants),
    [participants],
  );

  function updateParticipantName(id: string, name: string) {
    setParticipants((current) =>
      current.map((participant) =>
        participant.id === id ? { ...participant, name } : participant,
      ),
    );
  }

  function handleAddParticipant() {
    setParticipants((current) => addParticipant(current));
  }

  function handleRemoveParticipant(id: string) {
    setParticipants((current) => removeParticipant(current, id));
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || request.phase !== "editing") return;

    setAttempted(true);
    const validationMessage = participantError(participants);
    if (validationMessage) return;

    const form = event.currentTarget;
    const data = new FormData(form);
    const names = participantNames(participants);
    submitting.current = true;
    dispatch({ type: "start" });

    try {
      const response = await fetch("/api/backend/booking-requests", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...(requestedSessionIDs.length === 1
            ? { session_id: requestedSessionIDs[0] }
            : { session_ids: requestedSessionIDs }),
          participants: names,
          full_name: joinParticipantNames(participants),
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
          <p className="request-note">
            Save your private status link below to check this request anytime.
            The club does not rely on email or text messages to confirm a
            request.
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
          <fieldset className="request-participants">
            <legend>Participants</legend>
            <p className="request-note">
              List everyone who will attend this training. Add one name per
              person, up to {PARTICIPANT_MAX_COUNT} people.
            </p>
            {participants.map((participant, index) => {
              const inputId = `booking-participant-${participant.id}`;
              return (
                <div className="request-participant-row" key={participant.id}>
                  <label htmlFor={inputId}>
                    Participant {index + 1}
                    <input
                      id={inputId}
                      value={participant.name}
                      onChange={(event) =>
                        updateParticipantName(
                          participant.id,
                          event.target.value,
                        )
                      }
                      maxLength={200}
                      autoComplete="off"
                      disabled={request.phase === "submitting"}
                      aria-invalid={attempted && participant.name.trim() === ""}
                    />
                  </label>
                  {participants.length > 1 && (
                    <button
                      type="button"
                      className="request-remove-participant"
                      onClick={() => handleRemoveParticipant(participant.id)}
                      disabled={request.phase === "submitting"}
                      aria-label={`Remove participant ${index + 1}`}
                    >
                      Remove
                    </button>
                  )}
                </div>
              );
            })}
            <button
              type="button"
              className="request-add-participant"
              onClick={handleAddParticipant}
              disabled={
                request.phase === "submitting" ||
                participants.length >= PARTICIPANT_MAX_COUNT
              }
            >
              + Add another participant
            </button>
            {attempted && participantMessage && (
              <p className="form-error" role="alert">
                {participantMessage}
              </p>
            )}
          </fieldset>

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
            Email and phone are optional. Adding one may help the club follow up
            about your pending request, but a name is all you need to submit.
            Requests are reviewed by a club admin and do not reserve a spot.
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
