"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

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

  useEffect(() => {
    const token = window.location.hash.slice(1);
    if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
      setPhase("invalid");
      return;
    }

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
          </article>
        )}
      </section>

      <footer className="training-footer"><span>MM<span className="footer-ball">●</span>U TENNIS CLUB</span><span>Good things happen on court.</span></footer>
    </main>
  );
}
