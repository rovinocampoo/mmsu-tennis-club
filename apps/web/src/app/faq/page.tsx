import type { Metadata } from "next";
import Link from "next/link";
import FaqAccordion from "@/components/faq-accordion";

export const metadata: Metadata = {
  title: "FAQ | MMSU Tennis Club",
  description:
    "Answers about MMSU Tennis Club membership, dues, court and lighting fees, equipment rental, training, and booking.",
};

export default function FaqPage() {
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
          <Link href="/suggestions">Suggestions &amp; feedback</Link>
          <Link href="/coach">Coach portal</Link>
        </nav>
      </header>

      <section className="info-hero" aria-labelledby="faq-title">
        <div className="info-hero-copy">
          <p className="section-kicker">
            <span /> Club information
          </p>
          <h1 id="faq-title">Frequently asked questions.</h1>
          <p className="info-hero-intro">
            Membership, court and lighting fees, equipment rental, training,
            and booking — answered in one place. Search below or browse by
            topic.
          </p>
        </div>
      </section>

      <section className="info-section" aria-labelledby="fee-reference-heading">
        <div className="schedule-heading compact-heading">
          <div>
            <p className="section-kicker">AT A GLANCE</p>
            <h2 id="fee-reference-heading">Fee reference</h2>
          </div>
          <p className="timezone-note">All amounts in Philippine pesos (₱).</p>
        </div>

        <div className="info-fee-grid">
          <article className="info-fee-card">
            <h3>Membership</h3>
            <dl>
              <div>
                <dt>Members 18 and above (non-MMSU)</dt>
                <dd>
                  <strong>₱1,500.00</strong> initial ·{" "}
                  <strong>₱1,000.00</strong> annual renewal
                </dd>
              </div>
              <div>
                <dt>Members 18 and below</dt>
                <dd>
                  <strong>₱500.00</strong> initial ·{" "}
                  <strong>₱500.00</strong> annual renewal
                </dd>
              </div>
              <div>
                <dt>Reactivation after 2 years inactive</dt>
                <dd>
                  <strong>₱1,500.00</strong>
                </dd>
              </div>
            </dl>
          </article>

          <article className="info-fee-card">
            <h3>Court and lighting</h3>
            <dl>
              <div>
                <dt>Member light fee</dt>
                <dd>
                  <strong>₱20.00</strong> per person
                </dd>
              </div>
              <div>
                <dt>Non-member court fee</dt>
                <dd>
                  <strong>₱50.00</strong> per person per hour, paid on the spot
                </dd>
              </div>
              <div>
                <dt>MMSU employees, students, varsity</dt>
                <dd>Free court and lighting fees</dd>
              </div>
            </dl>
          </article>

          <article className="info-fee-card">
            <h3>Equipment rental</h3>
            <dl>
              <div>
                <dt>Ball rental</dt>
                <dd>
                  <strong>₱80.00</strong> per game
                </dd>
              </div>
              <div>
                <dt>Racket rental</dt>
                <dd>
                  <strong>₱20.00</strong> per game
                </dd>
              </div>
            </dl>
          </article>
        </div>

        <p className="info-note">
          Members qualify for free court and lighting fees for one year, subject
          to the club&apos;s membership rules. For membership eligibility and
          fee clarifications, please contact the club.
        </p>
      </section>

      <section
        className="info-section"
        aria-labelledby="faq-questions-heading"
      >
        <div className="schedule-heading compact-heading">
          <div>
            <p className="section-kicker">QUESTIONS</p>
            <h2 id="faq-questions-heading">Browse the FAQ</h2>
          </div>
        </div>
        <FaqAccordion />
      </section>

      <section className="info-cta" aria-labelledby="faq-cta-heading">
        <h2 id="faq-cta-heading">Still have a question?</h2>
        <p>
          Send the club a suggestion, concern, or question. Your contact details
          are optional.
        </p>
        <Link className="request-submit" href="/suggestions">
          Go to Suggestions &amp; Feedback
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
