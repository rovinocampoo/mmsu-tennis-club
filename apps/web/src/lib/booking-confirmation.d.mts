export type BookingCalendarData = {
  request_reference: string;
  status: "pending" | "partially_decided" | "approved" | "rejected";
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  requested_sessions?: Array<{ session_id: string; program_name: string; starts_at: string; ends_at: string; location: string; status: "pending" | "approved" | "rejected" }>;
};
export type BookingUpdateData = BookingCalendarData & { next_steps: string[] };
export type AdminApprovalData = {
  full_name: string;
  starts_at?: string;
  ends_at?: string;
  location?: string;
  sessions?: Array<{ starts_at: string; ends_at: string; location: string }>;
};
export function createGoogleCalendarURL(status: BookingCalendarData): string;
export function createGoogleCalendarURLs(status: BookingCalendarData): Array<{ session_id: string; program_name: string; starts_at: string; url: string }>;
export function createCalendarFile(status: BookingCalendarData): string;
export function createBookingMessage(status: BookingUpdateData, statusPageURL: string): string;
export function createAdminApprovalMessage(request: AdminApprovalData): string;
export function getCalendarFilename(reference: string): string;
