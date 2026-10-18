"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import styles from "./admin-slot-planner.module.css";

type AdminFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

type Availability = {
  id: string;
  coach_id: string;
  coach_name: string;
  created_at: string;
  starts_at: string;
  ends_at: string;
  notes: string | null;
};

type Program = {
  id: string;
  name: string;
  description: string | null;
};

type Session = {
  id: string;
  program_id: string;
  program_name: string;
  starts_at: string;
  ends_at: string;
  location: string;
  capacity: number | null;
  confirmed_booking_count: number;
  status: string;
  notes: string | null;
  is_public: boolean;
  assigned_coach_id: string | null;
  assigned_coach_name: string | null;
};

type SessionUpdate = {
  status?: string;
  notes?: string;
  is_public?: boolean;
};

type SlotPreset = {
  id: string;
  label: string;
  start: string;
  end: string;
};

type Frequency = "once" | "daily" | "weekdays" | "selected";

type PlannedSlot = {
  key: string;
  date: string;
  preset: SlotPreset;
  starts_at: string;
  ends_at: string;
  conflicts: string[];
};

type EditDraft = {
  program_id: string;
  date: string;
  starts_at: string;
  ends_at: string;
  location: string;
  capacity: string;
  notes: string;
  is_public: boolean;
  acknowledge_booked_changes: boolean;
};

type Period = "week" | "month";

const PRESETS: SlotPreset[] = [
  { id: "morning-0530", label: "5:30–6:30 AM", start: "05:30", end: "06:30" },
  { id: "morning-0600", label: "6–7 AM", start: "06:00", end: "07:00" },
  { id: "morning-0700", label: "7–8 AM", start: "07:00", end: "08:00" },
  { id: "evening-1600", label: "4–5 PM", start: "16:00", end: "17:00" },
  { id: "evening-1700", label: "5–6 PM", start: "17:00", end: "18:00" },
  { id: "evening-1800", label: "6–7 PM", start: "18:00", end: "19:00" },
  { id: "evening-1900", label: "7–8 PM", start: "19:00", end: "20:00" },
  { id: "evening-2000", label: "8–9 PM", start: "20:00", end: "21:00" },
];

function formatSlotTime(value: string) {
  const [hours, minutes] = value.split(":").map(Number);
  const suffix = hours >= 12 ? "PM" : "AM";
  const hour = hours % 12 || 12;

  return `${hour}:${String(minutes).padStart(2, "0")} ${suffix}`;
}

const WEEKDAYS = [
  { label: "Mon", value: 1 },
  { label: "Tue", value: 2 },
  { label: "Wed", value: 3 },
  { label: "Thu", value: 4 },
  { label: "Fri", value: 5 },
  { label: "Sat", value: 6 },
  { label: "Sun", value: 0 },
];

const localDate = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Manila",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

const localTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  hour: "numeric",
  minute: "2-digit",
  hour12: true,
});

const sessionDateTime = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

const adminMonth = new Intl.DateTimeFormat("en-PH", {
  timeZone: "Asia/Manila",
  month: "long",
  year: "numeric",
});

const weekDayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function dateKey(date: Date) {
  return localDate.format(date);
}

function toPHTISO(day: string, time: string) {
  return new Date(`${day}T${time}:00+08:00`).toISOString();
}

function getPHTTimeInput(value: string) {
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Manila",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(new Date(value));
}

function overlaps(startA: string, endA: string, startB: string, endB: string) {
  return (
    Date.parse(startA) < Date.parse(endB) &&
    Date.parse(endA) > Date.parse(startB)
  );
}

function normalizeLocation(value: string) {
  return value.trim().toLocaleLowerCase();
}

function getOccurrenceDates(
  startDate: string,
  endDate: string,
  frequency: Frequency,
  selectedWeekdays: number[],
) {
  if (!startDate) return [];

  const lastDate = frequency === "once" ? startDate : endDate;

  if (!lastDate || startDate > lastDate) return [];

  const first = new Date(`${startDate}T00:00:00Z`);
  const last = new Date(`${lastDate}T00:00:00Z`);

  if (Number.isNaN(first.getTime()) || Number.isNaN(last.getTime())) {
    return [];
  }

  const result: string[] = [];

  for (
    const day = new Date(first);
    day <= last;
    day.setUTCDate(day.getUTCDate() + 1)
  ) {
    const weekday = day.getUTCDay();

    if (frequency === "weekdays" && (weekday === 0 || weekday === 6)) {
      continue;
    }

    if (frequency === "selected" && !selectedWeekdays.includes(weekday)) {
      continue;
    }

    result.push(day.toISOString().slice(0, 10));

    // Stop early for an excessive date range.
    if (result.length > 62) break;
  }

  return result;
}

function calendarDates(year: number, month: number) {
  const start = new Date(Date.UTC(year, month - 1, 1));

  start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));

  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(start);
    date.setUTCDate(start.getUTCDate() + index);

    const key = date.toISOString().slice(0, 10);

    return {
      key,
      day: date.getUTCDate(),
      inMonth: key.slice(0, 7) === `${year}-${String(month).padStart(2, "0")}`,
    };
  });
}

