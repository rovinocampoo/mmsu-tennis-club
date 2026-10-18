export type BookingCalendarData = {
  request_reference: string;
  status: "pending" | "approved" | "rejected";
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
};
export type BookingUpdateData = BookingCalendarData & { next_steps: string[] };
export type AdminApprovalData = { full_name: string; starts_at: string; ends_at: string; location: string };
export function createGoogleCalendarURL(status: BookingCalendarData): string;
export function createCalendarFile(status: BookingCalendarData): string;
export function createBookingMessage(status: BookingUpdateData, statusPageURL: string): string;
export function createAdminApprovalMessage(request: AdminApprovalData): string;
export function getCalendarFilename(reference: string): string;
