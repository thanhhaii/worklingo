"use client";

import { useCallback, useEffect, useRef } from "react";
import { toast } from "sonner";
import { api, errorMessage, getToken } from "@/lib/api";
import { WsClient } from "@/lib/ws";
import type { Message, ServerEvent } from "@/lib/types";
import { useWorkspace } from "@/store/workspace";

const PAGE = 50;

let wsSingleton: WsClient | null = null;
export function getWs() {
  return wsSingleton;
}

/**
 * Owns data loading + the WebSocket for a session page.
 * Returns helpers used by the workspace components.
 */
export function useSessionController(sessionId: string, enabled: boolean) {
  const loadDetails = useCallback(async () => {
    const d = await api.session(sessionId);
    useWorkspace.getState().setDetails(d);
    return d;
  }, [sessionId]);

  const loadChannel = useCallback(
    async (channelId: string) => {
      const msgs = await api.messages(sessionId, channelId, { limit: PAGE });
      useWorkspace.getState().setChannelMessages(channelId, msgs, { limit: PAGE });
    },
    [sessionId],
  );

  const loadOlder = useCallback(
    async (channelId: string) => {
      const ch = useWorkspace.getState().messages[channelId];
      const first = ch?.items[0];
      if (!first || !ch.hasMore) return 0;
      const msgs = await api.messages(sessionId, channelId, { before: first.createdAt, limit: PAGE });
      useWorkspace.getState().setChannelMessages(channelId, msgs, { limit: PAGE, older: true });
      return msgs.length;
    },
    [sessionId],
  );

  const sendMessage = useCallback(
    async (channelId: string, content: string, parentId: string | null) => {
      const st = useWorkspace.getState();
      const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const optimistic: Message = {
        id: tempId,
        sessionId,
        channelId,
        parentId,
        authorType: "USER",
        userId: null,
        personaId: null,
        content,
        createdAt: new Date().toISOString(),
        pending: true,
      };
      st.upsertMessage(optimistic);
      try {
        const real = await api.postMessage(sessionId, channelId, content, parentId);
        const s2 = useWorkspace.getState();
        s2.removeMessage(channelId, tempId);
        s2.upsertMessage(real); // no-op merge if the WS echo already added it (dedupe by id)
      } catch (e) {
        const s2 = useWorkspace.getState();
        s2.removeMessage(channelId, tempId);
        toast.error(`Message not sent: ${errorMessage(e)}`);
        throw e;
      }
    },
    [sessionId],
  );

  // keep latest loaders in refs for WS callbacks
  const loadersRef = useRef({ loadDetails, loadChannel });
  useEffect(() => {
    loadersRef.current = { loadDetails, loadChannel };
  }, [loadDetails, loadChannel]);

  useEffect(() => {
    if (!enabled) return;
    const store = useWorkspace.getState();
    if (store.sessionId !== sessionId) store.reset(sessionId);

    let cancelled = false;
    loadDetails().catch((e) => {
      if (!cancelled) toast.error(`Failed to load session: ${errorMessage(e)}`);
    });
    api
      .personas()
      .then((p) => {
        if (!cancelled && p?.length) useWorkspace.getState().setPersonas(p);
      })
      .catch(() => {});
    api
      .evaluation(sessionId)
      .then((ev) => {
        if (!cancelled && ev) useWorkspace.getState().applyEvent({ type: "evaluation.updated", data: ev });
      })
      .catch(() => {});

    const onEvent = (ev: ServerEvent) => {
      const st = useWorkspace.getState();
      st.applyEvent(ev);
      switch (ev.type) {
        case "task.new": {
          const persona = st.personas.find((p) => p.id === ev.data.personaId);
          const channel = st.channels.find((c) => c.id === ev.data.channelId);
          const where = channel ? (channel.kind === "CHANNEL" ? `#${channel.name}` : "a DM") : "";
          const fn = ev.data.priority === "CRITICAL" ? toast.error : toast.info;
          fn(`New task${ev.data.priority === "CRITICAL" ? " (CRITICAL)" : ""}: ${ev.data.title}`, {
            description: `${persona?.displayName ?? "A teammate"} in ${where}. Reply within 5 minutes.`,
            duration: 8000,
            action: channel
              ? {
                  label: "Open",
                  onClick: () => {
                    const s = useWorkspace.getState();
                    s.setActiveChannel(channel.id);
                    s.setHighlight(ev.data.messageId);
                  },
                }
              : undefined,
          });
          break;
        }
        case "warning.new":
          toast.warning(`Ghosting warning ${ev.data.totalWarnings}/3`, {
            description: ev.data.warning.reason,
            duration: 8000,
          });
          break;
        case "error":
          toast.error(ev.data.message || ev.data.code);
          break;
      }
    };

    const ws = new WsClient({
      getToken,
      onEvent,
      onStatus: (s) => useWorkspace.getState().setWsStatus(s),
      onReconnect: () => {
        // fill gaps missed while disconnected
        void loadersRef.current.loadDetails().catch(() => {});
        const active = useWorkspace.getState().activeChannelId;
        if (active) void loadersRef.current.loadChannel(active).catch(() => {});
      },
    });
    ws.subscribe(sessionId);
    ws.connect();
    wsSingleton = ws;

    const prune = setInterval(() => useWorkspace.getState().pruneTyping(), 1000);
    return () => {
      cancelled = true;
      clearInterval(prune);
      ws.close();
      if (wsSingleton === ws) wsSingleton = null;
    };
  }, [enabled, sessionId, loadDetails]);

  return { loadChannel, loadOlder, sendMessage, loadDetails };
}
