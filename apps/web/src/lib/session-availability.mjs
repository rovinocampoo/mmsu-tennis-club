export const SESSION_UNAVAILABLE_FULL = "Fully booked";
export const SESSION_UNAVAILABLE_CLOSED = "Not accepting requests";

export function sessionAvailability(session) {
  if (session.status !== "open")
    return { label: "Registration closed", kind: "closed" };
  if (session.remaining_capacity === 0) return { label: "Full", kind: "full" };
  if (session.capacity === null) return { label: "Open", kind: "open" };
  return {
    label: `${session.remaining_capacity ?? 0} spaces left`,
    kind: "open",
  };
}

// The human-readable reason a session cannot be requested. Capacity full and
// "closed for any other reason" are deliberately distinct so a session that is
// closed but still has space is never shown as "Fully booked".
export function unavailableReason(kind) {
  return kind === "full" ? SESSION_UNAVAILABLE_FULL : SESSION_UNAVAILABLE_CLOSED;
}
