export const MAX_BOOKING_SLOTS: number;
export const MAX_BOOKING_DATES: number;

export type BookingSelectionSession = {
  id: string;
  program_name: string;
  location: string;
  starts_at: string;
  ends_at: string;
  status: string;
  capacity: number | null;
  remaining_capacity: number | null;
};

export function validateBookingSelection(
  selectedIDs: string[],
  sessions: BookingSelectionSession[],
): string | null;
