import { cn } from "@/lib/utils";

export function scoreColor(n: number) {
  if (n >= 80) return "text-emerald-700 bg-emerald-50 border-emerald-200";
  if (n >= 60) return "text-amber-700 bg-amber-50 border-amber-200";
  return "text-red-700 bg-red-50 border-red-200";
}

export function ScorePill({ value, className }: { value: number; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex min-w-10 items-center justify-center rounded-full border px-2 py-0.5 text-xs font-bold tabular-nums",
        scoreColor(value),
        className,
      )}
    >
      {Math.round(value)}
    </span>
  );
}
