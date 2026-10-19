import Link from "next/link";

export default function AdminMobileMenu() {
  return (
    <details className="club-mobile-menu">
      <summary aria-label="Open admin navigation">
        <span className="club-menu-icon" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span>Menu</span>
      </summary>
      <nav className="club-mobile-menu-panel" aria-label="Admin navigation">
        <Link href="/training">Public schedule</Link>
        <Link href="/admin/approved-clients">Approved clients</Link>
        <Link href="/admin/coaches">Coaches</Link>
        <Link href="/admin/feedback">Feedback</Link>
        <a
          href="https://rallyledger-web.vercel.app/"
          rel="noopener noreferrer"
          aria-label="Open RallyLedger"
        >
          RallyLedger ↗
        </a>
      </nav>
    </details>
  );
}
