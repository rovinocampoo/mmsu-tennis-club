// Participant helpers for the public training request form.
//
// The form collects one name per participant. The server persists each name as
// a normalized row and keeps a comma-joined full_name for backward-compatible
// display and reports. These helpers are pure so add/remove/validation
// behavior can be unit tested.

export const PARTICIPANT_TOTAL_NAME_LIMIT = 200;
export const PARTICIPANT_MAX_COUNT = 20;

let participantSequence = 0;

export function createParticipant(id) {
  participantSequence += 1;
  return { id: id ?? `participant-${participantSequence}`, name: "" };
}

export function addParticipant(participants, id) {
  // Never stack empty rows: if a row is still blank, keep the list as-is so the
  // user fills the existing row instead of creating duplicate empty entries.
  if (participants.some((participant) => participant.name.trim() === "")) {
    return participants;
  }
  if (participants.length >= PARTICIPANT_MAX_COUNT) {
    return participants;
  }
  return [...participants, createParticipant(id)];
}

export function removeParticipant(participants, id) {
  // Always keep at least one participant row.
  if (participants.length <= 1) return participants;
  const next = participants.filter((participant) => participant.id !== id);
  return next.length > 0 ? next : participants;
}

export function participantNames(participants) {
  return participants.map((participant) => participant.name.trim()).filter(Boolean);
}

export function joinParticipantNames(participants) {
  return participantNames(participants).join(", ");
}

export function participantError(participants) {
  if (participants.length === 0) return "Add at least one participant name.";
  if (participants.length > PARTICIPANT_MAX_COUNT) {
    return `Add no more than ${PARTICIPANT_MAX_COUNT} participants.`;
  }
  const names = participants.map((participant) => participant.name.trim());
  if (names.some((name) => name === "")) {
    return "Enter a name for each participant, or remove the empty row.";
  }
  const joined = names.join(", ");
  if ([...joined].length > PARTICIPANT_TOTAL_NAME_LIMIT) {
    return "Participant names are too long. Use fewer or shorter names.";
  }
  return null;
}
