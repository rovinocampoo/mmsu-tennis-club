import type { Metadata } from "next";
import Link from "next/link";
import { getActiveTrainingPrograms, getUpcomingTrainingSessions, type TrainingProgram, type TrainingSession } from "@/lib/training-sessions";
import TrainingSchedule from "@/components/training-schedule";

export const metadata: Metadata = {
  title: "Training Schedule | MMSU Tennis Club",
  description: "See upcoming public tennis training sessions from the MMSU Tennis Club.",
};

export default async function TrainingPage() {
  let sessions: TrainingSession[] = [];
  let programs: TrainingProgram[] = [];
  let sessionsUnavailable = false;
  let programsUnavailable = false;
  const monthParts = new Intl.DateTimeFormat("en", { timeZone: "Asia/Manila", year: "numeric", month: "2-digit" }).formatToParts(new Date());
  const initialMonth = `${monthParts.find((part) => part.type === "year")?.value}-${monthParts.find((part) => part.type === "month")?.value}`;

  const [sessionResult, programResult] = await Promise.allSettled([
    getUpcomingTrainingSessions(),
    getActiveTrainingPrograms(),
  ]);
  if (sessionResult.status === "fulfilled") sessions = sessionResult.value;
  else sessionsUnavailable = true;
  if (programResult.status === "fulfilled") programs = programResult.value;
  else programsUnavailable = true;

  return (
    <main className="training-page">
      <header className="training-header">
        <Link className="brand" href="/" aria-label="MMSU Tennis Club home">
          <span className="brand-mark" aria-hidden="true">M</span>
          <span><strong>MMSU</strong><small>TENNIS CLUB</small></span>
        </Link>
        <span className="header-note"><Link href="/coach">Coach portal</Link> · <Link href="/admin">Club admin</Link></span>
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
        <TrainingSchedule
          sessions={sessions}
          programs={programs}
          programsUnavailable={programsUnavailable}
          sessionsUnavailable={sessionsUnavailable}
          initialMonth={initialMonth}
        />
      </section>

      <footer className="training-footer"><span>MM<span className="footer-ball">●</span>U TENNIS CLUB</span><span>Good things happen on court.</span></footer>
    </main>
  );
}
