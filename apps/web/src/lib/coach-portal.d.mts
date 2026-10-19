export type CoachPortalPeriod = "week" | "month";

export type CoachScheduleSession = {
  id: string;
  program_name: string;
  program_description: string | null;
  starts_at: string;
  ends_at: string;
  location: string;
  status: string;
  is_public: boolean;
  assigned_coach_id: string | null;
  assigned_coach_name: string | null;
  participant_names: string[];
  confirmed_booking_count: number;
};

export type CoachDirectoryEntry = {
  id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
  notes: string | null;
  is_active: boolean;
};

export type CoachPeriodRange = {
  start: Date;
  end: Date;
  startKey: string;
  endKey: string;
  label: string;
};

export type CoachScheduleSummary = {
  total: number;
  upcoming: number;
  assigned: number;
  unassigned: number;
  participantCount: number;
};

export type CoachCalendarCell = {
  key: string;
  day: number;
  inMonth: boolean;
};

export function phtDateKey(value: Date | string): string;
export function periodRange(period: CoachPortalPeriod, anchor: string): CoachPeriodRange | null;
export function shiftPeriod(period: CoachPortalPeriod, anchor: string, direction: number): string;
export function calendarGrid(year: number, month: number): CoachCalendarCell[];
export function sessionInRange(session: { starts_at: string }, range: CoachPeriodRange | null): boolean;
export function sessionsInRange<T extends { starts_at: string }>(sessions: T[], range: CoachPeriodRange | null): T[];
export function sortSessionsChronologically<T extends { starts_at: string }>(sessions: T[]): T[];
export function summarizeSchedule(sessions: CoachScheduleSession[], now?: Date | string): CoachScheduleSummary;
export function rosterLabel(session: { participant_names?: string[] }): string;