export default function AdminSchedule({
  adminFetch,
  onAuthorizationError,
}: {
  adminFetch: AdminFetch;
  onAuthorizationError: (cause: unknown) => void;
}) {
  const [availability, setAvailability] = useState<Availability[]>([]);
  const [programs, setPrograms] = useState<Program[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);

  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [period, setPeriod] = useState<Period>("week");
  const [anchor, setAnchor] = useState(() => dateKey(new Date()));
  const [selectedDate, setSelectedDate] = useState(() => dateKey(new Date()));

  const [scheduleView, setScheduleView] = useState<"list" | "calendar">(
    "calendar",
  );
  const [showPastSessions, setShowPastSessions] = useState(false);
  const [currentTimeMs, setCurrentTimeMs] = useState(() => Date.now());
  const [availabilityExpanded, setAvailabilityExpanded] = useState(false);

  // The single session-creation workflow.
  const [programID, setProgramID] = useState("");
  const [startDate, setStartDate] = useState(() => dateKey(new Date()));
  const [endDate, setEndDate] = useState(() => dateKey(new Date()));
  const [frequency, setFrequency] = useState<Frequency>("once");
  const [selectedWeekdays, setSelectedWeekdays] = useState<number[]>([
    1, 2, 3, 4, 5,
  ]);
  const [slotIDs, setSlotIDs] = useState<string[]>([]);
  const [customSlots, setCustomSlots] = useState<SlotPreset[]>([]);
  const [customStart, setCustomStart] = useState("09:00");
  const [customEnd, setCustomEnd] = useState("10:00");
  const [customSlotError, setCustomSlotError] = useState("");
  const [location, setLocation] = useState("");
  const [capacity, setCapacity] = useState("");
  const [notes, setNotes] = useState("");
  const [coachID, setCoachID] = useState("");
  const [isPublic, setIsPublic] = useState(false);

  const [sessionCreatorExpanded, setSessionCreatorExpanded] = useState(false);

  // Full edit state lives alongside the session-management UI.
  const [editing, setEditing] = useState<Session | null>(null);
  const [editDraft, setEditDraft] = useState<EditDraft | null>(null);

  const [creationNotice, setCreationNotice] = useState("");

  useEffect(() => {
    const updateTime = () => setCurrentTimeMs(Date.now());

    updateTime();

    const interval = window.setInterval(updateTime, 30_000);

    return () => window.clearInterval(interval);
  }, []);

  const readResponse = useCallback(
    async (response: Response, fallback: string) => {
      const body = await response.json().catch(() => ({}));

      if (!response.ok) {
        if (response.status === 401) {
          const cause = new Error(
            "Your admin session has expired. Please sign in again.",
          );
          onAuthorizationError(cause);
          throw cause;
        }

        if (response.status === 403) {
          const cause = new Error(
            "This Supabase account is not authorized to review club bookings.",
          );
          onAuthorizationError(cause);
          throw cause;
        }

        throw new Error(
          typeof body?.error === "string" ? body.error : fallback,
        );
      }

      return body;
    },
    [onAuthorizationError],
  );

  const loadSchedule = useCallback(async () => {
    setLoading(true);
    setError("");

    try {
      const [availabilityResponse, programsResponse, sessionsResponse] =
        await Promise.all([
          adminFetch("/api/backend/admin/coach-availability"),
          adminFetch("/api/backend/admin/training-programs"),
          adminFetch("/api/backend/admin/training-sessions"),
        ]);

      const [availabilityData, programsData, sessionsData] = await Promise.all([
        readResponse(
          availabilityResponse,
          "Could not load coach availability.",
        ),
        readResponse(programsResponse, "Could not load training programs."),
        readResponse(sessionsResponse, "Could not load training sessions."),
      ]);

      setAvailability(availabilityData.availability || []);
      setPrograms(programsData.programs || []);
      setSessions(sessionsData.sessions || []);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not load schedule management.",
      );
    } finally {
      setLoading(false);
    }
  }, [adminFetch, readResponse]);

  useEffect(() => {
    void loadSchedule();
  }, [loadSchedule]);

  const visibleSessions = useMemo(() => {
    if (showPastSessions) return sessions;

    return sessions.filter(
      (session) => Date.parse(session.ends_at) > currentTimeMs,
    );
  }, [sessions, showPastSessions, currentTimeMs]);

  const periodRange = useMemo(() => {
    const [year, monthNumber, day] = anchor.split("-").map(Number);
    const start = new Date(Date.UTC(year, monthNumber - 1, day));

    if (period === "week") {
      start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
    } else {
      start.setUTCDate(1);
    }

    const end = new Date(start);

    if (period === "week") {
      end.setUTCDate(end.getUTCDate() + 7);
    } else {
      end.setUTCMonth(end.getUTCMonth() + 1);
    }

    return {
      start,
      end,
      startKey: start.toISOString().slice(0, 10),
      endKey: end.toISOString().slice(0, 10),
    };
  }, [anchor, period]);

  const visibleAvailability = useMemo(
    () =>
      availability.filter((item) => {
        const day = dateKey(new Date(item.starts_at));
        return day >= periodRange.startKey && day < periodRange.endKey;
      }),
    [availability, periodRange],
  );

  const coaches = useMemo(() => {
    const latestByID = new Map<string, Availability>();

    for (const item of [...availability].sort((a, b) =>
      a.created_at.localeCompare(b.created_at),
    )) {
      latestByID.set(item.coach_id, item);
    }

    return [...latestByID.values()].sort((a, b) =>
      a.coach_name.localeCompare(b.coach_name),
    );
  }, [availability]);

  const listedSessions = useMemo(
    () =>
      [...visibleSessions].sort((a, b) => {
        const aUpcoming = Date.parse(a.ends_at) > currentTimeMs;
        const bUpcoming = Date.parse(b.ends_at) > currentTimeMs;

        if (aUpcoming !== bUpcoming) return aUpcoming ? -1 : 1;

        return a.starts_at.localeCompare(b.starts_at);
      }),
    [visibleSessions, currentTimeMs],
  );

  // Calendar sessions use the calendar's selected month, not the availability
  // week/month range.
  const [month, setMonth] = useState(() => {
    const today = dateKey(new Date());

    return {
      year: Number(today.slice(0, 4)),
      month: Number(today.slice(5, 7)),
    };
  });

  const calendarSessions = visibleSessions.filter((session) =>
    dateKey(new Date(session.starts_at)).startsWith(
      `${month.year}-${String(month.month).padStart(2, "0")}`,
    ),
  );

  const monthAvailability = availability.filter((item) =>
    dateKey(new Date(item.starts_at)).startsWith(
      `${month.year}-${String(month.month).padStart(2, "0")}`,
    ),
  );

  const selectedSessions = calendarSessions.filter(
    (session) => dateKey(new Date(session.starts_at)) === selectedDate,
  );

  const selectedAvailability = monthAvailability.filter(
    (item) => dateKey(new Date(item.starts_at)) === selectedDate,
  );

  const occurrenceDates = useMemo(
    () => getOccurrenceDates(startDate, endDate, frequency, selectedWeekdays),
    [startDate, endDate, frequency, selectedWeekdays],
  );

  const rangeError = useMemo(() => {
    if (!startDate) return "Choose the first date.";

    if (frequency !== "once" && !endDate) {
      return "Choose the last date.";
    }

    if (frequency !== "once" && startDate > endDate) {
      return "The last date must be on or after the first date.";
    }

    if (frequency === "selected" && selectedWeekdays.length === 0) {
      return "Select at least one weekday.";
    }

    if (occurrenceDates.length === 0) {
      return "No dates match the selected schedule pattern.";
    }

    if (occurrenceDates.length > 62) {
      return "Choose no more than 62 matching dates.";
    }

    return "";
  }, [startDate, endDate, frequency, selectedWeekdays, occurrenceDates]);

  const allSlots = useMemo(() => [...PRESETS, ...customSlots], [customSlots]);

  const capacityInvalid =
    capacity.trim() !== "" &&
    (!Number.isSafeInteger(Number(capacity)) || Number(capacity) <= 0);

  const planned = useMemo<PlannedSlot[]>(() => {
    if (rangeError || slotIDs.length === 0) return [];

    const pickedSlots = allSlots.filter((slot) => slotIDs.includes(slot.id));

    const raw: PlannedSlot[] = occurrenceDates.flatMap((date) =>
      pickedSlots.map((preset) => ({
        key: `${date}:${preset.id}`,
        date,
        preset,
        starts_at: toPHTISO(date, preset.start),
        ends_at: toPHTISO(date, preset.end),
        conflicts: [],
      })),
    );

    return raw.map((item) => {
      const found: string[] = [];

      for (const current of sessions) {
        if (current.status === "cancelled") continue;

        const hasOverlap = overlaps(
          item.starts_at,
          item.ends_at,
          current.starts_at,
          current.ends_at,
        );

        if (!hasOverlap) continue;

        if (
          normalizeLocation(current.location) === normalizeLocation(location)
        ) {
          found.push(
            `Venue conflict: ${current.program_name} · ${current.location} · ${localTime.format(
              new Date(current.starts_at),
            )}–${localTime.format(new Date(current.ends_at))}`,
          );
        }

        if (coachID && current.assigned_coach_id === coachID) {
          const coachName =
            coaches.find((coach) => coach.coach_id === coachID)?.coach_name ||
            "selected coach";

          found.push(
            `Coach ${coachName} is already assigned to ${current.program_name} at ${localTime.format(
              new Date(current.starts_at),
            )}–${localTime.format(new Date(current.ends_at))}`,
          );
        }
      }

      // Check overlaps within the batch itself. In particular, 5:30–6:30 AM
      // and 6–7 AM overlap for 30 minutes.
      for (const other of raw) {
        if (other.key === item.key || other.date !== item.date) continue;

        if (
          overlaps(item.starts_at, item.ends_at, other.starts_at, other.ends_at)
        ) {
          found.push(`Selected slots overlap: ${other.preset.label}`);
        }
      }

      return { ...item, conflicts: [...new Set(found)] };
    });
  }, [
    rangeError,
    occurrenceDates,
    slotIDs,
    sessions,
    location,
    coachID,
    coaches,
  ]);

  const conflictCount = planned.filter(
    (item) => item.conflicts.length > 0,
  ).length;
  const groupedPlanned = useMemo(() => {
    const groups = new Map<string, PlannedSlot[]>();

    for (const item of planned) {
      const current = groups.get(item.date) ?? [];
      current.push(item);
      groups.set(item.date, current);
    }

    return [...groups.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, slots]) => ({
        date,
        slots: slots.sort((a, b) => a.starts_at.localeCompare(b.starts_at)),
      }));
  }, [planned]);

  function addCustomSlot() {
    setCustomSlotError("");

    if (!customStart || !customEnd || customStart >= customEnd) {
      setCustomSlotError("The end time must be after the start time.");
      return;
    }

    if (
      allSlots.some(
        (slot) => slot.start === customStart && slot.end === customEnd,
      )
    ) {
      setCustomSlotError("That time slot already exists.");
      return;
    }

    const id = `custom-${customStart.replace(":", "")}-${customEnd.replace(":", "")}`;

    const slot: SlotPreset = {
      id,
      start: customStart,
      end: customEnd,
      label: `${formatSlotTime(customStart)}–${formatSlotTime(customEnd)}`,
    };

    setCustomSlots((current) => [...current, slot]);
    setSlotIDs((current) => [...current, id]);
  }

  function removeCustomSlot(id: string) {
    setCustomSlots((current) => current.filter((slot) => slot.id !== id));
    setSlotIDs((current) => current.filter((slotID) => slotID !== id));
    setCustomSlotError("");
  }

  async function createSessions(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setNotice("");
    setCreationNotice("");

    if (!programID) {
      setError("Select a training program.");
      return;
    }

    if (!location.trim()) {
      setError("Enter a venue or court.");
      return;
    }

    if (rangeError) {
      setError(rangeError);
      return;
    }

    if (slotIDs.length === 0) {
      setError("Select at least one time slot.");
      return;
    }

    if (capacityInvalid) {
      setError(
        "Capacity must be a positive whole number, or left blank for unlimited capacity.",
      );
      return;
    }

    if (planned.length === 0 || planned.length > 500) {
      setError("A batch must contain between 1 and 500 sessions.");
      return;
    }

    if (conflictCount > 0) {
      setError(
        "Resolve every venue or coach conflict before creating this schedule.",
      );
      return;
    }

    setBusy(true);

    try {
      // One API request means the backend can commit the complete batch
      // atomically or reject the whole batch.
      const response = await adminFetch(
        "/api/backend/admin/training-sessions/batch",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            sessions: planned.map((item) => ({
              program_id: programID,
              starts_at: item.starts_at,
              ends_at: item.ends_at,
              location: location.trim(),
              capacity: capacity.trim() === "" ? null : Number(capacity),
              notes: notes.trim(),
              is_public: isPublic,
              coach_id: coachID || null,
            })),
          }),
        },
      );

      const result = await readResponse(
        response,
        "Could not create the selected schedule.",
      );

      setSlotIDs([]);
      setCreationNotice(
        `${result.created ?? planned.length} training session(s) created successfully. ${
          isPublic
            ? "They are published on the public schedule."
            : "They remain hidden until you publish them."
        }`,
      );

      await loadSchedule();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not create the selected schedule.",
      );
    } finally {
      setBusy(false);
    }
  }

  function beginEdit(session: Session) {
    setEditing(session);

    setEditDraft({
      program_id: session.program_id,
      date: dateKey(new Date(session.starts_at)),
      starts_at: getPHTTimeInput(session.starts_at),
      ends_at: getPHTTimeInput(session.ends_at),
      location: session.location,
      capacity: session.capacity === null ? "" : String(session.capacity),
      notes: session.notes || "",
      is_public: session.is_public,
      acknowledge_booked_changes: false,
    });

    setError("");
    setNotice("");
  }

  const editHasBookedScheduleChanges = Boolean(
    editing &&
    editDraft &&
    editing.confirmed_booking_count > 0 &&
    (editing.program_id !== editDraft.program_id ||
      !editDraft.date ||
      !editDraft.starts_at ||
      !editDraft.ends_at ||
      Date.parse(editing.starts_at) !==
        Date.parse(toPHTISO(editDraft.date, editDraft.starts_at)) ||
      Date.parse(editing.ends_at) !==
        Date.parse(toPHTISO(editDraft.date, editDraft.ends_at)) ||
      normalizeLocation(editing.location) !==
        normalizeLocation(editDraft.location)),
  );

  function changeEditDraft<K extends keyof EditDraft>(
    field: K,
    value: EditDraft[K],
    scheduleField = false,
  ) {
    setEditDraft((current) =>
      current
        ? {
            ...current,
            [field]: value,
            ...(scheduleField ? { acknowledge_booked_changes: false } : {}),
          }
        : current,
    );
  }

  async function saveEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (!editing || !editDraft) return;

    if (!editDraft.program_id) {
      setError("Select a training program.");
      return;
    }

    if (!editDraft.date || !editDraft.starts_at || !editDraft.ends_at) {
      setError("Choose the session date, start time, and end time.");
      return;
    }

    if (!editDraft.location.trim()) {
      setError("Enter a venue.");
      return;
    }

    const startsAt = toPHTISO(editDraft.date, editDraft.starts_at);
    const endsAt = toPHTISO(editDraft.date, editDraft.ends_at);

    if (Date.parse(startsAt) >= Date.parse(endsAt)) {
      setError("The session end time must be after its start time.");
      return;
    }

    if (
      editDraft.capacity.trim() !== "" &&
      (!Number.isSafeInteger(Number(editDraft.capacity)) ||
        Number(editDraft.capacity) <= 0)
    ) {
      setError(
        "Capacity must be a positive whole number, or left blank for unlimited capacity.",
      );
      return;
    }

    const nextCapacity =
      editDraft.capacity.trim() === "" ? null : Number(editDraft.capacity);

    if (
      nextCapacity !== null &&
      nextCapacity < editing.confirmed_booking_count
    ) {
      setError(
        `Capacity cannot be lower than the ${editing.confirmed_booking_count} confirmed booking(s).`,
      );
      return;
    }

    if (editHasBookedScheduleChanges && !editDraft.acknowledge_booked_changes) {
      setError(
        "Acknowledge the booked-session warning before saving these changes.",
      );
      return;
    }

    setBusy(true);
    setError("");
    setNotice("");

    try {
      const response = await adminFetch(
        `/api/backend/admin/training-sessions/${editing.id}`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            program_id: editDraft.program_id,
            starts_at: startsAt,
            ends_at: endsAt,
            location: editDraft.location.trim(),
            capacity: nextCapacity,
            notes: editDraft.notes.trim(),
            is_public: editDraft.is_public,
            acknowledge_booked_changes: editDraft.acknowledge_booked_changes,
          }),
        },
      );

      await readResponse(response, "Could not edit the training session.");

      setEditing(null);
      setEditDraft(null);
      setNotice(
        "Session changes saved. No notification was automatically sent to booked players.",
      );

      await loadSchedule();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not edit the training session.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function updateSession(id: string, update: SessionUpdate) {
    setBusy(true);
    setError("");
    setNotice("");

    try {
      const response = await adminFetch(
        `/api/backend/admin/training-sessions/${id}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(update),
        },
      );

      await readResponse(response, "Could not update the training session.");

      setNotice("Training session updated.");
      await loadSchedule();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not update the training session.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function assignCoach(sessionID: string, coachIDValue: string | null) {
    setBusy(true);
    setError("");
    setNotice("");

    try {
      const response = await adminFetch(
        `/api/backend/admin/training-sessions/${sessionID}/coach`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ coach_id: coachIDValue }),
        },
      );

      await readResponse(
        response,
        "Could not assign a coach to the training session.",
      );

      setNotice(
        coachIDValue
          ? "Coach assigned to the training session."
          : "Coach assignment removed.",
      );

      await loadSchedule();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not assign a coach to the training session.",
      );
    } finally {
      setBusy(false);
    }
  }

  function moveAvailabilityPeriod(direction: number) {
    const next = new Date(periodRange.start);

    if (period === "week") {
      next.setUTCDate(next.getUTCDate() + 7 * direction);
    } else {
      next.setUTCMonth(next.getUTCMonth() + direction);
    }

    setAnchor(next.toISOString().slice(0, 10));
  }

  function moveScheduleMonth(direction: number) {
    const next = new Date(Date.UTC(month.year, month.month - 1 + direction, 1));

    const nextMonth = {
      year: next.getUTCFullYear(),
      month: next.getUTCMonth() + 1,
    };

    setMonth(nextMonth);
    setSelectedDate(
      `${nextMonth.year}-${String(nextMonth.month).padStart(2, "0")}-01`,
    );
  }

  function editForm(session: Session) {
    if (editing?.id !== session.id || !editDraft) return null;

    return (
      <form className={styles.editForm} onSubmit={saveEdit}>
        <div className={styles.editFormHead}>
          <div>
            <p className="section-kicker">EDIT SESSION</p>
            <h3>{session.program_name}</h3>
          </div>

          <button
            type="button"
            className="admin-button secondary"
            disabled={busy}
            onClick={() => {
              setEditing(null);
              setEditDraft(null);
              setError("");
            }}
          >
            Close
          </button>
        </div>

        <div className={styles.fields}>
          <label>
            Training program
            <select
              required
              value={editDraft.program_id}
              onChange={(event) =>
                changeEditDraft("program_id", event.target.value, true)
              }
            >
              {programs.map((program) => (
                <option key={program.id} value={program.id}>
                  {program.name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Date
            <input
              required
              type="date"
              value={editDraft.date}
              onChange={(event) =>
                changeEditDraft("date", event.target.value, true)
              }
            />
          </label>

          <label>
            Start time
            <input
              required
              type="time"
              value={editDraft.starts_at}
              onChange={(event) =>
                changeEditDraft("starts_at", event.target.value, true)
              }
            />
          </label>

          <label>
            End time
            <input
              required
              type="time"
              value={editDraft.ends_at}
              onChange={(event) =>
                changeEditDraft("ends_at", event.target.value, true)
              }
            />
          </label>

          <label>
            Venue / court
            <input
              required
              maxLength={200}
              value={editDraft.location}
              onChange={(event) =>
                changeEditDraft("location", event.target.value, true)
              }
            />
          </label>

          <label>
            Capacity
            <input
              type="number"
              min="1"
              step="1"
              value={editDraft.capacity}
              placeholder="Unlimited"
              onChange={(event) =>
                changeEditDraft("capacity", event.target.value)
              }
            />
          </label>

          <label>
            Admin note
            <input
              maxLength={2000}
              value={editDraft.notes}
              onChange={(event) => changeEditDraft("notes", event.target.value)}
            />
          </label>
        </div>

        <label className={styles.publish}>
          <input
            type="checkbox"
            checked={editDraft.is_public}
            onChange={(event) =>
              changeEditDraft("is_public", event.target.checked)
            }
          />
          Published on the public schedule
        </label>

        {editHasBookedScheduleChanges && (
          <div className={styles.warningBox}>
            <strong>
              This session has {session.confirmed_booking_count} confirmed
              booking(s).
            </strong>
            <p>
              Changing its program, date, time, or venue will not automatically
              notify those players. Contact them separately after saving.
            </p>
            <label>
              <input
                type="checkbox"
                checked={editDraft.acknowledge_booked_changes}
                onChange={(event) =>
                  changeEditDraft(
                    "acknowledge_booked_changes",
                    event.target.checked,
                  )
                }
              />
              I understand and will contact affected players.
            </label>
          </div>
        )}

        <button
          type="submit"
          className="admin-button"
          disabled={
            busy ||
            (editHasBookedScheduleChanges &&
              !editDraft.acknowledge_booked_changes)
          }
        >
          {busy ? "Saving…" : "Save session changes"}
        </button>
      </form>
    );
  }

  return (
    <div className="schedule-management">
      {error && (
        <p className="admin-state form-error" role="alert">
          {error}
        </p>
      )}

      {notice && (
        <p className="admin-state form-success" role="status">
          {notice}
        </p>
      )}

      {/* Coach availability remains a separate planning reference. */}
      <section className="availability-panel">
        <div className="schedule-section-heading">
          <div>
            <p className="section-kicker">COACH INPUT</p>
            <h2>Submitted availability</h2>
          </div>

          <div className="schedule-view-switch">
            <button
              className="admin-button secondary"
              type="button"
              aria-expanded={availabilityExpanded}
              aria-controls="coach-availability-content"
              onClick={() => setAvailabilityExpanded((current) => !current)}
            >
              {availabilityExpanded ? "Hide availability" : "Show availability"}
            </button>

            <button
              className="admin-button secondary"
              type="button"
              disabled={loading || busy}
              onClick={() => void loadSchedule()}
            >
              {loading ? "Loading…" : "Refresh"}
            </button>
          </div>
        </div>

        {availabilityExpanded && (
          <div id="coach-availability-content">
            <p className="admin-help">
              Availability is a planning reference only. It does not create or
              publish a customer booking session.
            </p>

            <div className="availability-toolbar">
              <div
                className="schedule-tabs"
                role="group"
                aria-label="Availability review period"
              >
                <button
                  type="button"
                  className={period === "week" ? "active" : ""}
                  aria-pressed={period === "week"}
                  onClick={() => setPeriod("week")}
                >
                  Week
                </button>

                <button
                  type="button"
                  className={period === "month" ? "active" : ""}
                  aria-pressed={period === "month"}
                  onClick={() => setPeriod("month")}
                >
                  Month
                </button>
              </div>

              <div className="period-navigation">
                <button
                  className="admin-button secondary"
                  type="button"
                  aria-label="Previous period"
                  onClick={() => moveAvailabilityPeriod(-1)}
                >
                  ‹
                </button>

                <strong>
                  {periodRange.start.toLocaleDateString("en-PH", {
                    timeZone: "UTC",
                    month: "short",
                    day: "numeric",
                  })}{" "}
                  –{" "}
                  {new Date(
                    periodRange.end.getTime() - 86400000,
                  ).toLocaleDateString("en-PH", {
                    timeZone: "UTC",
                    month: "short",
                    day: "numeric",
                    year: "numeric",
                  })}
                </strong>

                <button
                  className="admin-button secondary"
                  type="button"
                  aria-label="Next period"
                  onClick={() => moveAvailabilityPeriod(1)}
                >
                  ›
                </button>
              </div>
            </div>

            {visibleAvailability.length === 0 && !loading ? (
              <p className="admin-state">
                {availability.length === 0
                  ? "No coach availability has been submitted."
                  : `No availability submitted for this ${period}.`}
              </p>
            ) : (
              <div className="availability-list">
                {visibleAvailability.map((item) => (
                  <article className="availability-card" key={item.id}>
                    <div>
                      <strong>{item.coach_name}</strong>
                      <p>
                        {sessionDateTime.format(new Date(item.starts_at))} –{" "}
                        {localTime.format(new Date(item.ends_at))}
                      </p>
                      {item.notes && <small>{item.notes}</small>}
                    </div>
                  </article>
                ))}
              </div>
            )}
          </div>
        )}
      </section>

      {/* One creation workflow for actual customer sessions. */}
      <section
        className={`availability-panel ${styles.root}`}
        id="session-creator"
      >
        <div className="schedule-section-heading">
          <div>
            <p className="section-kicker">ACTUAL CUSTOMER SESSIONS</p>
            <h2>Create training sessions</h2>
            <p className="admin-help">
              Create one day or a recurring schedule by selecting time slots.
              Every slot becomes a real training session.
            </p>
          </div>

          <div className="schedule-view-switch">
            <button
              className="admin-button secondary"
              type="button"
              aria-expanded={sessionCreatorExpanded}
              aria-controls="session-creation-content"
              onClick={() => setSessionCreatorExpanded((current) => !current)}
            >
              {sessionCreatorExpanded
                ? "Hide session creator"
                : "Show session creator"}
            </button>

            <button
              className="admin-button secondary"
              type="button"
              disabled={loading || busy}
              onClick={() => void loadSchedule()}
            >
              {loading ? "Loading…" : "Refresh schedule data"}
            </button>
          </div>
        </div>
        {creationNotice && (
          <p
            className="admin-state form-success"
            role="status"
            aria-live="polite"
          >
            {creationNotice}
          </p>
        )}
        {sessionCreatorExpanded && (
          <div id="session-creation-content">
            {programs.length === 0 && !loading ? (
              <p className="admin-state">
                No active training programs are available.
              </p>
            ) : (
              <form className={styles.form} onSubmit={createSessions}>
                <div className={styles.fields}>
                  <label>
                    Training program
                    <select
                      required
                      value={programID}
                      onChange={(event) => setProgramID(event.target.value)}
                    >
                      <option value="">Select a program</option>
                      {programs.map((program) => (
                        <option key={program.id} value={program.id}>
                          {program.name}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label>
                    Venue / court
                    <input
                      required
                      maxLength={200}
                      value={location}
                      onChange={(event) => setLocation(event.target.value)}
                      placeholder="e.g. MMSU Court 1"
                    />
                  </label>

                  <label>
                    First date
                    <input
                      required
                      type="date"
                      value={startDate}
                      onChange={(event) => setStartDate(event.target.value)}
                    />
                  </label>

                  <label>
                    Last date
                    <input
                      type="date"
                      required={frequency !== "once"}
                      disabled={frequency === "once"}
                      value={endDate}
                      onChange={(event) => setEndDate(event.target.value)}
                    />
                  </label>

                  <label>
                    Repeat pattern
                    <select
                      value={frequency}
                      onChange={(event) =>
                        setFrequency(event.target.value as Frequency)
                      }
                    >
                      <option value="once">One day only</option>
                      <option value="daily">Every day</option>
                      <option value="weekdays">Weekdays (Mon–Fri)</option>
                      <option value="selected">Selected weekdays</option>
                    </select>
                  </label>

                  <label>
                    Capacity per slot
                    <input
                      type="number"
                      min="1"
                      step="1"
                      value={capacity}
                      onChange={(event) => setCapacity(event.target.value)}
                      placeholder="Unlimited"
                    />
                  </label>

                  <label>
                    Assign coach (optional)
                    <select
                      value={coachID}
                      onChange={(event) => setCoachID(event.target.value)}
                    >
                      <option value="">Leave unassigned</option>
                      {coaches.map((coach) => (
                        <option key={coach.coach_id} value={coach.coach_id}>
                          {coach.coach_name}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label>
                    Admin note (optional)
                    <input
                      maxLength={2000}
                      value={notes}
                      onChange={(event) => setNotes(event.target.value)}
                    />
                  </label>
                </div>

                {frequency === "selected" && (
                  <fieldset className={styles.weekdays}>
                    <legend>Repeat on</legend>

                    {WEEKDAYS.map((day) => (
                      <label key={day.value} className="weekday-choice">
                        <input
                          type="checkbox"
                          checked={selectedWeekdays.includes(day.value)}
                          onChange={(event) =>
                            setSelectedWeekdays((current) =>
                              event.target.checked
                                ? [...current, day.value]
                                : current.filter(
                                    (value) => value !== day.value,
                                  ),
                            )
                          }
                        />
                        {day.label}
                      </label>
                    ))}
                  </fieldset>
                )}

                <fieldset className={styles.slotPicker}>
                  <legend>Choose time slots</legend>
                  <p>
                    Each selected time becomes a separate session on every
                    selected date.
                  </p>

                  <div className={styles.slotGrid}>
                    {allSlots.map((slot) => {
                      const isCustom = customSlots.some(
                        (customSlot) => customSlot.id === slot.id,
                      );

                      return (
                        <div className={styles.slotOption} key={slot.id}>
                          <label
                            className={
                              slotIDs.includes(slot.id)
                                ? styles.slotSelected
                                : ""
                            }
                          >
                            <input
                              type="checkbox"
                              checked={slotIDs.includes(slot.id)}
                              onChange={(event) =>
                                setSlotIDs((current) =>
                                  event.target.checked
                                    ? [...current, slot.id]
                                    : current.filter((id) => id !== slot.id),
                                )
                              }
                            />

                            <span>
                              {slot.label}
                              {isCustom && (
                                <small className={styles.customBadge}>
                                  Custom
                                </small>
                              )}
                            </span>
                          </label>

                          {isCustom && (
                            <button
                              type="button"
                              className={styles.removeCustomSlot}
                              onClick={() => removeCustomSlot(slot.id)}
                              aria-label={`Remove custom slot ${slot.label}`}
                            >
                              ×
                            </button>
                          )}
                        </div>
                      );
                    })}
                  </div>

                  <div className={styles.customSlot}>
                    <h4>Add a custom time slot</h4>

                    <div className={styles.customTimeFields}>
                      <label>
                        Start time
                        <input
                          type="time"
                          required
                          value={customStart}
                          onChange={(event) =>
                            setCustomStart(event.target.value)
                          }
                        />
                      </label>

                      <label>
                        End time
                        <input
                          type="time"
                          required
                          value={customEnd}
                          onChange={(event) => setCustomEnd(event.target.value)}
                        />
                      </label>

                      <button
                        type="button"
                        className="admin-button secondary"
                        onClick={addCustomSlot}
                      >
                        Add custom slot
                      </button>
                    </div>

                    {customSlotError && (
                      <p className="form-error" role="alert">
                        {customSlotError}
                      </p>
                    )}

                    <p className="admin-help">
                      Custom slots are selectable like standard slots and can be
                      used across your selected dates. Overlapping selections
                      will be flagged.
                    </p>
                  </div>

                  {slotIDs.includes("morning-0530") &&
                    slotIDs.includes("morning-0600") && (
                      <p className={styles.warning}>
                        5:30–6:30 AM overlaps 6–7 AM by 30 minutes. Those slots
                        cannot both be created at the same venue.
                      </p>
                    )}
                </fieldset>

                <label className={styles.publish}>
                  <input
                    type="checkbox"
                    checked={isPublic}
                    onChange={(event) => setIsPublic(event.target.checked)}
                  />
                  Publish created sessions immediately
                </label>

                {rangeError && (
                  <p className="form-error" role="alert">
                    {rangeError}
                  </p>
                )}

                {capacityInvalid && (
                  <p className="form-error" role="alert">
                    Capacity must be a positive whole number or blank for
                    unlimited capacity.
                  </p>
                )}

                <section
                  className={styles.reviewPanel}
                  aria-labelledby="schedule-review-title"
                >
                  <div className={styles.reviewHeading}>
                    <div>
                      <p className="section-kicker">REVIEW BEFORE CREATING</p>
                      <h3 id="schedule-review-title">Schedule preview</h3>
                    </div>

                    <div className={styles.reviewStats}>
                      <span>
                        {planned.length} slot{planned.length === 1 ? "" : "s"}
                      </span>
                      <span
                        className={
                          conflictCount
                            ? styles.conflictBadge
                            : styles.clearBadge
                        }
                      >
                        {conflictCount} conflicts
                      </span>
                    </div>
                  </div>

                  {groupedPlanned.length === 0 ? (
                    <p className={styles.reviewEmpty}>
                      Choose one or more time slots above to preview your
                      schedule.
                    </p>
                  ) : (
                    <div className={styles.reviewDates}>
                      {groupedPlanned.map(({ date, slots }) => (
                        <article className={styles.reviewDateCard} key={date}>
                          <div className={styles.reviewDateHeading}>
                            <strong>
                              {new Intl.DateTimeFormat("en-PH", {
                                timeZone: "Asia/Manila",
                                weekday: "short",
                                month: "short",
                                day: "numeric",
                                year: "numeric",
                              }).format(new Date(`${date}T12:00:00+08:00`))}
                            </strong>
                            <span>
                              {slots.length} session
                              {slots.length === 1 ? "" : "s"}
                            </span>
                          </div>

                          <div className={styles.reviewTimeList}>
                            {slots.map((slot) => (
                              <div
                                className={`${styles.reviewTimeRow} ${
                                  slot.conflicts.length
                                    ? styles.reviewTimeConflict
                                    : ""
                                }`}
                                key={slot.key}
                              >
                                <span
                                  className={styles.reviewTimeDot}
                                  aria-hidden="true"
                                />
                                <span className={styles.reviewTimeName}>
                                  {slot.preset.label}
                                </span>

                                {slot.conflicts.length > 0 && (
                                  <span className={styles.reviewConflictText}>
                                    {slot.conflicts.join("; ")}
                                  </span>
                                )}
                              </div>
                            ))}
                          </div>
                        </article>
                      ))}
                    </div>
                  )}
                </section>

                <button
                  className="admin-button"
                  type="submit"
                  disabled={
                    busy ||
                    loading ||
                    !programID ||
                    !location.trim() ||
                    slotIDs.length === 0 ||
                    Boolean(rangeError) ||
                    capacityInvalid ||
                    planned.length === 0 ||
                    planned.length > 500 ||
                    conflictCount > 0
                  }
                >
                  {busy
                    ? "Creating all sessions…"
                    : `Create ${planned.length} session${
                        planned.length === 1 ? "" : "s"
                      } atomically`}
                </button>

                <p className="admin-help">
                  One request creates the complete batch. The database performs
                  its own conflict checks; if any session conflicts, the batch
                  should be rejected rather than partially created.
                </p>
              </form>
            )}
          </div>
        )}
      </section>

      {/* Existing sessions, full edits, and publication controls live here. */}
      <section className="availability-panel">
        <div className="schedule-section-heading">
          <div>
            <p className="section-kicker">EXCEPTIONS &amp; PUBLICATION</p>
            <h2>Training sessions</h2>
            <p className="admin-help">
              Completed sessions are hidden by default. You can show past
              sessions to review history.
            </p>
          </div>

          <div className="schedule-view-switch">
            <button
              className="admin-button secondary"
              type="button"
              aria-pressed={showPastSessions}
              onClick={() => setShowPastSessions((current) => !current)}
            >
              {showPastSessions ? "Hide past sessions" : "Show past sessions"}
            </button>

            <div
              className="schedule-view-switch"
              role="group"
              aria-label="Admin schedule view"
            >
              <button
                type="button"
                aria-pressed={scheduleView === "list"}
                className={scheduleView === "list" ? "active" : ""}
                onClick={() => setScheduleView("list")}
              >
                List
              </button>

              <button
                type="button"
                aria-pressed={scheduleView === "calendar"}
                className={scheduleView === "calendar" ? "active" : ""}
                onClick={() => setScheduleView("calendar")}
              >
                Calendar
              </button>
            </div>
          </div>
        </div>

        {sessions.length === 0 && !loading ? (
          <p className="admin-state">No training sessions have been created.</p>
        ) : scheduleView === "calendar" ? (
          <>
            <div className="calendar-toolbar">
              <button
                type="button"
                aria-label="Previous month"
                onClick={() => moveScheduleMonth(-1)}
              >
                ‹
              </button>

              <h3>
                {adminMonth.format(
                  new Date(Date.UTC(month.year, month.month - 1, 1)),
                )}
              </h3>

              <button
                type="button"
                aria-label="Next month"
                onClick={() => moveScheduleMonth(1)}
              >
                ›
              </button>
            </div>

            <div
              className="training-calendar admin-calendar"
              role="grid"
              aria-label={`Training and coach availability for ${adminMonth.format(
                new Date(Date.UTC(month.year, month.month - 1, 1)),
              )}`}
            >
              {weekDayLabels.map((day) => (
                <div className="calendar-weekday" role="columnheader" key={day}>
                  {day}
                </div>
              ))}

              {calendarDates(month.year, month.month).map((day) => {
                const daySessions = calendarSessions.filter(
                  (session) => dateKey(new Date(session.starts_at)) === day.key,
                );

                const dayAvailability = monthAvailability.filter(
                  (item) => dateKey(new Date(item.starts_at)) === day.key,
                );

                return (
                  <button
                    className={`calendar-day${
                      day.inMonth ? "" : " outside-month"
                    }${selectedDate === day.key ? " selected" : ""}${
                      daySessions.length || dayAvailability.length
                        ? " has-sessions"
                        : ""
                    }`}
                    type="button"
                    role="gridcell"
                    aria-selected={selectedDate === day.key}
                    aria-label={`${day.key}, ${daySessions.length} sessions, ${dayAvailability.length} coach availability submissions`}
                    key={day.key}
                    onClick={() => {
                      setSelectedDate(day.key);

                      window.requestAnimationFrame(() => {
                        document
                          .getElementById("admin-selected-date-agenda")
                          ?.scrollIntoView({
                            behavior: "smooth",
                            block: "start",
                          });
                      });
                    }}
                  >
                    <span className="calendar-day-number">{day.day}</span>

                    {(daySessions.length > 0 || dayAvailability.length > 0) && (
                      <span className="calendar-day-count">
                        {daySessions.length
                          ? `${daySessions.length} sessions`
                          : ""}
                        {daySessions.length && dayAvailability.length
                          ? " · "
                          : ""}
                        {dayAvailability.length
                          ? `${dayAvailability.length} available`
                          : ""}
                      </span>
                    )}

                    <span className="calendar-day-dots" aria-hidden="true">
                      {daySessions.slice(0, 2).map((session) => (
                        <i
                          className={`dot-${
                            session.status === "cancelled"
                              ? "closed"
                              : session.is_public
                                ? "open"
                                : "full"
                          }`}
                          key={`session-${session.id}`}
                        />
                      ))}

                      {dayAvailability.slice(0, 2).map((item) => (
                        <i
                          className="dot-availability"
                          key={`availability-${item.id}`}
                        />
                      ))}
                    </span>
                  </button>
                );
              })}
            </div>

            <div
              className="admin-day-agenda"
              id="admin-selected-date-agenda"
              style={{ scrollMarginTop: "24px" }}
            >
              <h3>
                {new Intl.DateTimeFormat("en-PH", {
                  timeZone: "Asia/Manila",
                  weekday: "long",
                  month: "long",
                  day: "numeric",
                  year: "numeric",
                }).format(new Date(`${selectedDate}T12:00:00+08:00`))}
              </h3>

              {selectedSessions.length === 0 &&
              selectedAvailability.length === 0 ? (
                <p className="admin-state">
                  No sessions or availability on this date.
                </p>
              ) : (
                <>
                  {selectedAvailability.map((item) => (
                    <article
                      className="availability-card agenda-card"
                      key={item.id}
                    >
                      <div>
                        <strong>Coach availability · {item.coach_name}</strong>
                        <p>
                          {sessionDateTime.format(new Date(item.starts_at))} –{" "}
                          {localTime.format(new Date(item.ends_at))}
                        </p>
                        {item.notes && <small>{item.notes}</small>}
                      </div>
                    </article>
                  ))}

                  {selectedSessions.map((session) => (
                    <article
                      className="availability-card agenda-card"
                      key={session.id}
                    >
                      <div>
                        <strong>{session.program_name}</strong>
                        <p>
                          {sessionDateTime.format(new Date(session.starts_at))}{" "}
                          – {localTime.format(new Date(session.ends_at))} ·{" "}
                          {session.location}
                        </p>
                        <small>
                          {session.status} ·{" "}
                          {session.is_public ? "published" : "hidden"} ·{" "}
                          {session.confirmed_booking_count} confirmed
                          {session.capacity === null
                            ? ""
                            : ` of ${session.capacity}`}{" "}
                          ·{" "}
                          {session.assigned_coach_name
                            ? `Coach ${session.assigned_coach_name}`
                            : "Unassigned"}
                        </small>
                      </div>

                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={busy}
                        onClick={() => beginEdit(session)}
                      >
                        Edit session
                      </button>

                      {session.status !== "cancelled" && (
                        <button
                          className="admin-button secondary"
                          type="button"
                          disabled={busy}
                          onClick={() =>
                            void updateSession(session.id, {
                              status:
                                session.status === "open" ? "closed" : "open",
                            })
                          }
                        >
                          {session.status === "open"
                            ? "Close booking"
                            : "Reopen booking"}
                        </button>
                      )}

                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={busy}
                        onClick={() =>
                          void updateSession(session.id, {
                            is_public: !session.is_public,
                          })
                        }
                      >
                        {session.is_public ? "Unpublish" : "Publish"}
                      </button>

                      {session.status !== "cancelled" && (
                        <button
                          className="admin-button danger"
                          type="button"
                          disabled={busy}
                          onClick={() => {
                            if (
                              window.confirm(
                                `Cancel ${session.program_name}? It will no longer accept booking requests.`,
                              )
                            ) {
                              void updateSession(session.id, {
                                status: "cancelled",
                              });
                            }
                          }}
                        >
                          Cancel session
                        </button>
                      )}
                    </article>
                  ))}

                  {selectedSessions.map((session) => editForm(session))}
                </>
              )}
            </div>
          </>
        ) : listedSessions.length === 0 ? (
          <p className="admin-state">
            {showPastSessions
              ? "No training sessions are available to display."
              : "No upcoming or in-progress training sessions. Use “Show past sessions” to review completed sessions."}
          </p>
        ) : (
          <div className="schedule-session-list">
            {listedSessions.map((session) => (
              <div key={session.id}>
                <article className="schedule-session-card">
                  <div className="schedule-session-summary">
                    <div>
                      <h3>{session.program_name}</h3>

                      <p>
                        {sessionDateTime.format(new Date(session.starts_at))} –{" "}
                        {localTime.format(new Date(session.ends_at))} ·{" "}
                        {session.location}
                      </p>

                      <div className="session-status-row">
                        <span
                          className={`session-status session-status-${session.status}`}
                        >
                          {session.status === "open"
                            ? "Booking open"
                            : session.status === "cancelled"
                              ? "Cancelled"
                              : "Booking closed"}
                        </span>

                        <span
                          className={`session-status ${
                            session.is_public
                              ? "session-status-published"
                              : "session-status-hidden"
                          }`}
                        >
                          {session.is_public ? "Published" : "Hidden"}
                        </span>

                        <span className="session-booking-count">
                          {session.confirmed_booking_count} confirmed
                          {session.capacity === null
                            ? ""
                            : ` of ${session.capacity}`}
                        </span>

                        <span className="session-booking-count">
                          {session.assigned_coach_name
                            ? `Coach ${session.assigned_coach_name}`
                            : "No coach assigned"}
                        </span>
                      </div>

                      {session.notes && (
                        <p className="session-description">{session.notes}</p>
                      )}
                    </div>

                    <div className="schedule-session-actions">
                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={busy}
                        onClick={() => beginEdit(session)}
                      >
                        Edit session
                      </button>

                      <label className="session-coach-assignment">
                        Assigned coach
                        <select
                          value={session.assigned_coach_id || ""}
                          disabled={
                            busy ||
                            (coaches.length === 0 && !session.assigned_coach_id)
                          }
                          onChange={(event) =>
                            void assignCoach(
                              session.id,
                              event.target.value || null,
                            )
                          }
                        >
                          <option value="">Unassigned</option>

                          {session.assigned_coach_id &&
                            !coaches.some(
                              (coach) =>
                                coach.coach_id === session.assigned_coach_id,
                            ) && (
                              <option value={session.assigned_coach_id}>
                                {session.assigned_coach_name ||
                                  "Previously assigned coach"}
                              </option>
                            )}

                          {coaches.map((coach) => (
                            <option key={coach.coach_id} value={coach.coach_id}>
                              {coach.coach_name}
                            </option>
                          ))}
                        </select>
                      </label>

                      {coaches.length === 0 && (
                        <span className="admin-help">
                          A coach must submit availability before being
                          assigned.
                        </span>
                      )}

                      {session.status !== "cancelled" && (
                        <button
                          className="admin-button secondary"
                          type="button"
                          disabled={busy}
                          onClick={() =>
                            void updateSession(session.id, {
                              status:
                                session.status === "open" ? "closed" : "open",
                            })
                          }
                        >
                          {session.status === "open"
                            ? "Close booking"
                            : "Reopen booking"}
                        </button>
                      )}

                      {session.status !== "cancelled" && (
                        <button
                          className="admin-button danger"
                          type="button"
                          disabled={busy}
                          onClick={() => {
                            if (
                              window.confirm(
                                `Cancel ${session.program_name}? It will no longer accept booking requests.`,
                              )
                            ) {
                              void updateSession(session.id, {
                                status: "cancelled",
                              });
                            }
                          }}
                        >
                          Cancel session
                        </button>
                      )}

                      <button
                        className="admin-button secondary"
                        type="button"
                        disabled={busy}
                        onClick={() =>
                          void updateSession(session.id, {
                            is_public: !session.is_public,
                          })
                        }
                      >
                        {session.is_public ? "Unpublish" : "Publish"}
                      </button>
                    </div>
                  </div>

                  <form
                    className="schedule-note-form"
                    onSubmit={(event) => {
                      event.preventDefault();

                      const formData = new FormData(event.currentTarget);
                      const noteValue = String(formData.get("notes") || "");

                      void updateSession(session.id, {
                        notes: noteValue,
                      });
                    }}
                  >
                    <label>
                      Admin note
                      <input
                        name="notes"
                        maxLength={2000}
                        defaultValue={session.notes || ""}
                      />
                    </label>

                    <button
                      className="admin-button secondary"
                      type="submit"
                      disabled={busy}
                    >
                      Save note
                    </button>
                  </form>
                </article>

                {editForm(session)}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
