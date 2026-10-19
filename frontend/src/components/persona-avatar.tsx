import { initials } from "@/lib/personas";
import type { Persona } from "@/lib/types";
import { cn } from "@/lib/utils";

export function PersonaAvatar({
  persona,
  size = 36,
  className,
}: {
  persona: Pick<Persona, "displayName" | "avatarColor"> | null | undefined;
  size?: number;
  className?: string;
}) {
  const name = persona?.displayName ?? "?";
  return (
    <div
      aria-hidden
      className={cn("flex shrink-0 select-none items-center justify-center rounded-md font-bold text-white", className)}
      style={{
        width: size,
        height: size,
        background: persona?.avatarColor ?? "#616061",
        fontSize: Math.max(9, Math.round(size * 0.4)),
      }}
    >
      {initials(name)}
    </div>
  );
}

export function UserAvatar({ name, size = 36, className }: { name: string; size?: number; className?: string }) {
  return <PersonaAvatar persona={{ displayName: name, avatarColor: "#1264A3" }} size={size} className={className} />;
}
