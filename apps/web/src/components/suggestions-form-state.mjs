export const SUGGESTION_CATEGORIES = [
  "suggestion",
  "feedback",
  "facility",
  "training",
  "scheduling",
  "other",
];

export const SUGGESTION_MESSAGE_LIMIT = 2000;
export const SUGGESTION_NAME_LIMIT = 200;
export const SUGGESTION_EMAIL_LIMIT = 254;

export const initialSuggestionState = {
  phase: "editing",
  error: null,
};

export function suggestionReducer(state, action) {
  switch (action.type) {
    case "start":
      return state.phase === "editing"
        ? { phase: "submitting", error: null }
        : state;
    case "success":
      return state.phase === "submitting"
        ? { phase: "submitted", error: null }
        : state;
    case "failure":
      return { phase: "editing", error: action.error };
    case "reset":
      return initialSuggestionState;
    default:
      return state;
  }
}

export function validateSuggestion({ category, message, email }) {
  if (!SUGGESTION_CATEGORIES.includes((category ?? "").trim()))
    return "Choose a category.";
  const trimmedMessage = (message ?? "").trim();
  if (!trimmedMessage) return "Please enter your message.";
  if ([...trimmedMessage].length > SUGGESTION_MESSAGE_LIMIT)
    return `Message must be ${SUGGESTION_MESSAGE_LIMIT} characters or fewer.`;
  const cleanEmail = (email ?? "").trim();
  if (cleanEmail && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(cleanEmail))
    return "Enter a valid email address, or leave it blank to stay anonymous.";
  return null;
}
