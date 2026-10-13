import Link from "next/link";

export default function Home() {
  return (
    <main className="landing">
      <div className="court-mark" aria-hidden="true">
        <span />
      </div>
      <p className="eyebrow">Mariano Marcos State University</p>
      <h1>MMSU Tennis Club</h1>
      <p className="intro">
        Find a published training session, make time to practice, and enjoy the game with the MMSU community.
      </p>
      <p className="cta"><Link href="/training">Explore training sessions</Link></p>
      <nav className="landing-links" aria-label="Club portals">
        <Link href="/coach">Coach portal</Link>
        <Link href="/admin">Club admin</Link>
      </nav>
    </main>
  );
}
