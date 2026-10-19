"use client";

import { create } from "zustand";
import type {
  Channel,
  Deadline,
  Evaluation,
  Message,
  Persona,
  ServerEvent,
  Session,
  SessionDetails,
  Task,
  Warning,
} from "@/lib/types";
import type { WsStatus } from "@/lib/ws";
import { DEFAULT_PERSONAS } from "@/lib/personas";

export const TYPING_TTL_MS = 4_000;

type ChannelMessages = {
  /** all messages (top-level + thread replies), ascending by createdAt */
  items: Message[];
  loaded: boolean;
  hasMore: boolean;
};

type WorkspaceState = {
  sessionId: string | null;
  session: Session | null;
  channels: Channel[];
  tasks: Task[];
  deadlines: Deadline[];
  warnings: Warning[];
  personas: Persona[];
  evaluation: Evaluation | null;
  messages: Record<string, ChannelMessages>;
  activeChannelId: string | null;
  unread: Record<string, number>;
  /** channelId -> personaId -> expiry epoch ms */
  typing: Record<string, Record<string, number>>;
  threadParentId: string | null;
  /** message to flash/scroll to after a jump */
  highlightMessageId: string | null;
  wsStatus: WsStatus;

  reset: (sessionId: string) => void;
  setPersonas: (p: Persona[]) => void;
  setDetails: (d: SessionDetails) => void;
  setChannelMessages: (channelId: string, msgs: Message[], opts: { limit: number; older?: boolean }) => void;
  setActiveChannel: (id: string) => void;
  openThread: (parentId: string | null) => void;
  setHighlight: (id: string | null) => void;
  setWsStatus: (s: WsStatus) => void;
  upsertMessage: (m: Message) => void;
  removeMessage: (channelId: string, id: string) => void;
  applyEvent: (ev: ServerEvent) => void;
  pruneTyping: () => void;
};

function sortByCreated(a: Message, b: Message) {
  return a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0;
}

function mergeMessages(existing: Message[], incoming: Message[]): Message[] {
  const map = new Map<string, Message>();
  for (const m of existing) map.set(m.id, m);
  for (const m of incoming) {
    const prev = map.get(m.id);
    // never let a stale REST snapshot clobber a message that is currently streaming
    if (prev?.streaming && !m.streaming && m.content.length < prev.content.length) continue;
    map.set(m.id, prev ? { ...prev, ...m } : m);
  }
  return [...map.values()].sort(sortByCreated);
}

function upsertById<T extends { id: string }>(list: T[], item: T): T[] {
  const i = list.findIndex((x) => x.id === item.id);
  if (i === -1) return [...list, item];
  const copy = list.slice();
  copy[i] = item;
  return copy;
}

const initial = {
  sessionId: null,
  session: null,
  channels: [],
  tasks: [],
  deadlines: [],
  warnings: [],
  evaluation: null,
  messages: {},
  activeChannelId: null,
  unread: {},
  typing: {},
  threadParentId: null,
  highlightMessageId: null,
  wsStatus: "idle" as WsStatus,
};

