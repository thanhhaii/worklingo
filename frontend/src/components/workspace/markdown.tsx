"use client";

import { memo, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { cn } from "@/lib/utils";

type MdNode = { type: string; value?: string; children?: MdNode[]; data?: Record<string, unknown> };

const MENTION_RE = /@([A-Za-z0-9_][A-Za-z0-9_.-]*[A-Za-z0-9_]|[A-Za-z0-9_])/g;

/** remark plugin: turn `@handle` in text nodes into styled <span class="mention">. Skips code. */
function remarkMentions(options: { self?: string; known: string[] }) {
  const known = new Set(options.known.map((h) => h.toLowerCase()));
  const self = options.self?.toLowerCase();
  return (tree: MdNode) => {
    const walk = (node: MdNode) => {
      if (!node.children) return;
      const out: MdNode[] = [];
      for (const child of node.children) {
        if (child.type === "text" && child.value && child.value.includes("@")) {
          const text = child.value;
          let last = 0;
          for (const m of text.matchAll(MENTION_RE)) {
            const idx = m.index ?? 0;
            const prev = idx > 0 ? text[idx - 1]! : " ";
            const handle = m[1]!.toLowerCase();
            const isSelf = handle === self || handle === "you" || handle === "here" || handle === "channel";
            if (/[A-Za-z0-9_]/.test(prev) || (!known.has(handle) && !isSelf)) continue;
            if (idx > last) out.push({ type: "text", value: text.slice(last, idx) });
            out.push({
              type: "mention",
              data: {
                hName: "span",
                hProperties: { className: isSelf ? ["mention", "mention-me"] : ["mention"] },
                hChildren: [{ type: "text", value: m[0] }],
              },
            });
            last = idx + m[0].length;
          }
          if (last === 0) out.push(child);
          else if (last < text.length) out.push({ type: "text", value: text.slice(last) });
        } else {
          if (child.type !== "inlineCode" && child.type !== "code" && child.type !== "link") walk(child);
          out.push(child);
        }
      }
      node.children = out;
    };
    walk(tree);
  };
}

/** Convert a few Slack-style :emoji: shortcodes used by personas. */
const EMOJI: Record<string, string> = {
  wave: "👋",
  rotating_light: "🚨",
  white_check_mark: "✅",
  warning: "⚠️",
  tada: "🎉",
  eyes: "👀",
  rocket: "🚀",
  fire: "🔥",
  thumbsup: "👍",
  "+1": "👍",
  pray: "🙏",
  coffee: "☕",
  smile: "😄",
  slightly_smiling_face: "🙂",
  thinking_face: "🤔",
  x: "❌",
};
function replaceEmoji(s: string) {
  // avoid touching fenced/inline code: split on backtick segments
  return s
    .split(/(```[\s\S]*?(?:```|$)|`[^`\n]*`)/g)
    .map((part, i) => (i % 2 === 1 ? part : part.replace(/:([a-z0-9_+-]+):/g, (m, k: string) => EMOJI[k] ?? m)))
    .join("");
}

const components: Components = {
  a: ({ href, children, ...rest }) => (
    <a href={href} target="_blank" rel="noopener noreferrer" {...rest}>
      {children}
    </a>
  ),
  img: ({ alt }) => <span className="text-[#616061]">[image: {alt}]</span>,
};

function MarkdownInner({
  content,
  streaming,
  selfHandle,
  knownHandles,
  className,
}: {
  content: string;
  streaming?: boolean;
  selfHandle?: string;
  knownHandles: string[];
  className?: string;
}) {
  const remarkPlugins = useMemo(
    () => [remarkGfm, [remarkMentions, { self: selfHandle, known: knownHandles }] as [typeof remarkMentions, Parameters<typeof remarkMentions>[0]]],
    [selfHandle, knownHandles],
  );
  const text = useMemo(() => replaceEmoji(content), [content]);
  if (!text && streaming) return <div className={cn("msg-md streaming", className)}><p /></div>;
  return (
    <div className={cn("msg-md", streaming && "streaming", className)}>
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={streaming ? [] : [[rehypeHighlight, { detect: true, ignoreMissing: true }]]}
        components={components}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}

export const Markdown = memo(MarkdownInner);
