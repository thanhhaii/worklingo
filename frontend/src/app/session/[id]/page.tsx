"use client";

import { useParams } from "next/navigation";
import { Workspace } from "@/components/workspace/workspace";
import { useRequireAuth } from "@/hooks/use-require-auth";

export default function SessionPage() {
  const { id } = useParams<{ id: string }>();
  const { status, user } = useRequireAuth();
  if (status !== "authed" || !user) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#3F0E40] text-sm text-white/80">
        Loading workspace…
      </div>
    );
  }
  return <Workspace key={id} sessionId={id} username={user.username} />;
}
