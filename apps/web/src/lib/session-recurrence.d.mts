export type RecurrenceFrequency = "daily" | "weekdays" | "selected";

export type ConflictSession = {
  id: string;
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  status: string;
  assigned_coach_id: string | null;
};

export type ScheduleConflict = {
  sessionId: string;
  programName: string;
  location: string;
  startsAt: string;
  endsAt: string;
  reason: string;
};

export function addDateKey(dateKey: string, days: number): string;
export function getDateRangeError(startDate: string, endDate: string, maxDays?: number): string;
export function getTimeRangeError(startTime: string, endTime: string): string;
export function getOccurrenceDates(options: {
  startDate: string;
  endDate: string;
  frequency: RecurrenceFrequency;
  weekdays?: number[];
  maxDays?: number;
}): string[];
export function toPHTISO(dateKey: string, time: string): string;
export function phtTimeInput(isoDateTime: string): string;
export function findScheduleConflicts(options: {
  startsAt: string;
  endsAt: string;
  location: string;
  coachID?: string;
  sessions?: ConflictSession[];
}): ScheduleConflict[];
