export const MAX_BOOKING_SLOTS = 16;
export const MAX_BOOKING_DATES = 8;

const manilaDateKey = (value) =>
  new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Manila",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(value));

export function validateBookingSelection(selectedIDs, sessions) {
  if (sessions.length !== selectedIDs.length) {
    return "A selected slot is no longer available. Refresh the page and select your times again.";
  }
  if (new Set(selectedIDs).size !== selectedIDs.length) {
    return "A session slot was selected more than once. Remove the duplicate slot.";
  }
  if (sessions.length > MAX_BOOKING_SLOTS) {
    return `A booking can include no more than ${MAX_BOOKING_SLOTS} session slots.`;
  }
  const dates = new Set(sessions.map((session) => manilaDateKey(session.starts_at)));
  if (dates.size > MAX_BOOKING_DATES) {
    return `A booking can include sessions on no more than ${MAX_BOOKING_DATES} dates.`;
  }
  if (
    sessions.some(
      (session) =>
        session.status !== "open" ||
        (session.capacity !== null &&
          (session.remaining_capacity === null ||
            session.remaining_capacity <= 0)),
    )
  ) {
    return "One or more selected slots are no longer available. Remove them or refresh the schedule.";
  }
  if (sessions.length < 2) return null;

  const programName = sessions[0].program_name;
  const location = sessions[0].location.trim().toLocaleLowerCase();
  if (sessions.some((session) =>
    session.program_name !== programName ||
    session.location.trim().toLocaleLowerCase() !== location,
  )) {
    return "Choose sessions from the same training program and location.";
  }

  const chronological = [...sessions].sort((left, right) => left.starts_at.localeCompare(right.starts_at));
  let latestEnd = Date.parse(chronological[0].ends_at);
  for (let index = 1; index < chronological.length; index += 1) {
    const startsAt = Date.parse(chronological[index].starts_at);
    if (startsAt < latestEnd) {
      return "Selected session times overlap. Remove one of the conflicting slots.";
    }
    latestEnd = Math.max(latestEnd, Date.parse(chronological[index].ends_at));
  }

  return null;
}
