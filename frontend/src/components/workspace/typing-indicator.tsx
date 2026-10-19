"use client";

import type { Persona } from "@/lib/types";

export function TypingIndicator({ personaIds, personas }: { personaIds: string[]; personas: Persona[] }) {
  const names = personaIds.map((id) => personas.find((p) => p.id === id)?.displayName ?? id);
  let text = "";
  if (names.length === 1) text = `${names[0]} is typing`;
  else if (names.length === 2) text = `${names[0]} and ${names[1]} are typing`;
  else if (names.length > 2) text = "Several people are typing";
  return (
    <div className="h-5 px-5 text-xs text-[#616061]" aria-live="polite">
      {text && (
        <span className="flex items-center gap-1">
          <span className="flex gap-0.5">
            <span className="typing-dot size-1 rounded-full bg-[#616061]" />
            <span className="typing-dot size-1 rounded-full bg-[#616061]" />
            <span className="typing-dot size-1 rounded-full bg-[#616061]" />
          </span>
          <b className="font-bold">{text}</b>…
        </span>
      )}
    </div>
  );
}
