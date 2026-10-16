import Link from "next/link";

export default function Home() {
  return (
    <main className="home-page">
      <header className="home-header">
        <Link
          className="home-brand"
          href="/"
          aria-label="MMSU Tennis Club home"
        >
          <span className="home-brand-mark" aria-hidden="true">
            M
          </span>
          <span>
            <strong>MMSU</strong>
            <small>TENNIS CLUB</small>
          </span>
        </Link>

        <nav className="home-nav" aria-label="Main navigation">
          <Link href="#club">The club</Link>
          <Link href="#how-it-works">How it works</Link>
          <Link className="home-nav-cta" href="/training">
            Training schedule <span aria-hidden="true">↗</span>
          </Link>
        </nav>
      </header>

      <section className="home-hero" aria-labelledby="home-title">
        <div className="home-hero-photo" aria-hidden="true" />

        <div className="home-hero-content">
          <p className="home-eyebrow">
            <span aria-hidden="true" />
            MARIANO MARCOS STATE UNIVERSITY · ILOCOS NORTE
          </p>

          <h1 id="home-title">
            YOUR GAME.
            <br />
            YOUR <span>COMMUNITY.</span>
          </h1>

          <p className="home-hero-description">
            Good things happen on court. Find a training session, meet the
            tennis community, and make time for your next rally.
          </p>

          <div className="home-hero-actions">
            <Link className="home-primary-cta" href="/training">
              Explore training
              <span aria-hidden="true">↗</span>
            </Link>

            <a className="home-secondary-cta" href="#club">
              Discover the club
              <span aria-hidden="true">↓</span>
            </a>
          </div>
        </div>

        <div className="home-hero-caption">
          <span className="home-caption-line" />
          <span>MEET US ON COURT</span>
        </div>

        <div className="home-hero-number" aria-hidden="true">
          MMXXVI <span>·</span> TENNIS
        </div>
      </section>

      <div className="home-court-rule" aria-hidden="true">
        <span className="home-rally-ball" />
      </div>

      <section
        className="home-intro home-reveal"
        id="club"
        aria-labelledby="home-intro-title"
      >
        <div className="home-section-label">
          <span>01</span>
          <span>MORE THAN A GAME</span>
        </div>

        <div className="home-intro-grid">
          <h2 id="home-intro-title">
            Find your rhythm.
            <br />
            <span>Find your people.</span>
          </h2>

          <div className="home-intro-copy">
            <p>
              Tennis is better when there&apos;s a community behind every rally.
              MMSU Tennis Club brings people together through the game, one
              session at a time.
            </p>

            <p>
              Whether you&apos;re looking for your next practice or exploring
              ways to get on court, start with the schedule.
            </p>

            <Link className="home-text-link" href="/training">
              Find a training session <span aria-hidden="true">↗</span>
            </Link>
          </div>
        </div>

        <div className="home-values">
          <article className="home-value-card">
            <span className="home-value-index">01 / PLAY</span>
            <div className="home-value-icon" aria-hidden="true">
              <svg viewBox="0 0 48 48" fill="none">
                <circle cx="24" cy="24" r="15" />
                <path d="M15 12C24 20 24 28 15 36M33 12C24 20 24 28 33 36" />
              </svg>
            </div>
            <h3>Get on court</h3>
            <p>Explore the published training schedule and find a session.</p>
          </article>

          <article className="home-value-card">
            <span className="home-value-index">02 / CONNECT</span>
            <div className="home-value-icon" aria-hidden="true">
              <svg viewBox="0 0 48 48" fill="none">
                <circle cx="18" cy="17" r="7" />
                <circle cx="32" cy="19" r="5" />
                <path d="M5 39C5 30 10 26 18 26S31 30 31 39M29 29C37 27 43 31 43 39" />
              </svg>
            </div>
            <h3>Meet your community</h3>
            <p>
              Make practice a shared experience and enjoy the game together.
            </p>
          </article>

          <article className="home-value-card">
            <span className="home-value-index">03 / IMPROVE</span>
            <div className="home-value-icon" aria-hidden="true">
              <svg viewBox="0 0 48 48" fill="none">
                <path d="M8 38L20 26L27 31L41 13" />
                <path d="M29 13H41V25" />
                <path d="M8 10V38H41" />
              </svg>
            </div>
            <h3>Keep improving</h3>
            <p>Make room for more practice, more rallies, and new goals.</p>
          </article>
        </div>
      </section>

      <section
        className="home-process home-reveal"
        id="how-it-works"
        aria-labelledby="home-process-title"
      >
        <div className="home-process-heading">
          <div className="home-section-label home-section-label-light">
            <span>02</span>
            <span>YOUR NEXT SESSION</span>
          </div>

          <h2 id="home-process-title">
            Three steps.
            <br />
            <span>One court.</span>
          </h2>

          <p>
            Getting started is simple. Find a session, send your request, and
            wait for the club&apos;s confirmation.
          </p>
        </div>

        <div className="home-steps">
          <article className="home-step">
            <span className="home-step-number">01</span>
            <div>
              <h3>Find your session</h3>
              <p>Browse the schedule for available training sessions.</p>
            </div>
            <span className="home-step-arrow" aria-hidden="true">
              ↗
            </span>
          </article>

          <article className="home-step">
            <span className="home-step-number">02</span>
            <div>
              <h3>Send a request</h3>
              <p>Choose a session and submit your contact details.</p>
            </div>
            <span className="home-step-arrow" aria-hidden="true">
              ↗
            </span>
          </article>

          <article className="home-step">
            <span className="home-step-number">03</span>
            <div>
              <h3>Wait for confirmation</h3>
              <p>Your request is reviewed before your spot is confirmed.</p>
            </div>
            <span className="home-step-arrow" aria-hidden="true">
              ↗
            </span>
          </article>
        </div>

        <Link className="home-primary-cta" href="/training">
          View training schedule
          <span aria-hidden="true">↗</span>
        </Link>
      </section>

      <section className="home-final-cta home-reveal">
        <div className="home-final-court-lines" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>

        <p className="home-eyebrow">
          <span aria-hidden="true" />
          YOUR NEXT RALLY STARTS HERE
        </p>

        <h2>
          READY TO GET
          <br />
          <span>ON COURT?</span>
        </h2>

        <p>Find a session that fits your schedule and take the next step.</p>

        <Link className="home-primary-cta" href="/training">
          Explore training sessions
          <span aria-hidden="true">↗</span>
        </Link>
      </section>

      <footer className="home-footer">
        <Link className="home-brand home-footer-brand" href="/">
          <span className="home-brand-mark" aria-hidden="true">
            M
          </span>
          <span>
            <strong>MMSU</strong>
            <small>TENNIS CLUB</small>
          </span>
        </Link>

        <p>MADE FOR THE LOVE OF THE GAME.</p>

        <nav aria-label="Footer navigation">
          <Link href="/training">Training</Link>
          <Link href="/coach">Coach portal</Link>
          <Link href="/admin">Club admin</Link>
        </nav>
      </footer>
    </main>
  );
}
