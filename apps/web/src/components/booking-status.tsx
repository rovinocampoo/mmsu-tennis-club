"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { createBookingMessage, createCalendarFile, createGoogleCalendarURL, getCalendarFilename } from "@/lib/booking-confirmation.mjs";

type BookingStatus = {
  request_reference: string;
  status: "pending" | "approved" | "rejected";
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  next_steps: string[];
};

type Phase = "loading" | "loaded" | "invalid" | "expired" | "error";

const manilaDate = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "long",
  month: "long",
  day: "numeric",
  year: "numeric",
});
const manilaTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  hour: "numeric",
  minute: "2-digit",
  hour12: true,
});

function isBookingStatus(value: unknown): value is BookingStatus {
  if (!value || typeof value !== "object") return false;
  const status = value as Record<string, unknown>;
  return (
    typeof status.request_reference === "string" &&
    ["pending", "approved", "rejected"].includes(String(status.status)) &&
    typeof status.program_name === "string" &&
    typeof status.starts_at === "string" &&
    typeof status.ends_at === "string" &&
    typeof status.location === "string" &&
    Array.isArray(status.next_steps) &&
    status.next_steps.every((step) => typeof step === "string")
  );
}

export default function BookingStatus() {
  const [phase, setPhase] = useState<Phase>("loading");
  const [status, setStatus] = useState<BookingStatus | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [statusPageURL, setStatusPageURL] = useState("");
  const [copyNotice, setCopyNotice] = useState("");

  useEffect(() => {
    const token = window.location.hash.slice(1);
    if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
      setPhase("invalid");
      return;
    }

    setStatusPageURL(window.location.href);
    const controller = new AbortController();
    let active = true;
    setPhase("loading");

    async function loadStatus() {
      try {
        const response = await fetch("/api/backend/booking-status", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ tracking_token: token }),
          cache: "no-store",
          signal: controller.signal,
        });
        if (!active) return;
        if (response.status === 404) {
          setPhase("invalid");
          return;
        }
        if (response.status === 410) {
          setPhase("expired");
          return;
        }
        if (!response.ok) throw new Error("Booking status could not be loaded.");
        const result: unknown = await response.json();
        if (!isBookingStatus(result)) throw new Error("Booking status response was invalid.");
        if (!active) return;
        setStatus(result);
        setPhase("loaded");
      } catch {
        if (active && !controller.signal.aborted) setPhase("error");
      }
    }

    void loadStatus();
    return () => {
      active = false;
      controller.abort();
    };
  }, [attempt]);

  const calendarURL = status && status.status === "approved" ? createGoogleCalendarURL(status) : "";

  async function copyBookingMessage() {
    if (!status || !statusPageURL) return;
    const message = createBookingMessage(status, statusPageURL);
    setCopyNotice("");
    try {
      await navigator.clipboard.writeText(message);
      setCopyNotice("Booking message copied. Paste it into Messenger or SMS.");
      return;
    } catch {
      // Fall back for browsers without Clipboard API access.
    }
    const textarea = document.createElement("textarea");
    textarea.value = message;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    let copied = false;
    try { copied = document.execCommand("copy"); } catch { copied = false; }
    textarea.remove();
    setCopyNotice(copied
      ? "Booking message copied. Paste it into Messenger or SMS."
      : "Copy was blocked by your browser. Please select and copy the message from the page.");
  }

  function downloadCalendarFile() {
    if (!status || status.status !== "approved") return;
    const blob = new Blob([createCalendarFile(status)], { type: "text/calendar;charset=utf-8" });
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = getCalendarFilename(status.request_reference);
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
  }

  return (
    <main className="training-page">
      <header className="training-header">
        <Link className="brand" href="/" aria-label="MMSU Tennis Club home">
          <span className="brand-mark" aria-hidden="true">M</span>
          <span><strong>MMSU</strong><small>TENNIS CLUB</small></span>
        </Link>
        <span className="header-note">Private booking status</span>
      </header>

      <section className="booking-status-page" aria-labelledby="booking-status-title">
        <p className="section-kicker">BOOKING REQUEST</p>
        <h1 id="booking-status-title">Your request status</h1>
        {phase === "loading" && (
          <div className="loading-panel" role="status" aria-live="polite">
            <span className="loading-dot" aria-hidden="true" />
            <p>Loading your request status…</p>
          </div>
        )}
        {phase === "invalid" && (
          <div className="state-panel error-panel" role="alert">
            <span className="state-icon" aria-hidden="true">!</span>
            <div>
              <h2>Invalid status link</h2>
              <p>This private link is missing or invalid. Open the complete link shared when you submitted your request.</p>
            </div>
          </div>
        )}
        {phase === "expired" && (
          <div className="state-panel error-panel" role="alert">
            <span className="state-icon" aria-hidden="true">!</span>
            <div>
              <h2>Status link expired</h2>
              <p>This private link has expired. Contact the MMSU Tennis Club if you still need help with your request.</p>
            </div>
          </div>
        )}
        {phase === "error" && (
          <div className="state-panel error-panel" role="alert">
            <span className="state-icon" aria-hidden="true">!</span>
            <div>
              <h2>Status temporarily unavailable</h2>
              <p>We couldn’t load your request. Please try again.</p>
              <button className="request-submit" type="button" onClick={() => setAttempt((value) => value + 1)}>
                Try again
              </button>
            </div>
          </div>
        )}
        {phase === "loaded" && status && (
          <article className="booking-status-card" aria-live="polite">
            <div className="booking-status-heading">
              <div>
                <p className="request-note">Request reference</p>
                <strong>{status.request_reference}</strong>
              </div>
              <span className={`booking-status-badge booking-status-${status.status}`}>
                {status.status}
              </span>
            </div>
            <h2>{status.program_name}</h2>
            <dl className="booking-status-details">
              <div><dt>Date</dt><dd>{manilaDate.format(new Date(status.starts_at))}</dd></div>
              <div><dt>Time</dt><dd>{manilaTime.format(new Date(status.starts_at))} – {manilaTime.format(new Date(status.ends_at))} (PHT)</dd></div>
              <div><dt>Location</dt><dd>{status.location}</dd></div>
            </dl>
            <div className="booking-next-steps">
              <h3>Next steps</h3>
              <ul>{status.next_steps.map((step) => <li key={step}>{step}</li>)}</ul>
            </div>
            <div className="booking-actions" aria-label="Booking actions">
              {status.status === "approved" && <div className="booking-calendar-actions">
                <p className="booking-actions-title">Save your confirmed session</p>
                <a className="request-submit booking-calendar-link" href={calendarURL} target="_blank" rel="noopener noreferrer">
                  <span aria-hidden="true">▦</span> Add to Google Calendar
                </a>
                <button className="request-copy" type="button" onClick={downloadCalendarFile}>Download calendar file (.ics)</button>
                <p className="request-note">Times are shown in Philippine Time (PHT).</p>
              </div>}
              <div className="booking-message-actions">
                <button className="request-copy" type="button" onClick={() => void copyBookingMessage()}>
                  <span aria-hidden="true">▣</span> {status.status === "approved" ? "Copy confirmation message" : "Copy booking update"}
                </button>
                <button className="request-copy" type="button" onClick={() => setAttempt((value) => value + 1)}>
                  Refresh latest status
                </button>
              </div>
              {copyNotice && <p className="booking-copy-notice" role="status" aria-live="polite">{copyNotice}</p>}
              <p className="request-note">Share the copied message in Messenger or SMS.</p>
            </div>
          </article>
        )}
      </section>

      <footer className="training-footer"><span>MM<span className="footer-ball">●</span>U TENNIS CLUB</span><span>Good things happen on court.</span></footer>
    </main>
  );
}
