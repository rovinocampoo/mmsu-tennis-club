"use client";

import { useId, useMemo, useState } from "react";

type FaqItem = {
  question: string;
  answer: string[];
};

type FaqSection = {
  id: string;
  title: string;
  items: FaqItem[];
};

export const faqSections: FaqSection[] = [
  {
    id: "membership",
    title: "Membership and dues",
    items: [
      {
        question: "How much does membership cost?",
        answer: [
          "Membership fees depend on the member's age group.",
          "Non-MMSU members aged 18 and above pay an initial membership fee of ₱1,500.00 and an annual renewal fee of ₱1,000.00.",
          "Members aged 18 and below pay an initial membership fee of ₱500.00 and an annual renewal fee of ₱500.00.",
        ],
      },
      {
        question: "Do members get free court and lighting fees?",
        answer: [
          "Members qualify for free court and lighting fees for one year, subject to the club's membership rules.",
          "After the applicable free period, only the fees confirmed by the club apply.",
        ],
      },
      {
        question: "What happens if I stop paying my dues?",
        answer: [
          "Two years of non-payment makes a membership inactive.",
          "An inactive member must pay a reactivation membership fee of ₱1,500.00.",
        ],
      },
      {
        question: "Who can confirm my eligibility and fees?",
        answer: [
          "For membership eligibility and fee clarifications, please contact the club. You can also reach the club through the Suggestions & Feedback page.",
        ],
      },
    ],
  },
  {
    id: "court-lighting",
    title: "Court and lighting fees",
    items: [
      {
        question: "What is the member lighting fee?",
        answer: ["The member light fee is ₱20.00 per person."],
      },
      {
        question: "What does a non-member pay to play?",
        answer: [
          "A non-member court fee is ₱50.00 per person per hour, paid on the spot.",
        ],
      },
      {
        question: "Is there a separate non-member lighting fee?",
        answer: [
          "No separate non-member lighting fee has been published by the club. The confirmed non-member charge is the ₱50.00 per person per hour court fee.",
        ],
      },
      {
        question: "Which members play free?",
        answer: [
          "MMSU employees, students, and varsity members have free court and lighting fees, according to the club.",
        ],
      },
    ],
  },
  {
    id: "equipment",
    title: "Equipment rental",
    items: [
      {
        question: "Can I rent equipment on site?",
        answer: [
          "Ball rental is ₱80.00 per game.",
          "Racket rental is ₱20.00 per game.",
        ],
      },
    ],
  },
  {
    id: "training",
    title: "Training and scheduling",
    items: [
      {
        question: "How do I find training sessions?",
        answer: [
          "Browse the published training schedule on the Training page. It lists the club's active programs and upcoming sessions, including their times and venues.",
        ],
      },
      {
        question: "Do you publish coach schedules or training prices?",
        answer: [
          "The club does not publish coach names, training prices, or package tiers here. Coach availability is managed internally by the club.",
          "For questions about training, contact the club through the Suggestions & Feedback page.",
        ],
      },
    ],
  },
  {
    id: "booking",
    title: "Booking and cancellations",
    items: [
      {
        question: "How do I book a session?",
        answer: [
          "Open a session on the Training page and send a booking request with your contact details.",
          "A request is not a confirmed reservation. A club admin reviews each request before a spot is confirmed.",
        ],
      },
      {
        question: "How do I know if my request was approved?",
        answer: [
          "When you submit a request, the club gives you a private status link. Keep it safe and use it to check whether your request is pending, approved, or rejected.",
        ],
      },
      {
        question: "What is the cancellation policy?",
        answer: [
          "The club has not published a cancellation or refund policy. For cancellations or schedule changes, contact the club through the Suggestions & Feedback page.",
        ],
      },
    ],
  },
  {
    id: "getting-started",
    title: "Getting started",
    items: [
      {
        question: "How do I join the MMSU Tennis Club?",
        answer: [
          "Contact the club to confirm your eligibility and the correct membership fee for your situation.",
          "Membership fees, annual renewals, and reactivation rules are summarized in the Membership and dues section.",
        ],
      },
      {
        question: "I'm new to tennis. Can I still join a session?",
        answer: [
          "Yes. Browse the published training schedule to find a session that fits your level and schedule, then send a booking request.",
        ],
      },
    ],
  },
];

export default function FaqAccordion() {
  const baseId = useId().replace(/:/g, "");
  const [query, setQuery] = useState("");
  const [openItems, setOpenItems] = useState<Set<string>>(new Set());

  const normalizedQuery = query.trim().toLowerCase();

  const filteredSections = useMemo(() => {
    if (!normalizedQuery) return faqSections;
    return faqSections
      .map((section) => ({
        ...section,
        items: section.items.filter((item) =>
          [item.question, ...item.answer]
            .join(" ")
            .toLowerCase()
            .includes(normalizedQuery),
        ),
      }))
      .filter((section) => section.items.length > 0);
  }, [normalizedQuery]);

  const matchCount = filteredSections.reduce(
    (total, section) => total + section.items.length,
    0,
  );

  function toggle(itemKey: string) {
    setOpenItems((current) => {
      const next = new Set(current);
      if (next.has(itemKey)) next.delete(itemKey);
      else next.add(itemKey);
      return next;
    });
  }

  return (
    <div className="faq-accordion">
      <div className="faq-search">
        <label htmlFor={`${baseId}-search`}>
          Search the FAQ
          <input
            id={`${baseId}-search`}
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Try “membership fee”, “rental”, or “booking”"
            autoComplete="off"
          />
        </label>
        <p className="faq-search-count" role="status" aria-live="polite">
          {normalizedQuery
            ? `${matchCount} ${matchCount === 1 ? "answer" : "answers"} match “${query.trim()}”.`
            : `Browse ${faqSections.length} topics below.`}
        </p>
      </div>

      {filteredSections.length === 0 ? (
        <div className="state-panel empty-panel" role="status">
          <div>
            <p className="section-kicker">NO MATCHES</p>
            <h3>We couldn&apos;t find that answer.</h3>
            <p>
              Try a different keyword, or send your question to the club through
              the Suggestions &amp; Feedback page.
            </p>
          </div>
        </div>
      ) : (
        filteredSections.map((section) => (
          <section
            className="faq-group"
            key={section.id}
            aria-labelledby={`${baseId}-${section.id}`}
          >
            <h2 id={`${baseId}-${section.id}`}>{section.title}</h2>
            <div className="faq-list">
              {section.items.map((item) => {
                const itemKey = item.question;
                const slug = `${section.id}-${item.question
                  .toLowerCase()
                  .replace(/[^a-z0-9]+/g, "-")
                  .replace(/^-|-$/g, "")}`;
                const buttonId = `${baseId}-${slug}-button`;
                const panelId = `${baseId}-${slug}-panel`;
                const isOpen = openItems.has(itemKey);
                return (
                  <article className="faq-item" key={itemKey}>
                    <h3 className="faq-question-heading">
                      <button
                        id={buttonId}
                        type="button"
                        className="faq-question"
                        aria-expanded={isOpen}
                        aria-controls={panelId}
                        onClick={() => toggle(itemKey)}
                      >
                        <span>{item.question}</span>
                        <span className="faq-chevron" aria-hidden="true">
                          {isOpen ? "−" : "+"}
                        </span>
                      </button>
                    </h3>
                    <div
                      id={panelId}
                      role="region"
                      aria-labelledby={buttonId}
                      className="faq-answer"
                      hidden={!isOpen}
                    >
                      {item.answer.map((paragraph) => (
                        <p key={paragraph}>{paragraph}</p>
                      ))}
                    </div>
                  </article>
                );
              })}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
