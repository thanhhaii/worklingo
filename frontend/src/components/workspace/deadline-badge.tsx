"use client";

import { CheckCircle2, Timer, TimerOff } from "lucide-react";
import { useNow } from "@/hooks/use-now";
import { formatCountdown } from "@/lib/format";
import type { Deadline } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Display-only countdown; the server decides when a deadline expires. */
export function DeadlineBadge({ deadline, compact }: { deadline: Deadline; compact?: boolean }) {
  const now = useNow();
  if (deadline.status === "RESOLVED") {
    return (
      <span className="inline-flex items-center gap-1 rounded border border-emerald-200 bg-emerald-50 px-1.5 py-px text-[11px] font-bold text-emerald-700">
        <CheckCircle2 className="size-3" /> {compact ? "Done" : "Responded"}
      </span>
    );
  }
  if (deadline.status === "EXPIRED") {
    return (
      <span className="inline-flex items-center gap-1 rounded border border-red-200 bg-red-50 px-1.5 py-px text-[11px] font-bold text-red-700">
        <TimerOff className="size-3" /> {compact ? "Missed" : "Missed deadline"}
      </span>
    );
  }
  const remaining = now === 0 ? 0 : new Date(deadline.deadlineAt).getTime() - now;
  const urgent = remaining < 60_000;
  return (
    <span
      title="Reply in this channel before the timer runs out"
      className={cn(
        "inline-flex items-center gap-1 rounded bg-[#e01e5a] px-1.5 py-px text-[11px] font-bold text-white tabular-nums",
        urgent && "animate-pulse",
      )}
    >
      <Timer className="size-3" />
      {now === 0 ? "--:--" : remaining <= 0 ? "00:00" : formatCountdown(remaining)}
      {!compact && <span className="font-normal opacity-90">to reply</span>}
    </span>
  );
}
