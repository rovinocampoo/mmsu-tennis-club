// Smooth scrolling that respects the user's reduced-motion preference. Shared
// by the public, admin, and coach calendars so every in-page jump behaves the
// same way.

export function prefersReducedMotion() {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

export function scrollToElement(id) {
  if (typeof document === "undefined") return;
  document.getElementById(id)?.scrollIntoView({
    behavior: prefersReducedMotion() ? "auto" : "smooth",
    block: "start",
  });
}
