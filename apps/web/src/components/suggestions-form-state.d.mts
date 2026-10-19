export type SuggestionPhase = "editing" | "submitting" | "submitted";

export type SuggestionState = {
  phase: SuggestionPhase;
  error: string | null;
};

export type SuggestionAction =
  | { type: "start" }
  | { type: "success" }
  | { type: "failure"; error: string }
  | { type: "reset" };

export const SUGGESTION_CATEGORIES: readonly string[];
export const SUGGESTION_MESSAGE_LIMIT: number;
export const SUGGESTION_NAME_LIMIT: number;
export const SUGGESTION_EMAIL_LIMIT: number;
export const initialSuggestionState: SuggestionState;
export function suggestionReducer(state: SuggestionState, action: SuggestionAction): SuggestionState;
export function validateSuggestion(input: {
  category?: string;
  message?: string;
  email?: string;
}): string | null;
