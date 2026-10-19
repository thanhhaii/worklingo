import { MOCK } from "@/lib/config";

export function MockBanner() {
  if (!MOCK) return null;
  return (
    <div className="shrink-0 bg-amber-300 px-3 py-0.5 text-center text-[11px] font-semibold text-amber-950">
      MOCK MODE: in-browser fake API and WebSocket (NEXT_PUBLIC_MOCK=1). No backend is used.
    </div>
  );
}
