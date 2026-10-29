import type { PracticeAnswer, PracticeSubmission } from "./types";

export type PracticeDraft = { answers: Record<string, string>; selected: string[]; pending: PracticeSubmission | null };
export function restoreDraft(raw: string | null): PracticeDraft {
  const empty: PracticeDraft = { answers: {}, selected: [], pending: null };
  if (!raw) return empty;
  try {
    const value = JSON.parse(raw);
    if (!value || typeof value !== "object" || !value.answers || typeof value.answers !== "object" || Array.isArray(value.answers) || !Object.values(value.answers).every((v) => typeof v === "string") || !Array.isArray(value.selected) || !value.selected.every((v: unknown) => typeof v === "string")) return empty;
    const pending = value.pending;
    if (pending && (typeof pending.requestId !== "string" || !Array.isArray(pending.answers) || !pending.answers.every((a: PracticeAnswer) => a && typeof a.exerciseId === "string" && typeof a.answer === "string"))) value.pending = null;
    return { answers: value.answers, selected: value.selected, pending: value.pending ?? null };
  } catch { return empty; }
}
export function submissionFor(answers: PracticeAnswer[], pending: PracticeSubmission | null): PracticeSubmission {
  if (pending && JSON.stringify(pending.answers) === JSON.stringify(answers)) return pending;
  return { requestId: crypto.randomUUID(), answers };
}
