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
        A home for players to learn, practice, and enjoy the game together.
      </p>
      <p className="status">Our club website is getting ready.</p>
      <p className="cta"><Link href="/training">View upcoming training sessions</Link></p>
    </main>
  );
}
