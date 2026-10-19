import Link from "next/link";

export default function ClubMobileMenu() {
  return (
    <details className="club-mobile-menu">
      <summary aria-label="Open club navigation">
        <span className="club-menu-icon" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span>Menu</span>
      </summary>
      <nav className="club-mobile-menu-panel" aria-label="Club navigation">
        <Link href="/training">Training</Link>
        <Link href="/faq">FAQ</Link>
        <Link href="/suggestions">Suggestions &amp; feedback</Link>
        <Link href="/coach">Coach portal</Link>
        <Link href="/admin">Admin portal</Link>
      </nav>
    </details>
  );
}
