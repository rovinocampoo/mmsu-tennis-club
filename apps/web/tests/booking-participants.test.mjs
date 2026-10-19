import assert from "node:assert/strict";
import test from "node:test";
import {
  addParticipant,
  createParticipant,
  joinParticipantNames,
  participantError,
  participantNames,
  PARTICIPANT_MAX_COUNT,
  removeParticipant,
} from "../src/components/booking-participants.mjs";

function participant(id, name) {
  return { id, name };
}

test("a fresh participant row starts empty and requires a name", () => {
  const participants = [createParticipant("p1")];
  assert.equal(participants[0].name, "");
  assert.equal(
    participantError(participants),
    "Enter a name for each participant, or remove the empty row.",
  );
});

test("adding a participant is blocked while an empty row exists", () => {
  const participants = [participant("p1", "")];
  assert.deepEqual(addParticipant(participants, "p2"), participants);
});

test("adding appends a new row once existing rows are filled", () => {
  const participants = [participant("p1", "Ada")];
  const next = addParticipant(participants, "p2");
  assert.equal(next.length, 2);
  assert.equal(next[1].id, "p2");
  assert.equal(next[1].name, "");
});

test("removing a participant keeps at least one row", () => {
  const single = [participant("p1", "Ada")];
  assert.deepEqual(removeParticipant(single, "p1"), single);

  const group = [participant("p1", "Ada"), participant("p2", "Grace")];
  const next = removeParticipant(group, "p1");
  assert.deepEqual(next, [participant("p2", "Grace")]);
});

test("names are trimmed and joined for submission", () => {
  const participants = [
    participant("p1", "  Ada Lovelace "),
    participant("p2", "Grace Hopper"),
  ];
  assert.equal(joinParticipantNames(participants), "Ada Lovelace, Grace Hopper");
});

test("a valid individual or group request passes validation", () => {
  assert.equal(participantError([participant("p1", "Ada")]), null);
  assert.equal(
    participantError([participant("p1", "Ada"), participant("p2", "Grace")]),
    null,
  );
});

test("whitespace-only participant names are rejected", () => {
  assert.equal(
    participantError([participant("p1", "Ada"), participant("p2", "   ")]),
    "Enter a name for each participant, or remove the empty row.",
  );
});

test("overly long combined participant names are rejected", () => {
  const participants = [participant("p1", "A".repeat(150)), participant("p2", "B".repeat(150))];
  assert.equal(
    participantError(participants),
    "Participant names are too long. Use fewer or shorter names.",
  );
});

test("participant names are returned trimmed and blank rows dropped", () => {
  const participants = [
    participant("p1", "  Alex  "),
    participant("p2", "Sam"),
    participant("p3", "   "),
  ];
  assert.deepEqual(participantNames(participants), ["Alex", "Sam"]);
});

test("adding beyond the participant cap is blocked", () => {
  const participants = Array.from({ length: PARTICIPANT_MAX_COUNT }, (_, index) =>
    participant(`p${index}`, `Player ${index}`),
  );
  assert.equal(addParticipant(participants, "extra"), participants);
  assert.equal(
    participantError([...participants, participant("extra", "One Too Many")]),
    `Add no more than ${PARTICIPANT_MAX_COUNT} participants.`,
  );
});
