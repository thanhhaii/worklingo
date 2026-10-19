"use client";

import { useEffect, useMemo, useRef } from "react";
import { X } from "lucide-react";
import type { Channel, Deadline, Message, Persona, Task } from "@/lib/types";
import { Composer } from "./composer";
import { MessageItem } from "./message-item";
import { channelLabel, deadlineFor, isGrouped } from "./helpers";

type Props = {
  parent: Message;
  replies: Message[];
  channel: Channel | undefined;
  personas: Persona[];
  deadlines: Deadline[];
  tasks: Task[];
  username: string;
  disabled: boolean;
  onClose: () => void;
  onSend: (text: string) => Promise<void>;
  onTyping: () => void;
};

export function ThreadPane({
  parent,
  replies,
  channel,
  personas,
  deadlines,
  tasks,
  username,
  disabled,
  onClose,
  onSend,
  onTyping,
}: Props) {
  const knownHandles = useMemo(() => personas.map((p) => p.handle), [personas]);
  const scroller = useRef<HTMLDivElement>(null);
  const last = replies[replies.length - 1];
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (dist < 200) el.scrollTop = el.scrollHeight;
  }, [replies.length, last?.content.length]);

  const task = tasks.find((t) => t.messageId === parent.id);

  return (
    <aside className="flex h-full w-[380px] shrink-0 flex-col border-l border-[#e1e1e1] bg-white max-xl:absolute max-xl:inset-y-0 max-xl:right-0 max-xl:z-30 max-xl:shadow-xl">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-[#e1e1e1] px-4">
        <div className="min-w-0">
          <div className="text-[17px] font-black text-[#1d1c1d]">Thread</div>
        </div>
        <div className="flex items-center gap-2">
          <span className="truncate text-[13px] text-[#616061]">{channelLabel(channel, personas)}</span>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close thread"
            className="flex size-8 items-center justify-center rounded hover:bg-[#f0f0f0]"
          >
            <X className="size-4" />
          </button>
        </div>
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto py-2">
        <MessageItem
          message={parent}
          grouped={false}
          persona={personas.find((p) => p.id === parent.personaId)}
          personas={personas}
          username={username}
          knownHandles={knownHandles}
          deadline={deadlineFor(deadlines, parent.id)}
          task={task}
          inThread
        />
        <div className="my-2 flex items-center gap-3 px-5 text-[13px] text-[#616061]">
          <span>
            {replies.length} {replies.length === 1 ? "reply" : "replies"}
          </span>
          <div className="flex-1 border-t border-[#e1e1e1]" />
        </div>
        {replies.map((m, i) => (
          <MessageItem
            key={m.id}
            message={m}
            grouped={isGrouped(replies[i - 1], m)}
            persona={personas.find((p) => p.id === m.personaId)}
            personas={personas}
            username={username}
            knownHandles={knownHandles}
            deadline={deadlineFor(deadlines, m.id)}
            inThread
          />
        ))}
      </div>
      <Composer
        placeholder="Reply…"
        personas={personas}
        disabled={disabled}
        disabledReason="This session has ended."
        onSend={onSend}
        onTyping={onTyping}
        autoFocus
        focusKey={parent.id}
      />
    </aside>
  );
}
