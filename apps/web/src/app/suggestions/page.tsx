import type { Metadata } from "next";
import Link from "next/link";
import SuggestionsForm from "@/components/suggestions-form";

export const metadata: Metadata = {
  title: "Suggestions & Feedback | MMSU Tennis Club",
  description:
    "Send the MMSU Tennis Club a suggestion, feedback, or question about facilities, training, or scheduling. Contact details are optional.",
};

export default function SuggestionsPage() {
  return (
    <main className="training-page">
      <header className="training-header">
        <Link className="brand" href="/" aria-label="MMSU Tennis Club home">
          <span className="brand-mark" aria-hidden="true">
            M
          </span>
          <span>
            <strong>MMSU</strong>
            <small>TENNIS CLUB</small>
          </span>
        </Link>
        <nav className="portal-nav" aria-label="Club navigation">
          <Link href="/training">Training schedule</Link>
          <Link href="/faq">FAQ</Link>
          <Link href="/coach">Coach portal</Link>
        </nav>
      </header>

      <section className="info-hero" aria-labelledby="suggestions-title">
        <div className="info-hero-copy">
          <p className="section-kicker">
            <span /> Your voice matters
          </p>
          <h1 id="suggestions-title">Suggestions &amp; feedback.</h1>
          <p className="info-hero-intro">
            Tell the club what is working and what could be better — from
            facilities and equipment to training and scheduling. You can submit
            anonymously.
          </p>
        </div>
      </section>

      <section className="info-section" aria-labelledby="suggestions-form-heading">
        <div className="schedule-heading compact-heading">
          <div>
            <p className="section-kicker">SEND A MESSAGE</p>
            <h2 id="suggestions-form-heading">Share your thoughts</h2>
          </div>
          <p className="timezone-note">All fields except the message are optional.</p>
        </div>
        <SuggestionsForm />
      </section>

      <section className="info-cta" aria-labelledby="suggestions-cta-heading">
        <h2 id="suggestions-cta-heading">Looking for an answer instead?</h2>
        <p>
          Membership fees, court and lighting rates, equipment rental, and
          booking questions are covered in the FAQ.
        </p>
        <Link className="request-submit" href="/faq">
          Read the FAQ
        </Link>
      </section>

      <footer className="training-footer">
        <span>
          MM<span className="footer-ball">●</span>U TENNIS CLUB
        </span>
        <span>Good things happen on court.</span>
      </footer>
    </main>
  );
}
