"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Hash, PanelRight, PanelRightClose, Users, WifiOff } from "lucide-react";
import { PersonaAvatar } from "@/components/persona-avatar";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { api, errorMessage } from "@/lib/api";
import { CHANNEL_TOPICS } from "@/lib/personas";
import type { Message } from "@/lib/types";
import { cn } from "@/lib/utils";
import { getWs, useSessionController } from "@/hooks/use-session-controller";
import { useWorkspace } from "@/store/workspace";
import { Composer } from "./composer";
import { MessageList } from "./message-list";
import { RightSidebar } from "./right-sidebar";
import { Sidebar } from "./sidebar";
import { ThreadPane } from "./thread-pane";
import { TypingIndicator } from "./typing-indicator";
import { dmPersona } from "./helpers";

const EMPTY: Message[] = [];

function useMediaQuery(q: string) {
  const [match, setMatch] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia(q);
    const on = () => setMatch(mq.matches);
    on();
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [q]);
  return match;
}

export function Workspace({ sessionId, username }: { sessionId: string; username: string }) {
  const router = useRouter();
  const { loadChannel, loadOlder, sendMessage } = useSessionController(sessionId, true);

  const session = useWorkspace((s) => s.session);
  const channels = useWorkspace((s) => s.channels);
  const personas = useWorkspace((s) => s.personas);
  const tasks = useWorkspace((s) => s.tasks);
  const deadlines = useWorkspace((s) => s.deadlines);
  const activeId = useWorkspace((s) => s.activeChannelId);
  const channelState = useWorkspace((s) => (s.activeChannelId ? s.messages[s.activeChannelId] : undefined));
  const typingMap = useWorkspace((s) => (s.activeChannelId ? s.typing[s.activeChannelId] : undefined));
  const threadParentId = useWorkspace((s) => s.threadParentId);
  const highlightId = useWorkspace((s) => s.highlightMessageId);
  const wsStatus = useWorkspace((s) => s.wsStatus);
  const unreadTotal = useWorkspace((s) => Object.values(s.unread).reduce((a, b) => a + b, 0));
  const { openThread, setActiveChannel, setHighlight } = useWorkspace.getState();

  const isWide = useMediaQuery("(min-width: 1024px)");
  const [rightOpenPref, setRightOpenPref] = useState<boolean | null>(null);
  const rightOpen = rightOpenPref ?? isWide;
  const [ending, setEnding] = useState(false);
  const [completedModal, setCompletedModal] = useState(false);
  const [scrollSignal, setScrollSignal] = useState(0);
  const prevStatus = useRef<string | null>(null);
  const selfEnded = useRef(false);

  const channel = channels.find((c) => c.id === activeId);
  const persona = dmPersona(channel, personas);
  const messages = channelState?.items ?? EMPTY;
  const active = session?.status === "ACTIVE";

  // Load messages when switching channel (always refetch to fill gaps).
  useEffect(() => {
    if (!activeId) return;
    loadChannel(activeId).catch((e) => toast.error(`Failed to load messages: ${errorMessage(e)}`));
  }, [activeId, loadChannel]);

  // Session completion detection.
  useEffect(() => {
    if (!session) return;
    if (prevStatus.current === "ACTIVE" && session.status !== "ACTIVE" && !selfEnded.current) {
      setCompletedModal(true);
    }
    prevStatus.current = session.status;
  }, [session]);

  // Clear highlight after a moment.
  useEffect(() => {
    if (!highlightId) return;
    const t = setTimeout(() => setHighlight(null), 2500);
    return () => clearTimeout(t);
  }, [highlightId, setHighlight]);

  // Tab title with unread count.
  useEffect(() => {
    document.title = `${unreadTotal ? `(${unreadTotal}) ` : ""}WorkLingo`;
    return () => {
      document.title = "WorkLingo";
    };
  }, [unreadTotal]);

  const threadParent = useMemo(
    () => (threadParentId ? messages.find((m) => m.id === threadParentId) : undefined),
    [threadParentId, messages],
  );
  const threadReplies = useMemo(
    () => (threadParentId ? messages.filter((m) => m.parentId === threadParentId) : EMPTY),
    [threadParentId, messages],
  );

  // expired entries are pruned by the controller every second
  const typingIds = useMemo(() => Object.keys(typingMap ?? {}), [typingMap]);

  const onSendMain = useCallback(
    async (text: string) => {
      if (!activeId) return;
      setScrollSignal((n) => n + 1);
      await sendMessage(activeId, text, null);
    },
    [activeId, sendMessage],
  );
  const onSendThread = useCallback(
    async (text: string) => {
      if (!activeId || !threadParentId) return;
      await sendMessage(activeId, text, threadParentId);
    },
    [activeId, threadParentId, sendMessage],
  );
  const onTyping = useCallback(() => {
    if (activeId) getWs()?.sendTyping(activeId);
  }, [activeId]);
  const onReply = useCallback((m: Message) => openThread(m.id), [openThread]);
  const onLoadOlder = useCallback(() => (activeId ? loadOlder(activeId) : Promise.resolve(0)), [activeId, loadOlder]);

  const onJump = useCallback(
    (channelId: string, messageId: string) => {
      setActiveChannel(channelId);
      setHighlight(messageId);
      if (!isWide) setRightOpenPref(false);
    },
    [setActiveChannel, setHighlight, isWide],
  );

  async function endSession() {
    setEnding(true);
    selfEnded.current = true;
    try {
      await api.endSession(sessionId);
      router.push(`/session/${sessionId}/report`);
    } catch (e) {
      selfEnded.current = false;
      toast.error(`Could not end session: ${errorMessage(e)}`);
      setEnding(false);
    }
  }

  const intro = channel ? (
    <div className="px-5 pt-10 pb-4">
      {channel.kind === "CHANNEL" ? (
        <>
          <div className="flex items-center gap-1 text-[28px] font-black text-[#1d1c1d]">
            <Hash className="size-7" /> {channel.name}
          </div>
          <p className="mt-1 text-[15px] text-[#616061]">
            This is the very beginning of the <b className="text-[#1264a3]">#{channel.name}</b> channel.{" "}
            {CHANNEL_TOPICS[channel.name] ?? ""}
          </p>
        </>
      ) : (
        <>
          <div className="flex items-center gap-3">
            <PersonaAvatar persona={persona} size={72} className="rounded-lg" />
            <div>
              <div className="text-[22px] font-black text-[#1d1c1d]">{persona?.displayName}</div>
              <div className="text-[15px] text-[#616061]">{persona?.role}</div>
            </div>
          </div>
          <p className="mt-3 text-[15px] text-[#616061]">
            This conversation is just between <b className="text-[#1264a3]">@{persona?.handle}</b> and you.
          </p>
        </>
      )}
    </div>
  ) : null;

  const placeholder = channel
    ? channel.kind === "CHANNEL"
      ? `Message #${channel.name}`
      : `Message ${persona?.displayName ?? ""}`
    : "Message";

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <Sidebar username={username} />
      <div className="relative flex min-w-0 flex-1">
        <main className="flex min-w-0 flex-1 flex-col bg-white">
          <header className="flex h-12 shrink-0 items-center gap-3 border-b border-[#e1e1e1] px-5">
            <div className="flex min-w-0 flex-1 items-center gap-2">
              {channel?.kind === "DM" ? (
                <>
                  <PersonaAvatar persona={persona} size={24} className="rounded" />
                  <span className="truncate text-[18px] font-black text-[#1d1c1d]">{persona?.displayName}</span>
                  <span className="truncate text-[13px] text-[#616061]">{persona?.role}</span>
                </>
              ) : channel ? (
                <>
                  <span className="flex min-w-0 shrink-0 items-center text-[18px] font-black whitespace-nowrap text-[#1d1c1d]">
                    <Hash className="size-[18px] shrink-0" />
                    <span className="truncate">{channel.name}</span>
                  </span>
                  <span className="hidden min-w-0 truncate border-l border-[#e1e1e1] pl-3 text-[13px] text-[#616061] xl:inline">
                    {CHANNEL_TOPICS[channel.name] ?? ""}
                  </span>
                </>
              ) : (
                <span className="text-[#616061]">Loading…</span>
              )}
            </div>
            {wsStatus === "reconnecting" && (
              <span className="flex items-center gap-1 rounded bg-amber-100 px-2 py-0.5 text-xs font-bold text-amber-800">
                <WifiOff className="size-3.5" /> Reconnecting…
              </span>
            )}
            {channel?.kind === "CHANNEL" && (
              <span className="hidden items-center gap-1 rounded-md border border-[#ddd] px-1.5 py-0.5 sm:flex" title="Members">
                <span className="flex -space-x-1">
                  {personas.map((p) => (
                    <PersonaAvatar key={p.id} persona={p} size={20} className="rounded ring-2 ring-white" />
                  ))}
                </span>
                <Users className="size-3.5 text-[#616061]" />
                <span className="text-[13px] text-[#616061]">{personas.length + 1}</span>
              </span>
            )}
            <button
              type="button"
              onClick={() => setRightOpenPref(!rightOpen)}
              className="flex size-8 items-center justify-center rounded text-[#616061] hover:bg-[#f0f0f0]"
              aria-label={rightOpen ? "Hide session panel" : "Show session panel"}
              title={rightOpen ? "Hide session panel" : "Show session panel"}
            >
              {rightOpen ? <PanelRightClose className="size-5" /> : <PanelRight className="size-5" />}
            </button>
          </header>

          {session && !active && (
            <div className="shrink-0 border-b border-[#e1e1e1] bg-[#f8f8f8] px-5 py-2 text-sm text-[#616061]">
              This session has ended ({session.status.toLowerCase()}). The workspace is read-only.{" "}
              <a href={`/session/${sessionId}/report`} className="font-bold text-[#1264a3] hover:underline">
                View report
              </a>
            </div>
          )}

          {activeId && (
            <MessageList
              key={activeId}
              messages={messages}
              loaded={!!channelState?.loaded}
              hasMore={!!channelState?.hasMore}
              personas={personas}
              deadlines={deadlines}
              tasks={tasks}
              username={username}
              highlightId={highlightId}
              intro={intro}
              onReply={onReply}
              onLoadOlder={onLoadOlder}
              scrollSignal={scrollSignal}
            />
          )}
          {!activeId && <div className="flex-1" />}
          <TypingIndicator personaIds={typingIds} personas={personas} />
          <Composer
            placeholder={placeholder}
            personas={personas}
            disabled={!active || !activeId}
            disabledReason={session ? "This session has ended." : "Loading…"}
            onSend={onSendMain}
            onTyping={onTyping}
            autoFocus
            focusKey={activeId ?? ""}
          />
        </main>

        {threadParent && (
          <ThreadPane
            parent={threadParent}
            replies={threadReplies}
            channel={channel}
            personas={personas}
            deadlines={deadlines}
            tasks={tasks}
            username={username}
            disabled={!active}
            onClose={() => openThread(null)}
            onSend={onSendThread}
            onTyping={onTyping}
          />
        )}
      </div>

      {rightOpen && (
        <div className={cn(!isWide && "absolute inset-y-0 right-0 z-40 shadow-2xl")}>
          <RightSidebar
            session={session}
            tasks={tasks}
            deadlines={deadlines}
            channels={channels}
            personas={personas}
            ending={ending}
            onEnd={endSession}
            onJump={onJump}
          />
        </div>
      )}

      <Dialog open={completedModal} onOpenChange={setCompletedModal}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Session complete</DialogTitle>
            <DialogDescription>
              Time&apos;s up! Your workday is over and your messages are being evaluated. Let&apos;s look at how you
              did.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCompletedModal(false)}>
              Stay here
            </Button>
            <Button
              className="bg-[#007a5a] text-white hover:bg-[#148567]"
              onClick={() => router.push(`/session/${sessionId}/report`)}
            >
              View my report
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
