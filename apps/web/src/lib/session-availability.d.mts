export type SessionAvailabilityKind = "open" | "full" | "closed";

export type SessionAvailability = {
  label: string;
  kind: SessionAvailabilityKind;
};

export const SESSION_UNAVAILABLE_FULL: string;
export const SESSION_UNAVAILABLE_CLOSED: string;

export function sessionAvailability(session: {
  status: string;
  capacity: number | null;
  remaining_capacity: number | null;
}): SessionAvailability;

export function unavailableReason(kind: SessionAvailabilityKind): string;
