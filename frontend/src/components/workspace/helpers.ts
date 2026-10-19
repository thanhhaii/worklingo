import type { Channel, Deadline, Message, Persona } from "@/lib/types";

export function channelLabel(ch: Channel | undefined, personas: Persona[]): string {
  if (!ch) return "";
  if (ch.kind === "CHANNEL") return `#${ch.name}`;
  const p = personas.find((x) => x.id === (ch.personaId ?? ch.name));
  return p?.displayName ?? ch.name;
}

export function dmPersona(ch: Channel | undefined, personas: Persona[]): Persona | undefined {
  if (!ch || ch.kind !== "DM") return undefined;
  return personas.find((x) => x.id === (ch.personaId ?? ch.name));
}

export function authorKey(m: Message): string {
  return m.authorType === "PERSONA" ? `p:${m.personaId}` : m.authorType === "USER" ? "u" : "s";
}

const GROUP_WINDOW_MS = 5 * 60_000;

export function isGrouped(prev: Message | undefined, m: Message): boolean {
  if (!prev) return false;
  if (m.authorType === "SYSTEM" || prev.authorType === "SYSTEM") return false;
  if (authorKey(prev) !== authorKey(m)) return false;
  const a = new Date(prev.createdAt);
  const b = new Date(m.createdAt);
  if (a.toDateString() !== b.toDateString()) return false;
  return b.getTime() - a.getTime() < GROUP_WINDOW_MS;
}

export function deadlineFor(deadlines: Deadline[], messageId: string): Deadline | undefined {
  // prefer pending deadline, else latest
  let best: Deadline | undefined;
  for (const d of deadlines) {
    if (d.messageId !== messageId) continue;
    if (d.status === "PENDING") return d;
    if (!best || d.createdAt > best.createdAt) best = d;
  }
  return best;
}
