export type FeedbackRecord = {
  id: string;
  category: string;
  message: string;
  name: string | null;
  email: string | null;
  status: string;
  created_at: string;
  reviewed_at: string | null;
};

export type FeedbackSummary = {
  new: number;
  reviewed: number;
  archived: number;
  total: number;
};

export type FeedbackFilters = {
  query?: string;
  status?: string;
  category?: string;
  dateFrom?: string;
  dateTo?: string;
};

export const FEEDBACK_STATUSES: readonly string[];
export const FEEDBACK_CATEGORIES: readonly string[];
export const FEEDBACK_CATEGORY_LABELS: Record<string, string>;

export function summarizeSubmissions(submissions: FeedbackRecord[]): FeedbackSummary;
export function filterSubmissions<T extends FeedbackRecord>(
  submissions: T[],
  filters?: FeedbackFilters,
  dateKeyOf?: (submission: T) => string,
): T[];
export function escapeCsvValue(value: unknown): string;
export function toCsv(
  columns: { label: string; value: (row: FeedbackRecord) => unknown }[],
  rows: FeedbackRecord[],
): string;
export function buildFeedbackCsv(
  submissions: FeedbackRecord[],
  options?: { includeEmail?: boolean },
): string;
export function feedbackExportFilename(date?: Date): string;
