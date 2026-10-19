import assert from "node:assert/strict";
import test from "node:test";
import {
  initialSuggestionState,
  suggestionReducer,
  validateSuggestion,
} from "../src/components/suggestions-form-state.mjs";

test("submitted submission cannot return to editing until reset", () => {
  const submitting = suggestionReducer(initialSuggestionState, { type: "start" });
  const submitted = suggestionReducer(submitting, { type: "success" });

  assert.deepEqual(submitted, { phase: "submitted", error: null });
  assert.equal(suggestionReducer(submitted, { type: "start" }), submitted);
  assert.deepEqual(suggestionReducer(submitted, { type: "reset" }), initialSuggestionState);
});

test("failed submission returns to editable form with the error", () => {
  const submitting = suggestionReducer(initialSuggestionState, { type: "start" });
  const failed = suggestionReducer(submitting, { type: "failure", error: "Too many requests." });

  assert.deepEqual(failed, { phase: "editing", error: "Too many requests." });
});

test("validation requires a known category and a message", () => {
  assert.equal(validateSuggestion({ category: "", message: "hi" }), "Choose a category.");
  assert.equal(validateSuggestion({ category: "other", message: "   " }), "Please enter your message.");
  assert.equal(validateSuggestion({ category: "other", message: "Court lights were great." }), null);
});

test("validation enforces the message length limit", () => {
  const tooLong = "a".repeat(2001);
  assert.equal(validateSuggestion({ category: "feedback", message: tooLong }), "Message must be 2000 characters or fewer.");
  assert.equal(validateSuggestion({ category: "feedback", message: "a".repeat(2000) }), null);
});

test("validation allows anonymous submissions but checks an optional email", () => {
  assert.equal(validateSuggestion({ category: "suggestion", message: "More weekend sessions", email: "" }), null);
  assert.equal(
    validateSuggestion({ category: "suggestion", message: "More weekend sessions", email: "not-an-email" }),
    "Enter a valid email address, or leave it blank to stay anonymous.",
  );
});
