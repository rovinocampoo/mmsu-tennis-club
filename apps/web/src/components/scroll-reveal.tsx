"use client";

import { useEffect } from "react";

/** One-time viewport reveals for marked elements on the public training page. */
export default function ScrollReveal() {
  useEffect(() => {
    const root = document.querySelector<HTMLElement>(".training-page");
    if (!root || !("IntersectionObserver" in window)) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    root.classList.add("motion-enabled");

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          entry.target.classList.add("is-revealed");
          observer.unobserve(entry.target);
        }
      },
      { threshold: 0.12, rootMargin: "0px 0px -40px 0px" },
    );

    const observeMarkedElements = () => {
      root
        .querySelectorAll<HTMLElement>("[data-reveal]:not(.is-revealed)")
        .forEach((element) => observer.observe(element));
    };

    observeMarkedElements();

    // Observe session cards inserted after calendar date or tab changes.
    const mutations = new MutationObserver(observeMarkedElements);
    mutations.observe(root, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["hidden"],
    });

    return () => {
      mutations.disconnect();
      observer.disconnect();
      root.classList.remove("motion-enabled");
    };
  }, []);

  return null;
}