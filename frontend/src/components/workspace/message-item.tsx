"use client";

import { memo, useState } from "react";
import { ChevronDown, MessageSquareText, TriangleAlert, Loader2 } from "lucide-react";
import { PersonaAvatar } from "@/components/persona-avatar";
import { formatTime } from "@/lib/format";
import type { Deadline, Message, Persona, Task } from "@/lib/types";
import { cn } from "@/lib/utils";
import { DeadlineBadge } from "./deadline-badge";
import { Markdown } from "./markdown";
import { TalkingPointList } from "./talking-points";

export type MessageItemProps = {
  message: Message;
  grouped: boolean;
  persona: Persona | undefined;
  username: string;
  knownHandles: string[];
  deadline?: Deadline;
  task?: Task;
  replies?: Message[];
  personas: Persona[];
  highlighted?: boolean;
  onReply?: (m: Message) => void;
  inThread?: boolean;
};

function TaskChip({ task }: { task: Task }) {
  const points = task.talkingPoints ?? [];
  const covered = points.filter((p) => p.status === "COVERED").length;
  // expanded by default unless DONE; user toggle overrides
  const [override, setOverride] = useState<boolean | null>(null);
  const open = override ?? task.status !== "DONE";
  return (
    <div className="mt-0.5 mb-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <span
          className={cn(
            "rounded px-1.5 py-px text-[10px] font-black tracking-wide uppercase",
            task.priority === "CRITICAL" ? "bg-[#e01e5a] text-white" : "bg-[#1264a3]/10 text-[#1264a3]",
          )}
        >
          {task.priority === "CRITICAL" ? "Critical task" : "Task"}
        </span>
        <span className="text-[13px] font-bold text-[#1d1c1d]">{task.title}</span>
        {points.length > 0 && (
          <button
            type="button"
            onClick={() => setOverride(!open)}
            aria-expanded={open}
            className="flex items-center gap-0.5 rounded px-1 text-xs text-[#616061] hover:bg-[#f0f0f0]"
          >
            Talking points {covered}/{points.length}
            <ChevronDown className={cn("size-3.5 transition-transform", !open && "-rotate-90")} />
          </button>
        )}
      </div>
      {open && points.length > 0 && (
        <TalkingPointList points={points} className="mt-1 rounded-md border border-[#e1e1e1] bg-[#fafafa] px-2 py-1.5" />
      )}
    </div>
  );
}

function MessageItemInner({
  message: m,
  grouped,
  persona,
  username,
  knownHandles,
  deadline,
  task,
  replies,
  personas,
  highlighted,
  onReply,
  inThread,
}: MessageItemProps) {
  const time = formatTime(m.createdAt);

  if (m.authorType === "SYSTEM") {
    return (
      <div id={`msg-${m.id}`} className={cn("px-3 py-1.5 md:px-5", highlighted && "msg-flash")}>
        <div className="flex gap-3 rounded-md border border-amber-300 border-l-4 border-l-[#e01e5a] bg-amber-50 px-3 py-2">
          <TriangleAlert className="mt-0.5 size-5 shrink-0 text-[#e01e5a]" />
          <div className="min-w-0 flex-1">
            <div className="flex items-baseline gap-2">
              <span className="text-[13px] font-black tracking-wide text-[#9b1b45] uppercase">WorkLingo System</span>
              <span className="text-xs text-[#616061]">{time}</span>
            </div>
            <Markdown content={m.content} knownHandles={knownHandles} selfHandle={username} className="text-[#5c2a00]" />
          </div>
        </div>
      </div>
    );
  }

  const name = m.authorType === "USER" ? username : (persona?.displayName ?? "Unknown");
  const avatarPersona =
    m.authorType === "USER" ? { displayName: username, avatarColor: "#1264A3" } : persona;
  const replyCount = replies?.length ?? 0;
  const replyAvatars = replies
    ? [...new Map(replies.map((r) => [r.authorType === "USER" ? "u" : r.personaId, r])).values()].slice(0, 4)
    : [];

  return (
    <div
      id={`msg-${m.id}`}
      className={cn(
        "group relative flex gap-2 px-3 hover:bg-[#f8f8f8] md:px-5",
        grouped ? "py-0.5" : "pt-2 pb-1",
        highlighted && "msg-flash",
        deadline?.status === "PENDING" && "bg-[#fff5f7] hover:bg-[#ffeef2]",
      )}
    >
      <div className="w-9 shrink-0">
        {grouped ? (
          <span className="invisible block pt-1 text-right text-[11px] leading-[22px] text-[#616061] group-hover:visible">
            {new Date(m.createdAt).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" }).replace(/\s?[AP]M/i, "")}
          </span>
        ) : (
          <PersonaAvatar persona={avatarPersona} size={36} className="mt-0.5" />
        )}
      </div>
      <div className={cn("min-w-0 flex-1", !inThread && onReply && !m.pending && "pointer-coarse:pr-10")}>
        {!grouped && (
          <div className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-[15px] font-black text-[#1d1c1d]">{name}</span>
            {persona && m.authorType === "PERSONA" && (
              <span className="text-xs text-[#616061]">{persona.role}</span>
            )}
            <span className="text-xs text-[#616061]">{time}</span>
          </div>
        )}
        {task && <TaskChip task={task} />}
        <div className={cn(m.pending && "opacity-60")}>
          <Markdown
            content={m.content}
            streaming={m.streaming}
            knownHandles={knownHandles}
            selfHandle={username}
          />
        </div>
        {(deadline || m.pending) && (
          <div className="mt-1 flex items-center gap-2">
            {deadline && <DeadlineBadge deadline={deadline} />}
            {m.pending && (
              <span className="flex items-center gap-1 text-xs text-[#616061]">
                <Loader2 className="size-3 animate-spin" /> Sending…
              </span>
            )}
          </div>
        )}
        {!inThread && replyCount > 0 && onReply && (
          <button
            type="button"
            onClick={() => onReply(m)}
            className="mt-1 -ml-1 flex items-center gap-1.5 rounded-md border border-transparent px-1 py-0.5 text-[13px] hover:border-[#ddd] hover:bg-white"
          >
            {replyAvatars.map((r) => (
              <PersonaAvatar
                key={r.id}
                size={20}
                className="rounded"
                persona={
                  r.authorType === "USER"
                    ? { displayName: username, avatarColor: "#1264A3" }
                    : personas.find((p) => p.id === r.personaId)
                }
              />
            ))}
            <span className="font-bold text-[#1264a3]">
              {replyCount} {replyCount === 1 ? "reply" : "replies"}
            </span>
            <span className="text-xs text-[#616061] max-sm:hidden">Last reply {formatTime(replies![replies!.length - 1]!.createdAt)}</span>
          </button>
        )}
      </div>
      {!inThread && onReply && !m.pending && (
        <div className="absolute -top-3 right-3 hidden rounded-md border border-[#ddd] bg-white shadow-sm group-hover:flex pointer-coarse:top-1 pointer-coarse:flex md:right-5">
          <button
            type="button"
            onClick={() => onReply(m)}
            className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-[#1d1c1d] hover:bg-[#f0f0f0]"
            title="Reply in thread"
          >
            <MessageSquareText className="size-4" /> <span className="pointer-coarse:hidden">Reply in thread</span>
          </button>
        </div>
      )}
    </div>
  );
}

export const MessageItem = memo(MessageItemInner);
