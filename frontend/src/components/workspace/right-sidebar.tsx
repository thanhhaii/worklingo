"use client";

import { Clock, Flag, Hash, ListTodo, LogOut, MessageCircle, TriangleAlert } from "lucide-react";
import { useNow } from "@/hooks/use-now";
import { formatCountdown } from "@/lib/format";
import type { Channel, Deadline, Persona, Session, Task } from "@/lib/types";
import { cn } from "@/lib/utils";
import { PersonaAvatar } from "@/components/persona-avatar";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { DeadlineBadge } from "./deadline-badge";
import { channelLabel } from "./helpers";

const STRIKES = 3;

type Props = {
  session: Session | null;
  tasks: Task[];
  deadlines: Deadline[];
  channels: Channel[];
  personas: Persona[];
  ending: boolean;
  onEnd: () => void;
  onJump: (channelId: string, messageId: string) => void;
};

export function RightSidebar({ session, tasks, deadlines, channels, personas, ending, onEnd, onJump }: Props) {
  const now = useNow();
  const remaining = session && now ? new Date(session.expiresAt).getTime() - now : null;
  const total = session ? new Date(session.expiresAt).getTime() - new Date(session.startedAt).getTime() : 1;
  const pct = remaining === null ? 100 : Math.max(0, Math.min(100, (remaining / total) * 100));
  const warnings = session?.warnings ?? 0;
  const active = session?.status === "ACTIVE";

  const order = { OPEN: 0, ACKNOWLEDGED: 1, DONE: 2 } as const;
  const sortedTasks = [...tasks].sort(
    (a, b) =>
      order[a.status] - order[b.status] ||
      (a.priority === b.priority ? 0 : a.priority === "CRITICAL" ? -1 : 1) ||
      b.createdAt.localeCompare(a.createdAt),
  );
  // pending deadlines not tied to a task (direct @mention requests)
  const pendingRequests = deadlines.filter((d) => d.status === "PENDING" && !d.taskId);

  return (
    <aside className="flex h-full w-[300px] shrink-0 flex-col border-l border-[#e1e1e1] bg-[#fbfbfb]">
      <div className="flex h-12 shrink-0 items-center border-b border-[#e1e1e1] px-4 text-[15px] font-black text-[#1d1c1d]">
        Session
        {!active && session && (
          <span className="ml-2 rounded bg-[#616061] px-1.5 text-[11px] font-bold text-white">{session.status}</span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <section className="border-b border-[#e1e1e1] p-4">
          <div className="flex items-center gap-1.5 text-xs font-bold tracking-wide text-[#616061] uppercase">
            <Clock className="size-3.5" /> Time remaining
          </div>
          <div
            className={cn(
              "mt-1 text-3xl font-black text-[#1d1c1d] tabular-nums",
              remaining !== null && remaining < 5 * 60_000 && active && "text-[#e01e5a]",
            )}
          >
            {!session || remaining === null ? "--:--" : active ? formatCountdown(remaining) : "00:00"}
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[#e8e8e8]">
            <div
              className={cn("h-full rounded-full transition-[width] duration-1000", pct < 10 ? "bg-[#e01e5a]" : "bg-[#2eb67d]")}
              style={{ width: `${active ? pct : 0}%` }}
            />
          </div>
        </section>

        <section className="border-b border-[#e1e1e1] p-4">
          <div className="flex items-center gap-1.5 text-xs font-bold tracking-wide text-[#616061] uppercase">
            <TriangleAlert className="size-3.5" /> Ghosting warnings
          </div>
          <div className="mt-2 flex items-center gap-2">
            {Array.from({ length: STRIKES }).map((_, i) => (
              <span
                key={i}
                title={i < warnings ? "Warning received" : "No warning"}
                className={cn(
                  "flex size-8 items-center justify-center rounded-md border-2",
                  i < warnings ? "border-[#e01e5a] bg-[#e01e5a] text-white" : "border-dashed border-[#ccc] text-[#ccc]",
                )}
              >
                <Flag className="size-4" />
              </span>
            ))}
            <span className={cn("ml-1 text-sm font-bold tabular-nums", warnings > 0 ? "text-[#e01e5a]" : "text-[#616061]")}>
              {warnings}/{STRIKES}
              {warnings > STRIKES && <span className="ml-1 text-xs">(+{warnings - STRIKES})</span>}
            </span>
          </div>
          <p className="mt-2 text-xs text-[#616061]">
            Reply within 5 minutes when someone asks you something. Warnings lower your score.
          </p>
        </section>

        <section className="p-4">
          <div className="flex items-center gap-1.5 text-xs font-bold tracking-wide text-[#616061] uppercase">
            <ListTodo className="size-3.5" /> Tasks & requests
          </div>
          {sortedTasks.length === 0 && pendingRequests.length === 0 && (
            <p className="mt-2 text-sm text-[#616061]">No tasks yet. Your teammates will drop some in soon.</p>
          )}
          <ul className="mt-2 flex flex-col gap-2">
            {pendingRequests.map((d) => {
              const ch = channels.find((c) => c.id === d.channelId);
              return (
                <li key={d.id}>
                  <button
                    type="button"
                    onClick={() => onJump(d.channelId, d.messageId)}
                    className="w-full rounded-lg border border-[#f3c4d2] bg-white p-2.5 text-left hover:border-[#e01e5a] hover:shadow-sm"
                  >
                    <div className="flex items-center gap-1.5 text-[13px] font-bold text-[#1d1c1d]">
                      <MessageCircle className="size-3.5 text-[#e01e5a]" /> Awaiting your reply
                    </div>
                    <div className="mt-1 flex items-center justify-between gap-2">
                      <span className="flex min-w-0 items-center gap-1 truncate text-xs text-[#616061]">
                        {ch?.kind === "CHANNEL" ? <Hash className="size-3" /> : null}
                        {channelLabel(ch, personas).replace(/^#/, "")}
                      </span>
                      <DeadlineBadge deadline={d} compact />
                    </div>
                  </button>
                </li>
              );
            })}
            {sortedTasks.map((t) => {
              const p = personas.find((x) => x.id === t.personaId);
              const ch = channels.find((c) => c.id === t.channelId);
              const dl =
                deadlines.find((d) => d.taskId === t.id && d.status === "PENDING") ??
                deadlines.find((d) => d.taskId === t.id);
              return (
                <li key={t.id}>
                  <button
                    type="button"
                    onClick={() => onJump(t.channelId, t.messageId)}
                    className={cn(
                      "w-full rounded-lg border bg-white p-2.5 text-left hover:shadow-sm",
                      t.status === "DONE" ? "border-[#e1e1e1] opacity-70" : "border-[#ddd] hover:border-[#1264a3]",
                    )}
                  >
                    <div className="flex items-start gap-2">
                      <PersonaAvatar persona={p} size={20} className="mt-0.5 rounded" />
                      <div className="min-w-0 flex-1">
                        <div className="line-clamp-2 text-[13px] font-bold break-words text-[#1d1c1d]">{t.title}</div>
                        <div className="mt-0.5 truncate text-xs text-[#616061]">
                          {p?.displayName ?? t.personaId} · {channelLabel(ch, personas)}
                        </div>
                      </div>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      {t.priority === "CRITICAL" && (
                        <span className="rounded bg-[#e01e5a] px-1.5 py-px text-[10px] font-black tracking-wide text-white uppercase">
                          Critical
                        </span>
                      )}
                      <span
                        className={cn(
                          "rounded px-1.5 py-px text-[10px] font-bold tracking-wide uppercase",
                          t.status === "OPEN" && "bg-[#fff4e5] text-[#9a5b00]",
                          t.status === "ACKNOWLEDGED" && "bg-[#e8f1fa] text-[#1264a3]",
                          t.status === "DONE" && "bg-[#e5f4ee] text-[#007a5a]",
                        )}
                      >
                        {t.status === "ACKNOWLEDGED" ? "Acknowledged" : t.status === "OPEN" ? "Open" : "Done"}
                      </span>
                      {dl && dl.status !== "RESOLVED" && <span className="ml-auto"><DeadlineBadge deadline={dl} compact /></span>}
                    </div>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      </div>
      <div className="shrink-0 border-t border-[#e1e1e1] p-4">
        {active ? (
          <AlertDialog>
            <AlertDialogTrigger
              disabled={ending || !session}
              className="flex h-9 w-full items-center justify-center gap-2 rounded-lg border border-[#e01e5a] bg-white text-sm font-bold text-[#e01e5a] hover:bg-[#fff0f4] disabled:opacity-50"
            >
              <LogOut className="size-4" /> {ending ? "Ending…" : "End session"}
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>End this session?</AlertDialogTitle>
                <AlertDialogDescription>
                  Your conversation will be evaluated and you&apos;ll see your report. You can&apos;t send more messages
                  after ending.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Keep working</AlertDialogCancel>
                <AlertDialogAction className="bg-[#e01e5a] text-white hover:bg-[#c01a4d]" onClick={onEnd}>
                  End session
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        ) : (
          session && (
            <a
              href={`/session/${session.id}/report`}
              className="flex h-9 w-full items-center justify-center rounded-lg bg-[#007a5a] text-sm font-bold text-white hover:bg-[#148567]"
            >
              View report
            </a>
          )
        )}
      </div>
    </aside>
  );
}
