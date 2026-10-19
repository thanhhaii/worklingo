"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { ArrowRight, Clock, LogOut, Play, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Logo } from "@/components/logo";
import { PersonaAvatar } from "@/components/persona-avatar";
import { ScorePill } from "@/components/score";
import { ApiError, api, errorMessage } from "@/lib/api";
import { DEFAULT_PERSONAS } from "@/lib/personas";
import { durationBetween, formatCountdown, formatDateTime } from "@/lib/format";
import type { Evaluation, Persona, Session } from "@/lib/types";
import { useRequireAuth } from "@/hooks/use-require-auth";
import { useNow } from "@/hooks/use-now";
import { useAuth } from "@/store/auth";

export default function DashboardPage() {
  const router = useRouter();
  const { status, user } = useRequireAuth();
  const logout = useAuth((s) => s.logout);
  const now = useNow();
  const [active, setActive] = useState<Session | null>(null);
  const [history, setHistory] = useState<Session[] | null>(null);
  const [evals, setEvals] = useState<Record<string, Evaluation | null>>({});
  const [personas, setPersonas] = useState<Persona[]>(DEFAULT_PERSONAS);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  const load = useCallback(async () => {
    try {
      const [a, list] = await Promise.all([api.activeSession(), api.sessions()]);
      setLoadError(null);
      setActive(a);
      setHistory(list);
      api.personas().then(setPersonas).catch(() => {});
      // Session has no score field; fetch evaluations of finished sessions (most recent 20).
      const finished = list.filter((s) => s.status !== "ACTIVE").slice(0, 20);
      const results = await Promise.all(
        finished.map((s) =>
          api
            .evaluation(s.id)
            .then((e) => [s.id, e] as const)
            .catch(() => [s.id, null] as const),
        ),
      );
      setEvals(Object.fromEntries(results));
    } catch (e) {
      setLoadError(errorMessage(e));
    }
  }, []);

  useEffect(() => {
    if (status === "authed") void Promise.resolve().then(load);
  }, [status, load]);

  async function start() {
    setStarting(true);
    try {
      const s = await api.createSession();
      router.push(`/session/${s.id}`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        const a = await api.activeSession().catch(() => null);
        if (a) {
          toast.info("You already have an active session. Resuming it.");
          router.push(`/session/${a.id}`);
          return;
        }
      }
      toast.error(errorMessage(e));
      setStarting(false);
    }
  }

  if (status !== "authed") {
    return <div className="flex flex-1 items-center justify-center text-sm text-[#616061]">Loading…</div>;
  }

  const activeRemaining = active ? new Date(active.expiresAt).getTime() - now : 0;
  const pastSessions = (history ?? []).filter((s) => s.id !== active?.id);

  return (
    <div className="flex flex-1 flex-col bg-[#f8f8f8]">
      <header className="flex h-14 shrink-0 items-center justify-between bg-[#3F0E40] px-6 text-white">
        <div className="flex items-center gap-2">
          <Logo className="size-7" />
          <span className="text-lg font-black tracking-tight">WorkLingo</span>
        </div>
        <div className="flex items-center gap-3 text-sm">
          <span className="opacity-80">
            Signed in as <b>{user?.username}</b>
          </span>
          <Button
            size="sm"
            variant="ghost"
            className="text-white hover:bg-white/10 hover:text-white"
            onClick={() => {
              logout();
              router.replace("/login");
            }}
          >
            <LogOut /> Sign out
          </Button>
        </div>
      </header>

      <main className="mx-auto w-full max-w-4xl flex-1 px-6 py-10">
        <section className="rounded-xl border border-[#e1e1e1] bg-white p-8 shadow-sm">
          <h1 className="text-2xl font-bold text-[#1d1c1d]">Welcome back, {user?.username}</h1>
          <p className="mt-2 max-w-2xl text-[15px] text-[#454245]">
            Join a simulated workspace for 60 minutes. Your AI teammates will chat, ask questions and drop surprise
            tasks. Reply in clear, professional English within 5 minutes when someone needs you, or you&apos;ll get a
            ghosting warning.
          </p>
          <div className="mt-5 flex flex-wrap gap-4">
            {personas.map((p) => (
              <div key={p.id} className="flex items-center gap-2">
                <PersonaAvatar persona={p} size={32} />
                <div className="leading-tight">
                  <div className="text-sm font-bold text-[#1d1c1d]">{p.displayName}</div>
                  <div className="text-xs text-[#616061]">{p.role}</div>
                </div>
              </div>
            ))}
          </div>
          <div className="mt-6">
            {active ? (
              <div className="flex flex-wrap items-center gap-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4">
                <div className="flex-1">
                  <div className="flex items-center gap-2 font-bold text-emerald-900">
                    <span className="size-2 rounded-full bg-emerald-500" /> Session in progress
                  </div>
                  <div className="mt-1 flex items-center gap-3 text-sm text-emerald-900/80">
                    <span className="flex items-center gap-1">
                      <Clock className="size-3.5" /> {formatCountdown(activeRemaining)} left
                    </span>
                    <span className="flex items-center gap-1">
                      <TriangleAlert className="size-3.5" /> {active.warnings} warning(s)
                    </span>
                  </div>
                </div>
                <Button
                  className="h-10 bg-[#007a5a] px-5 font-bold text-white hover:bg-[#148567]"
                  onClick={() => router.push(`/session/${active.id}`)}
                >
                  Resume session <ArrowRight />
                </Button>
              </div>
            ) : (
              <Button
                disabled={starting || history === null}
                onClick={start}
                className="h-11 bg-[#007a5a] px-6 text-base font-bold text-white hover:bg-[#148567]"
              >
                <Play /> {starting ? "Setting up your workspace…" : "Start new session"}
              </Button>
            )}
          </div>
          {loadError && (
            <div className="mt-4 flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {loadError}
              <Button size="xs" variant="outline" onClick={() => void load()}>
                Retry
              </Button>
            </div>
          )}
        </section>

        <section className="mt-10">
          <h2 className="mb-3 text-lg font-bold text-[#1d1c1d]">History</h2>
          <div className="overflow-hidden rounded-xl border border-[#e1e1e1] bg-white">
            {history === null && !loadError && <div className="p-6 text-sm text-[#616061]">Loading…</div>}
            {history !== null && pastSessions.length === 0 && (
              <div className="p-6 text-sm text-[#616061]">No past sessions yet. Start your first one above.</div>
            )}
            {pastSessions.map((s) => {
              const ev = evals[s.id];
              return (
                <Link
                  key={s.id}
                  href={`/session/${s.id}/report`}
                  className="flex items-center gap-4 border-b border-[#eee] px-5 py-3 last:border-0 hover:bg-[#f8f8f8]"
                >
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-bold text-[#1d1c1d]">{formatDateTime(s.startedAt)}</div>
                    <div className="text-xs text-[#616061]">
                      {durationBetween(s.startedAt, s.endedAt)} · {s.warnings} warning(s)
                    </div>
                  </div>
                  <Badge variant={s.status === "COMPLETED" ? "secondary" : "outline"}>{s.status}</Badge>
                  <div className="flex w-56 items-center justify-end gap-2 text-xs text-[#616061]">
                    {ev?.status === "DONE" ? (
                      <>
                        <span className="hidden sm:inline">G {ev.grammar} · T {ev.tone} · V {ev.vocabulary}</span>
                        <ScorePill value={ev.overall} />
                      </>
                    ) : ev?.status === "PENDING" ? (
                      <span>Evaluating…</span>
                    ) : ev?.status === "FAILED" ? (
                      <span className="text-red-600">Evaluation failed</span>
                    ) : (
                      <span>–</span>
                    )}
                  </div>
                  <ArrowRight className="size-4 text-[#9a9a9a]" />
                </Link>
              );
            })}
          </div>
        </section>
      </main>
    </div>
  );
}
