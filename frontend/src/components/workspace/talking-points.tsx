"use client";

import { Check, Circle } from "lucide-react";
import type { TalkingPoint } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Ordered list of a Task's Talking points: ○ pending / ✓ covered (live via task.updated). */
export function TalkingPointList({ points, className }: { points: TalkingPoint[]; className?: string }) {
  const sorted = [...points].sort((a, b) => a.position - b.position);
  return (
    <ul className={cn("flex flex-col gap-1", className)}>
      {sorted.map((tp) => {
        const covered = tp.status === "COVERED";
        return (
          <li key={tp.id} className="flex items-start gap-1.5 text-xs leading-snug">
            {covered ? (
              <Check className="mt-px size-3.5 shrink-0 text-[#007a5a]" aria-label="Covered" />
            ) : (
              <Circle className="mt-px size-3.5 shrink-0 text-[#a0a0a0]" aria-label="Pending" />
            )}
            <span className={cn("break-words", covered ? "text-[#007a5a]" : "text-[#454245]")}>{tp.content}</span>
          </li>
        );
      })}
    </ul>
  );
}
