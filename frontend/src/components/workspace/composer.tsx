"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { AtSign, Bold, Code, Italic, SendHorizontal } from "lucide-react";
import { PersonaAvatar } from "@/components/persona-avatar";
import type { Persona } from "@/lib/types";
import { cn } from "@/lib/utils";

type Props = {
  placeholder: string;
  personas: Persona[];
  disabled?: boolean;
  disabledReason?: string;
  onSend: (text: string) => Promise<void>;
  onTyping?: () => void;
  autoFocus?: boolean;
  /** changes to this key reset draft focus (e.g. channel switch) */
  focusKey?: string;
};

type MentionState = { start: number; query: string } | null;

function findMention(value: string, caret: number): MentionState {
  const upto = value.slice(0, caret);
  const m = /(^|\s)@([A-Za-z0-9_]*)$/.exec(upto);
  if (!m) return null;
  return { start: caret - m[2]!.length - 1, query: m[2]!.toLowerCase() };
}

export function Composer({ placeholder, personas, disabled, disabledReason, onSend, onTyping, autoFocus, focusKey }: Props) {
  const [value, setValue] = useState("");
  const [mention, setMention] = useState<MentionState>(null);
  const [sel, setSel] = useState(0);
  const [sending, setSending] = useState(false);
  const ref = useRef<HTMLTextAreaElement>(null);
  const lastTyping = useRef(0);

  const matches = useMemo(() => {
    if (!mention) return [];
    return personas.filter(
      (p) =>
        p.handle.toLowerCase().includes(mention.query) ||
        p.displayName.toLowerCase().includes(mention.query) ||
        p.id.includes(mention.query),
    );
  }, [mention, personas]);

  useEffect(() => {
    if (autoFocus && !disabled) ref.current?.focus();
  }, [autoFocus, disabled, focusKey]);

  // autosize
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 220)}px`;
  }, [value]);

  function updateMention(el: HTMLTextAreaElement) {
    const m = findMention(el.value, el.selectionStart ?? el.value.length);
    setMention(m);
    setSel(0);
  }

  function insertMention(p: Persona) {
    if (!mention) return;
    const el = ref.current;
    const caret = el?.selectionStart ?? value.length;
    const before = value.slice(0, mention.start);
    const after = value.slice(caret);
    const insert = `@${p.handle} `;
    const next = before + insert + after;
    setValue(next);
    setMention(null);
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      const pos = before.length + insert.length;
      el.setSelectionRange(pos, pos);
    });
  }

  function wrap(prefix: string, suffix = prefix) {
    const el = ref.current;
    if (!el) return;
    const s = el.selectionStart;
    const e = el.selectionEnd;
    const next = value.slice(0, s) + prefix + value.slice(s, e) + suffix + value.slice(e);
    setValue(next);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(s + prefix.length, e + prefix.length);
    });
  }

  async function submit() {
    const text = value.trim();
    if (!text || disabled || sending) return;
    setSending(true);
    setValue("");
    setMention(null);
    try {
      await onSend(text);
    } catch {
      setValue((v) => (v ? v : text)); // restore draft on failure
    } finally {
      setSending(false);
      requestAnimationFrame(() => ref.current?.focus());
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (mention && matches.length) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSel((i) => (i + 1) % matches.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSel((i) => (i - 1 + matches.length) % matches.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        insertMention(matches[sel] ?? matches[0]!);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMention(null);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  }

  return (
    <div className="relative px-5 pb-5">
      {mention && matches.length > 0 && (
        <div className="absolute bottom-full left-5 z-20 mb-1 w-80 overflow-hidden rounded-lg border border-[#ddd] bg-white py-1 shadow-lg">
          <div className="px-3 py-1 text-xs font-bold text-[#616061]">People</div>
          {matches.map((p, i) => (
            <button
              key={p.id}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault();
                insertMention(p);
              }}
              onMouseEnter={() => setSel(i)}
              className={cn(
                "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm",
                i === sel ? "bg-[#1164a3] text-white" : "text-[#1d1c1d]",
              )}
            >
              <PersonaAvatar persona={p} size={20} className="rounded" />
              <span className="font-bold">{p.displayName}</span>
              <span className={cn("truncate text-xs", i === sel ? "text-white/80" : "text-[#616061]")}>
                @{p.handle}
              </span>
            </button>
          ))}
        </div>
      )}
      <div
        className={cn(
          "rounded-lg border border-[#bbb] bg-white focus-within:border-[#868686] focus-within:shadow-[0_0_0_1px_#868686]",
          disabled && "bg-[#f8f8f8]",
        )}
      >
        <div className="flex items-center gap-0.5 border-b border-transparent px-1.5 pt-1.5 text-[#616061]">
          <ToolbarBtn label="Bold" onClick={() => wrap("**")} disabled={disabled}>
            <Bold className="size-4" />
          </ToolbarBtn>
          <ToolbarBtn label="Italic" onClick={() => wrap("_")} disabled={disabled}>
            <Italic className="size-4" />
          </ToolbarBtn>
          <ToolbarBtn label="Code" onClick={() => wrap("`")} disabled={disabled}>
            <Code className="size-4" />
          </ToolbarBtn>
          <ToolbarBtn
            label="Mention someone"
            disabled={disabled}
            onClick={() => {
              const el = ref.current;
              if (!el) return;
              const s = el.selectionStart;
              const needsSpace = s > 0 && !/\s/.test(value[s - 1] ?? "");
              const ins = (needsSpace ? " " : "") + "@";
              const next = value.slice(0, s) + ins + value.slice(el.selectionEnd);
              setValue(next);
              requestAnimationFrame(() => {
                el.focus();
                el.setSelectionRange(s + ins.length, s + ins.length);
                setMention({ start: s + ins.length - 1, query: "" });
              });
            }}
          >
            <AtSign className="size-4" />
          </ToolbarBtn>
        </div>
        <textarea
          ref={ref}
          rows={1}
          value={value}
          disabled={disabled}
          placeholder={disabled ? (disabledReason ?? placeholder) : placeholder}
          onChange={(e) => {
            setValue(e.target.value);
            updateMention(e.target);
            const now = Date.now();
            if (onTyping && now - lastTyping.current > 2500 && e.target.value) {
              lastTyping.current = now;
              onTyping();
            }
          }}
          onKeyDown={onKeyDown}
          onClick={(e) => updateMention(e.currentTarget)}
          onBlur={() => setTimeout(() => setMention(null), 100)}
          className="block max-h-[220px] min-h-[40px] w-full resize-none bg-transparent px-3 py-2 text-[15px] leading-[22px] text-[#1d1c1d] outline-none placeholder:text-[#868686] disabled:cursor-not-allowed"
          aria-label={placeholder}
        />
        <div className="flex items-center justify-between px-2 pb-1.5">
          <span className="text-[11px] text-[#868686]">
            <b>Enter</b> to send · <b>Shift + Enter</b> for a new line · <b>@</b> to mention
          </span>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={disabled || !value.trim() || sending}
            aria-label="Send message"
            className={cn(
              "flex h-7 w-8 items-center justify-center rounded text-white transition-colors",
              value.trim() && !disabled ? "bg-[#007a5a] hover:bg-[#148567]" : "bg-transparent text-[#9a9a9a]",
            )}
          >
            <SendHorizontal className="size-4" />
          </button>
        </div>
      </div>
    </div>
  );
}

function ToolbarBtn({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className="flex size-7 items-center justify-center rounded hover:bg-[#f0f0f0] hover:text-[#1d1c1d] disabled:opacity-40"
    >
      {children}
    </button>
  );
}
