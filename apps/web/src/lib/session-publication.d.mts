export type PublicationScope = "day" | "month";

export type PublicationWindow = {
  start: string;
  end: string;
};

export type PublishableSession = {
  status: string;
  is_public: boolean;
  starts_at: string;
};

export function phtDateKey(value: string | number | Date): string;

export function publicationWindow(
  scope: PublicationScope,
  date: string,
): PublicationWindow;

export function eligibleUnpublishedSessions<T extends PublishableSession>(
  sessions: T[],
  scope: PublicationScope,
  date: string,
): T[];

export function skippedUnpublishedSessions<T extends PublishableSession>(
  sessions: T[],
  scope: PublicationScope,
  date: string,
): T[];
