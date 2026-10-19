"use client";

import { ChevronDown, Hash, Home, SquarePen } from "lucide-react";
import Link from "next/link";
import { PersonaAvatar } from "@/components/persona-avatar";
import { CHANNEL_ORDER } from "@/lib/personas";
import type { Channel } from "@/lib/types";
import { cn } from "@/lib/utils";
import { useWorkspace } from "@/store/workspace";
import { dmPersona } from "./helpers";

function UnreadPill({ n }: { n: number }) {
  if (!n) return null;
  return (
    <span className="ml-auto min-w-[18px] rounded-full bg-[#cd2553] px-1.5 text-center text-[11px] leading-[18px] font-bold text-white tabular-nums">
      {n > 99 ? "99+" : n}
    </span>
  );
}

export function Sidebar({ username }: { username: string }) {
  const channels = useWorkspace((s) => s.channels);
  const personas = useWorkspace((s) => s.personas);
  const activeId = useWorkspace((s) => s.activeChannelId);
  const unread = useWorkspace((s) => s.unread);
  const typing = useWorkspace((s) => s.typing);
  const setActive = useWorkspace((s) => s.setActiveChannel);

  const chans = channels
    .filter((c) => c.kind === "CHANNEL")
    .sort((a, b) => {
      const ia = CHANNEL_ORDER.indexOf(a.name);
      const ib = CHANNEL_ORDER.indexOf(b.name);
      return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib) || a.name.localeCompare(b.name);
    });
  const dms = channels
    .filter((c) => c.kind === "DM")
    .sort((a, b) => {
      const ia = personas.findIndex((p) => p.id === a.personaId);
      const ib = personas.findIndex((p) => p.id === b.personaId);
      return ia - ib;
    });

  const item = (c: Channel, content: React.ReactNode) => {
    const isActive = c.id === activeId;
    const n = unread[c.id] ?? 0;
    return (
      <li key={c.id}>
        <button
          type="button"
          onClick={() => setActive(c.id)}
          className={cn(
            "flex h-7 w-full items-center gap-2 rounded-md px-2.5 text-left text-[15px] text-[#cfc3cf] transition-colors",
            isActive ? "bg-[#1164a3] text-white" : "hover:bg-[#350d36]",
            n > 0 && !isActive && "font-bold text-white",
          )}
        >
          {content}
          <UnreadPill n={isActive ? 0 : n} />
        </button>
      </li>
    );
  };

  return (
    <nav className="flex h-full w-[260px] shrink-0 flex-col bg-[#3F0E40] text-[#cfc3cf] max-lg:w-[220px]">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-[#5d2c5d] px-4">
        <button type="button" className="flex min-w-0 items-center gap-1 text-[18px] font-black text-white">
          <span className="truncate">WorkLingo</span>
          <ChevronDown className="size-4 shrink-0" />
        </button>
        <span className="flex size-8 items-center justify-center rounded-full bg-white text-[#3F0E40]" title="New message (disabled)">
          <SquarePen className="size-4" />
        </span>
      </div>
      <div className="sidebar-scroll flex-1 overflow-y-auto py-3">
        <ul className="px-2">
          <li>
            <Link
              href="/"
              className="flex h-7 items-center gap-2 rounded-md px-2.5 text-[15px] hover:bg-[#350d36]"
            >
              <Home className="size-4" /> Dashboard
            </Link>
          </li>
        </ul>
        <div className="mt-4 px-2">
          <div className="flex h-7 items-center gap-1 px-2.5 text-[15px] text-[#cfc3cf]">
            <ChevronDown className="size-3.5" /> Channels
          </div>
          <ul>
            {chans.map((c) =>
              item(
                c,
                <>
                  <Hash className="size-4 shrink-0 opacity-80" />
                  <span className="truncate">{c.name}</span>
                </>,
              ),
            )}
          </ul>
        </div>
        <div className="mt-4 px-2">
          <div className="flex h-7 items-center gap-1 px-2.5 text-[15px] text-[#cfc3cf]">
            <ChevronDown className="size-3.5" /> Direct messages
          </div>
          <ul>
            {dms.map((c) => {
              const p = dmPersona(c, personas);
              const isTyping = !!(p && typing[c.id]?.[p.id]);
              return item(
                c,
                <>
                  <span className="relative shrink-0">
                    <PersonaAvatar persona={p} size={20} className="rounded" />
                    <span className="absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full border-2 border-[#3F0E40] bg-[#2bac76]" />
                  </span>
                  <span className="truncate">{p?.displayName ?? c.name}</span>
                  {isTyping && <span className="text-xs font-normal italic opacity-70">typing…</span>}
                </>,
              );
            })}
          </ul>
        </div>
      </div>
      <div className="flex h-12 shrink-0 items-center gap-2 border-t border-[#5d2c5d] px-4 text-sm">
        <PersonaAvatar persona={{ displayName: username, avatarColor: "#1264A3" }} size={24} className="rounded" />
        <span className="truncate font-bold text-white">{username}</span>
        <span className="size-2 rounded-full bg-[#2bac76]" />
      </div>
    </nav>
  );
}
