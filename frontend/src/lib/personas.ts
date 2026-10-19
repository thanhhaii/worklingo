import type { Persona } from "./types";

/** Static persona list from the contract; used as fallback before GET /personas resolves. */
export const DEFAULT_PERSONAS: Persona[] = [
  { id: "sarah", handle: "pm_sarah", displayName: "Sarah Chen", role: "Product Manager", avatarColor: "#E01E5A" },
  { id: "alex", handle: "tech_lead_alex", displayName: "Alex Novak", role: "Tech Lead", avatarColor: "#2EB67D" },
  { id: "david", handle: "client_david", displayName: "David Miller", role: "Client (UK)", avatarColor: "#ECB22E" },
];

export const CHANNEL_ORDER = ["general", "sprint-planning", "incident-room", "watercooler"];

export const CHANNEL_TOPICS: Record<string, string> = {
  general: "Company-wide announcements and work-based matters",
  "sprint-planning": "Sprint goals, estimates and scope discussions",
  "incident-room": "Production incidents. Keep it calm, clear and concise.",
  watercooler: "Non-work banter and small talk",
};

export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join("");
}
