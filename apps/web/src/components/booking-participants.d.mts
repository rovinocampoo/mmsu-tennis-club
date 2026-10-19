export type Participant = { id: string; name: string };

export const PARTICIPANT_TOTAL_NAME_LIMIT: number;
export const PARTICIPANT_MAX_COUNT: number;
export function createParticipant(id?: string): Participant;
export function addParticipant(participants: Participant[], id?: string): Participant[];
export function removeParticipant(participants: Participant[], id: string): Participant[];
export function participantNames(participants: Participant[]): string[];
export function joinParticipantNames(participants: Participant[]): string;
export function participantError(participants: Participant[]): string | null;
