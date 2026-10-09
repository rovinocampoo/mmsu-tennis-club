import type { Metadata } from "next";
import Link from "next/link";
import { getUpcomingTrainingSessions, type TrainingSession } from "@/lib/training-sessions";

export const metadata: Metadata = {
  title: "Training Schedule | MMSU Tennis Club",
  description: "See upcoming public tennis training sessions from the MMSU Tennis Club.",
};

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

function sessionAvailability(status: string, remaining: number | null, capacity: number | null) {
  if (status !== "open") return { label: "Registration closed", kind: "closed" };
  if (remaining === 0) return { label: "Full", kind: "full" };
  if (capacity === null) return { label: "Open", kind: "open" };
  return { label: `${remaining} spaces left`, kind: "open" };
}

export default async function TrainingPage() {
  let sessions: TrainingSession[] = [];
  let unavailable = false;

  try {
    sessions = await getUpcomingTrainingSessions();
  } catch {
    unavailable = true;
  }

  return (
    <main className="training-page">
      <header className="training-header">
        <Link className="brand" href="/" aria-label="MMSU Tennis Club home">
          <span className="brand-mark" aria-hidden="true">M</span>
          <span><strong>MMSU</strong><small>TENNIS CLUB</small></span>
        </Link>
        <span className="header-note">Mariano Marcos State University</span>
      </header>

      <section className="training-hero" aria-labelledby="training-title">
        <div className="hero-copy">
          <p className="section-kicker"><span /> On court, together</p>
          <h1 id="training-title">Find your next<br />day on court.</h1>
          <p className="hero-intro">Explore upcoming training sessions from MMSU Tennis Club. Find a time to learn, practice, and enjoy the game with us.</p>
        </div>
        <div className="hero-court" aria-hidden="true">
          <div className="court-lines"><i /><i /><i /><i /></div>
          <span className="ball-mark" />
          <span className="court-caption">ILOCOS NORTE · PHILIPPINES</span>
        </div>
      </section>

      <section className="schedule-section" aria-labelledby="schedule-heading">
        <div className="schedule-heading">
          <div>
            <p className="section-kicker">MAKE TIME FOR THE GAME</p>
            <h2 id="schedule-heading">Upcoming training</h2>
          </div>
          <p className="timezone-note">All times are Philippine Time (PHT)</p>
        </div>

        {unavailable ? (
          <div className="state-panel error-panel" role="alert">
            <span className="state-icon" aria-hidden="true">!</span>
            <div><h3>Schedule temporarily unavailable</h3><p>We couldn’t load the training schedule right now. Please try again in a little while.</p></div>
          </div>
        ) : sessions?.length === 0 ? (
          <div className="state-panel empty-panel">
            <span className="empty-court" aria-hidden="true"><i /></span>
            <div><p className="section-kicker">SEE YOU SOON</p><h3>No sessions just yet</h3><p>No upcoming training sessions are currently published. Check back soon for the next chance to get on court.</p></div>
          </div>
        ) : (
          <div className="session-list">
            {sessions?.map((session) => {
              const availability = sessionAvailability(session.status, session.remaining_capacity, session.capacity);
              return (
                <article className="session-card" key={session.id}>
                  <div className="session-date"><span>{manilaDate.format(new Date(session.starts_at)).split(",")[0]}</span><strong>{new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", day: "2-digit" }).format(new Date(session.starts_at))}</strong><small>{new Intl.DateTimeFormat("en-PH", { timeZone: "Asia/Manila", month: "short" }).format(new Date(session.starts_at))}</small></div>
                  <div className="session-main">
                    <div className="session-title-row"><h3>{session.program_name}</h3><span className={`availability availability-${availability.kind}`}>{availability.label}</span></div>
                    {session.program_description && <p className="session-description">{session.program_description}</p>}
                    <div className="session-details">
                      <span><span className="detail-icon" aria-hidden="true">◷</span>{manilaDate.format(new Date(session.starts_at))}</span>
                      <span><span className="detail-icon" aria-hidden="true">◴</span>{manilaTime.format(new Date(session.starts_at))} – {manilaTime.format(new Date(session.ends_at))}</span>
                      <span><span className="detail-icon" aria-hidden="true">⌖</span>{session.location}</span>
                    </div>
                  </div>
                  {session.status === "open" && session.capacity !== null && session.remaining_capacity !== 0 && <div className="session-spaces"><strong>{session.remaining_capacity}</strong><span>spaces left</span></div>}
                </article>
              );
            })}
          </div>
        )}
      </section>

      <footer className="training-footer"><span>MM<span className="footer-ball">●</span>U TENNIS CLUB</span><span>Good things happen on court.</span></footer>
    </main>
  );
}
