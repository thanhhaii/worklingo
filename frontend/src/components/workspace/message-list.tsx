"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, Loader2 } from "lucide-react";
import { formatDay } from "@/lib/format";
import type { Deadline, Message, Persona, Task } from "@/lib/types";
import { MessageItem } from "./message-item";
import { deadlineFor, isGrouped } from "./helpers";

type Props = {
  messages: Message[];
  loaded: boolean;
  hasMore: boolean;
  personas: Persona[];
  deadlines: Deadline[];
  tasks: Task[];
  username: string;
  highlightId: string | null;
  intro: React.ReactNode;
  onReply: (m: Message) => void;
  onLoadOlder: () => Promise<number>;
  /** bump to force scroll to bottom (e.g. after user sends) */
  scrollSignal: number;
};

const NEAR_BOTTOM_PX = 120;

export function MessageList({
  messages,
  loaded,
  hasMore,
  personas,
  deadlines,
  tasks,
  username,
  highlightId,
  intro,
  onReply,
  onLoadOlder,
  scrollSignal,
}: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const [showJump, setShowJump] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const prevHeight = useRef<number | null>(null);

  const knownHandles = useMemo(() => personas.map((p) => p.handle), [personas]);

  const { top, repliesByParent } = useMemo(() => {
    const top: Message[] = [];
    const repliesByParent = new Map<string, Message[]>();
    for (const m of messages) {
      if (m.parentId) {
        const arr = repliesByParent.get(m.parentId) ?? [];
        arr.push(m);
        repliesByParent.set(m.parentId, arr);
      } else top.push(m);
    }
    return { top, repliesByParent };
  }, [messages]);

  const tasksByMessage = useMemo(() => new Map(tasks.map((t) => [t.messageId, t])), [tasks]);

  const scrollToBottom = useCallback((smooth = false) => {
    const el = scroller.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: smooth ? "smooth" : "auto" });
  }, []);

  // Parent keys this component by channelId, so mount == channel switch: start at the bottom.
  useLayoutEffect(() => {
    scrollToBottom();
  }, [scrollToBottom]);

  // Content change (new message, streaming delta): follow if user is at bottom.
  const last = top[top.length - 1];
  const contentSig = `${top.length}:${last?.id}:${last?.content.length}:${messages.length}`;
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    if (prevHeight.current !== null) {
      // restore position after prepending older messages
      el.scrollTop = el.scrollHeight - prevHeight.current;
      prevHeight.current = null;
      return;
    }
    if (atBottom.current) scrollToBottom();
    else setShowJump(true);
  }, [contentSig, scrollToBottom]);

  useEffect(() => {
    if (scrollSignal === 0) return;
    atBottom.current = true;
    requestAnimationFrame(() => {
      setShowJump(false);
      scrollToBottom(true);
    });
  }, [scrollSignal, scrollToBottom]);

  // jump to highlighted message
  useEffect(() => {
    if (!highlightId) return;
    const el = document.getElementById(`msg-${highlightId}`);
    if (el) {
      atBottom.current = false;
      el.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  }, [highlightId, top.length]);

  const onScroll = async () => {
    const el = scroller.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    atBottom.current = dist < NEAR_BOTTOM_PX;
    if (atBottom.current) setShowJump(false);
    if (el.scrollTop < 80 && hasMore && !loadingOlder && loaded) {
      setLoadingOlder(true);
      prevHeight.current = el.scrollHeight;
      try {
        const n = await onLoadOlder();
        if (n === 0) prevHeight.current = null;
      } catch {
        prevHeight.current = null;
      } finally {
        setLoadingOlder(false);
      }
    }
  };

  return (
    <div className="relative min-h-0 flex-1">
      <div ref={scroller} onScroll={onScroll} className="h-full overflow-x-hidden overflow-y-auto">
        <div className="flex min-h-full flex-col justify-end pb-2">
          {loadingOlder && (
            <div className="flex justify-center py-2 text-[#616061]">
              <Loader2 className="size-4 animate-spin" />
            </div>
          )}
          {loaded && !hasMore && intro}
          {!loaded && (
            <div className="flex flex-1 items-center justify-center py-10 text-sm text-[#616061]">
              <Loader2 className="mr-2 size-4 animate-spin" /> Loading messages…
            </div>
          )}
          {top.map((m, i) => {
            const prev = top[i - 1];
            const newDay = !prev || new Date(prev.createdAt).toDateString() !== new Date(m.createdAt).toDateString();
            return (
              <div key={m.id}>
                {newDay && (
                  <div className="relative my-2 flex items-center justify-center">
                    <div className="absolute inset-x-0 top-1/2 border-t border-[#e1e1e1]" />
                    <span className="relative rounded-full border border-[#e1e1e1] bg-white px-3 py-0.5 text-[13px] font-bold text-[#1d1c1d]">
                      {formatDay(m.createdAt)}
                    </span>
                  </div>
                )}
                <MessageItem
                  message={m}
                  grouped={!newDay && isGrouped(prev, m)}
                  persona={personas.find((p) => p.id === m.personaId)}
                  personas={personas}
                  username={username}
                  knownHandles={knownHandles}
                  deadline={deadlineFor(deadlines, m.id)}
                  task={tasksByMessage.get(m.id)}
                  replies={repliesByParent.get(m.id)}
                  highlighted={highlightId === m.id}
                  onReply={onReply}
                />
              </div>
            );
          })}
        </div>
      </div>
      {showJump && (
        <button
          type="button"
          onClick={() => {
            atBottom.current = true;
            setShowJump(false);
            scrollToBottom(true);
          }}
          className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-full bg-[#1164a3] px-3 py-1 text-xs font-bold text-white shadow-md hover:bg-[#0b4c80]"
        >
          <ArrowDown className="size-3.5" /> New messages
        </button>
      )}
    </div>
  );
}
