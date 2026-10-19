"use client";

import { FormEvent, useReducer, useRef, useState } from "react";
import {
  initialSuggestionState,
  suggestionReducer,
  validateSuggestion,
  SUGGESTION_EMAIL_LIMIT,
  SUGGESTION_MESSAGE_LIMIT,
  SUGGESTION_NAME_LIMIT,
} from "./suggestions-form-state.mjs";

const categoryOptions = [
  { value: "suggestion", label: "Suggestion" },
  { value: "feedback", label: "Feedback" },
  { value: "facility", label: "Facility" },
  { value: "training", label: "Training" },
  { value: "scheduling", label: "Scheduling" },
  { value: "other", label: "Other" },
];

export default function SuggestionsForm() {
  const [state, dispatch] = useReducer(
    suggestionReducer,
    initialSuggestionState,
  );
  const [messageLength, setMessageLength] = useState(0);
  const submitting = useRef(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || state.phase === "submitting") return;

    const form = event.currentTarget;
    const data = new FormData(form);
    const payload = {
      category: String(data.get("category") || ""),
      message: String(data.get("message") || ""),
      name: String(data.get("name") || ""),
      email: String(data.get("email") || ""),
    };

    const validationError = validateSuggestion(payload);
    if (validationError) {
      dispatch({ type: "failure", error: validationError });
      return;
    }

    submitting.current = true;
    dispatch({ type: "start" });
    try {
      const response = await fetch("/api/backend/feedback-submissions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(
          typeof result?.error === "string"
            ? result.error
            : "We could not send your message. Please try again.",
        );
      }
      dispatch({ type: "success" });
      form.reset();
      setMessageLength(0);
    } catch (cause) {
      dispatch({
        type: "failure",
        error:
          cause instanceof Error
            ? cause.message
            : "We could not send your message. Please try again.",
      });
    } finally {
      submitting.current = false;
    }
  }

  if (state.phase === "submitted") {
    return (
      <div className="suggestion-success" role="status" aria-live="polite">
        <span className="suggestion-success-mark" aria-hidden="true">
          ✓
        </span>
        <h3>Thank you for your message.</h3>
        <p>
          Your submission has been received by the club. If you shared contact
          details, the club may reach out to follow up.
        </p>
        <button
          className="admin-button secondary"
          type="button"
          onClick={() => dispatch({ type: "reset" })}
        >
          Submit another message
        </button>
      </div>
    );
  }

  const submittingNow = state.phase === "submitting";

  return (
    <form className="suggestion-form" onSubmit={submit} noValidate>
      <label className="suggestion-field">
        Category
        <select name="category" defaultValue="" required disabled={submittingNow}>
          <option value="" disabled>
            Choose a category
          </option>
          {categoryOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>

      <label className="suggestion-field">
        Your message
        <textarea
          name="message"
          rows={7}
          maxLength={SUGGESTION_MESSAGE_LIMIT}
          required
          disabled={submittingNow}
          onChange={(event) => setMessageLength([...event.target.value].length)}
          placeholder="Share an idea, a concern, or something the club did well."
        />
        <span className="suggestion-counter" aria-hidden="true">
          {messageLength} / {SUGGESTION_MESSAGE_LIMIT} characters
        </span>
      </label>

      <div className="suggestion-identity">
        <label className="suggestion-field">
          Name (optional)
          <input
            name="name"
            type="text"
            autoComplete="name"
            maxLength={SUGGESTION_NAME_LIMIT}
            disabled={submittingNow}
            placeholder="You can stay anonymous"
          />
        </label>

        <label className="suggestion-field">
          Email or contact (optional)
          <input
            name="email"
            type="email"
            autoComplete="email"
            maxLength={SUGGESTION_EMAIL_LIMIT}
            disabled={submittingNow}
            placeholder="Only used to reply to you"
          />
        </label>
      </div>

      <p className="suggestion-privacy">
        Contact details are optional and are only used to respond to your
        submission. You can submit anonymously. Your message is private and is
        never shown publicly.
      </p>

      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}

      <button className="request-submit" type="submit" disabled={submittingNow}>
        {submittingNow ? "Sending…" : "Send to the club"}
      </button>
    </form>
  );
}