export const useWorkspace = create<WorkspaceState>((set, get) => ({
  ...initial,
  personas: DEFAULT_PERSONAS,

  reset: (sessionId) => set({ ...initial, sessionId }),
  setPersonas: (personas) => set({ personas }),

  setDetails: (d) =>
    set((s) => ({
      session: d.session,
      channels: d.channels,
      tasks: d.tasks,
      deadlines: d.deadlines,
      warnings: d.warnings,
      activeChannelId:
        s.activeChannelId && d.channels.some((c) => c.id === s.activeChannelId)
          ? s.activeChannelId
          : (d.channels.find((c) => c.kind === "CHANNEL" && c.name === "general") ?? d.channels[0])?.id ?? null,
    })),

  setChannelMessages: (channelId, msgs, { limit, older }) =>
    set((s) => {
      const prev = s.messages[channelId];
      const items = mergeMessages(prev?.items ?? [], msgs);
      const hasMore = older || !prev?.loaded ? msgs.length >= limit : prev.hasMore;
      return { messages: { ...s.messages, [channelId]: { items, loaded: true, hasMore } } };
    }),

  setActiveChannel: (id) =>
    set((s) => ({
      activeChannelId: id,
      unread: { ...s.unread, [id]: 0 },
      threadParentId: s.activeChannelId === id ? s.threadParentId : null,
    })),

  openThread: (parentId) => set({ threadParentId: parentId }),
  setHighlight: (id) => set({ highlightMessageId: id }),
  setWsStatus: (wsStatus) => set({ wsStatus }),

  upsertMessage: (m) =>
    set((s) => {
      const prev = s.messages[m.channelId] ?? { items: [], loaded: false, hasMore: false };
      return { messages: { ...s.messages, [m.channelId]: { ...prev, items: mergeMessages(prev.items, [m]) } } };
    }),

  removeMessage: (channelId, id) =>
    set((s) => {
      const prev = s.messages[channelId];
      if (!prev) return {};
      return { messages: { ...s.messages, [channelId]: { ...prev, items: prev.items.filter((m) => m.id !== id) } } };
    }),

  pruneTyping: () =>
    set((s) => {
      const now = Date.now();
      let changed = false;
      const typing: WorkspaceState["typing"] = {};
      for (const [ch, map] of Object.entries(s.typing)) {
        const next: Record<string, number> = {};
        for (const [pid, exp] of Object.entries(map)) {
          if (exp > now) next[pid] = exp;
          else changed = true;
        }
        if (Object.keys(next).length) typing[ch] = next;
      }
      return changed ? { typing } : {};
    }),

  applyEvent: (ev) => {
    const s = get();
    switch (ev.type) {
      case "pong":
      case "error":
        return;
      case "message.new": {
        const m = ev.data;
        if (s.sessionId && m.sessionId && m.sessionId !== s.sessionId) return;
        const exists = s.messages[m.channelId]?.items.some((x) => x.id === m.id);
        if (m.authorType === "USER") {
          // server echo of our own post may beat the POST response: drop the optimistic copy
          const temp = s.messages[m.channelId]?.items.find(
            (x) => x.pending && x.content === m.content && (x.parentId ?? null) === (m.parentId ?? null),
          );
          if (temp) s.removeMessage(m.channelId, temp.id);
        }
        s.upsertMessage(m);
        set((st) => {
          const patch: Partial<WorkspaceState> = {};
          if (!exists && m.authorType !== "USER" && st.activeChannelId !== m.channelId) {
            patch.unread = { ...st.unread, [m.channelId]: (st.unread[m.channelId] ?? 0) + 1 };
          }
          if (m.personaId && st.typing[m.channelId]?.[m.personaId]) {
            const map = { ...st.typing[m.channelId] };
            delete map[m.personaId];
            patch.typing = { ...st.typing, [m.channelId]: map };
          }
          return patch;
        });
        return;
      }
      case "message.delta": {
        const { messageId, channelId, delta } = ev.data;
        set((st) => {
          const ch = st.messages[channelId];
          if (!ch) return {};
          const idx = ch.items.findIndex((m) => m.id === messageId);
          if (idx === -1) return {};
          const items = ch.items.slice();
          const cur = items[idx]!;
          items[idx] = { ...cur, content: cur.content + delta, streaming: true };
          return { messages: { ...st.messages, [channelId]: { ...ch, items } } };
        });
        return;
      }
      case "message.done": {
        const m = { ...ev.data, streaming: false };
        const exists = s.messages[m.channelId]?.items.some((x) => x.id === m.id);
        // force-replace content (mergeMessages guards against shorter snapshots only for REST loads)
        set((st) => {
          const prev = st.messages[m.channelId] ?? { items: [], loaded: false, hasMore: false };
          const items = exists
            ? prev.items.map((x) => (x.id === m.id ? { ...x, ...m } : x))
            : [...prev.items, m].sort(sortByCreated);
          const patch: Partial<WorkspaceState> = {
            messages: { ...st.messages, [m.channelId]: { ...prev, items } },
          };
          if (!exists && m.authorType !== "USER" && st.activeChannelId !== m.channelId) {
            patch.unread = { ...st.unread, [m.channelId]: (st.unread[m.channelId] ?? 0) + 1 };
          }
          return patch;
        });
        return;
      }
      case "typing": {
        const { channelId, personaId } = ev.data;
        set((st) => ({
          typing: {
            ...st.typing,
            [channelId]: { ...(st.typing[channelId] ?? {}), [personaId]: Date.now() + TYPING_TTL_MS },
          },
        }));
        return;
      }
      case "task.new":
      case "task.updated":
        set((st) => ({ tasks: upsertById(st.tasks, ev.data) }));
        return;
      case "deadline.new":
      case "deadline.updated":
        set((st) => ({ deadlines: upsertById(st.deadlines, ev.data) }));
        return;
      case "warning.new":
        set((st) => ({
          warnings: upsertById(st.warnings, ev.data.warning),
          session: st.session ? { ...st.session, warnings: ev.data.totalWarnings } : st.session,
        }));
        return;
      case "session.updated":
        if (s.sessionId && ev.data.id !== s.sessionId) return;
        set({ session: ev.data });
        return;
      case "evaluation.updated":
        if (s.sessionId && ev.data.sessionId !== s.sessionId) return;
        set({ evaluation: ev.data });
        return;
    }
  },
}));
