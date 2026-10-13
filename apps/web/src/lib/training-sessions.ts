export type TrainingSession = {
  id: string;
  program_name: string;
  program_description: string | null;
  starts_at: string;
  ends_at: string;
  location: string;
  capacity: number | null;
  confirmed_booking_count: number;
  remaining_capacity: number | null;
  status: string;
  is_public: boolean;
};

export type TrainingProgram = {
  id: string;
  name: string;
  description: string | null;
};

const DEFAULT_API_BASE_URL = "http://localhost:8080";

export async function getActiveTrainingPrograms(): Promise<TrainingProgram[]> {
  const baseUrl = (process.env.API_BASE_URL || DEFAULT_API_BASE_URL).replace(/\/$/, "");
  const response = await fetch(`${baseUrl}/api/training-programs`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error("Training program request failed");
  }

  const programs: unknown = await response.json();
  if (!Array.isArray(programs)) {
    throw new Error("Training program response was invalid");
  }
  return programs as TrainingProgram[];
}

export async function getUpcomingTrainingSessions(): Promise<TrainingSession[]> {
  const baseUrl = (process.env.API_BASE_URL || DEFAULT_API_BASE_URL).replace(/\/$/, "");
  const response = await fetch(`${baseUrl}/api/training-sessions`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error("Training schedule request failed");
  }

  const sessions: unknown = await response.json();
  if (!Array.isArray(sessions)) {
    throw new Error("Training schedule response was invalid");
  }

  return sessions as TrainingSession[];
}
