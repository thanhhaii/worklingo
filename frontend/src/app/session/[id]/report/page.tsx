"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, Check, Loader2, MessagesSquare, RefreshCw, TriangleAlert, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Logo } from "@/components/logo";
import { scoreColor } from "@/components/score";
import { api, errorMessage, getToken } from "@/lib/api";
import { durationBetween, formatDateTime, formatTime } from "@/lib/format";
import type { Evaluation, IssueCategory, MessageReview, SessionDetails, TaskReview } from "@/lib/types";
import { WsClient } from "@/lib/ws";
import { cn } from "@/lib/utils";
import { DEFAULT_PERSONAS } from "@/lib/personas";
import { useRequireAuth } from "@/hooks/use-require-auth";

const POLL_MS = 4000;

const CATEGORY_STYLE: Record<IssueCategory, string> = {
  GRAMMAR: "bg-[#e8f1fa] text-[#1264a3] border-[#bcd7ef]",
  TONE: "bg-[#fdeef3] text-[#c01a4d] border-[#f3c4d2]",
  VOCABULARY: "bg-[#e5f4ee] text-[#007a5a] border-[#b5e0cf]",
};

const STATUS_STYLE: Record<TaskReview["status"], string> = {
  OPEN: "bg-[#fff4e5] text-[#9a5b00]",
  ACKNOWLEDGED: "bg-[#e8f1fa] text-[#1264a3]",
  DONE: "bg-[#e5f4ee] text-[#007a5a]",
};

function TasksSection({ reviews, personaName }: { reviews: TaskReview[]; personaName: (id: string) => string }) {
  return (
    <section className="mt-6 rounded-xl border bg-white p-4 sm:p-6">
      <h2 className="text-lg font-bold text-[#1d1c1d]">
        Tasks <span className="text-sm font-normal text-[#616061]">({reviews.length})</span>
      </h2>
      {reviews.length === 0 ? (
        <p className="mt-2 text-sm text-[#616061]">No tasks were assigned in this session.</p>
      ) : (
        <div className="mt-3 flex flex-col gap-4">
          {reviews.map((t) => (
            <div key={t.taskId} className="rounded-lg border border-[#e1e1e1] p-3 sm:p-4">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-[15px] font-black text-[#1d1c1d]">{t.title}</span>
                <span className="text-xs text-[#616061]">{personaName(t.personaId)}</span>
                <span
                  className={cn(
                    "rounded px-1.5 py-px text-[10px] font-bold tracking-wide uppercase",
                    STATUS_STYLE[t.status],
                  )}
                >
                  {t.status === "ACKNOWLEDGED" ? "Acknowledged" : t.status === "OPEN" ? "Open" : "Done"}
                </span>
              </div>
              <ul className="mt-3 flex flex-col gap-3">
                {(t.talkingPoints ?? []).map((p) => (
                  <li key={p.talkingPointId} className="flex gap-2 text-sm">
                    {p.covered ? (
                      <Check className="mt-0.5 size-4 shrink-0 text-[#007a5a]" aria-label="Covered" />
                    ) : (
                      <X className="mt-0.5 size-4 shrink-0 text-[#e01e5a]" aria-label="Missed" />
                    )}
                    <div className="min-w-0 flex-1 break-words">
                      <div className="font-semibold text-[#1d1c1d]">{p.content}</div>
                      {p.learnerQuote && (
                        <div className="mt-0.5 text-[#454245]">
                          <span className="text-xs font-bold text-[#616061]">You wrote: </span>
                          <span className="italic">&ldquo;{p.learnerQuote}&rdquo;</span>
                        </div>
                      )}
                      {p.reason && <div className="mt-0.5 text-[#616061]">{p.reason}</div>}
                      {p.sample && (
                        <div className="mt-1 rounded bg-[#f4f9f6] px-2 py-1 text-[#1d1c1d]">
                          <span className="text-xs font-bold text-[#007a5a]">Sample: </span>
                          {p.sample}
                        </div>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function MessageReviewSection({ reviews }: { reviews: MessageReview[] }) {
  const issueCount = reviews.reduce((n, r) => n + r.issues.length, 0);
  const majorCount = reviews.reduce((n, r) => n + r.issues.filter((i) => i.severity === "MAJOR").length, 0);
  return (
    <section className="mt-6 overflow-hidden rounded-xl border bg-white">
      <div className="px-4 pt-5 pb-3 sm:px-6">
        <h2 className="text-lg font-bold text-[#1d1c1d]">Message review</h2>
        <p className="mt-0.5 text-sm text-[#616061]">
          {reviews.length} message(s) · {issueCount} issue(s) ·{" "}
          <span className={cn(majorCount > 0 && "font-bold text-[#e01e5a]")}>{majorCount} major</span>
        </p>
      </div>
      {reviews.length === 0 ? (
        <p className="px-4 pb-6 text-sm text-[#616061] sm:px-6">You didn&apos;t send any messages.</p>
      ) : (
        <div className="border-t border-[#e1e1e1]">
          <div className="hidden bg-[#f8f8f8] text-xs font-bold tracking-wide text-[#616061] uppercase md:grid md:grid-cols-[2fr_3fr]">
            <div className="px-6 py-2">Message</div>
            <div className="px-4 py-2">Issues</div>
          </div>
          {reviews.map((r) => (
            <div key={r.messageId} className="grid border-b border-[#eee] last:border-b-0 md:grid-cols-[2fr_3fr]">
              <div className="min-w-0 px-4 pt-3 pb-1 sm:px-6 md:py-3">
                <div className="text-xs text-[#616061]">
                  <span className="font-bold">{r.channelLabel}</span> · {formatTime(r.createdAt)}
                </div>
                <p className="mt-1 text-sm break-words whitespace-pre-wrap text-[#1d1c1d]">{r.content}</p>
              </div>
              <div className="min-w-0 px-4 pt-1 pb-3 sm:px-6 md:px-4 md:py-3">
                {r.issues.length === 0 ? (
                  <div className="flex items-center gap-1.5 text-sm font-semibold text-[#007a5a]">
                    <Check className="size-4" /> No issues
                  </div>
                ) : (
                  <ul className="flex flex-col gap-3">
                    {r.issues.map((i, k) => (
                      <li key={k} className="text-sm">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span
                            className={cn(
                              "inline-block rounded border px-1.5 py-0.5 text-[10px] font-black tracking-wide uppercase",
                              CATEGORY_STYLE[i.category] ?? "bg-gray-100",
                            )}
                          >
                            {i.category}
                          </span>
                          <span
                            className={cn(
                              "text-[10px] font-black tracking-wide uppercase",
                              i.severity === "MAJOR" ? "text-[#e01e5a]" : "text-[#616061]",
                            )}
                          >
                            {i.severity}
                          </span>
                        </div>
                        <div className="mt-1 break-words">
                          <span className="text-[#9b1b45] line-through decoration-[#e01e5a]/40">{i.original}</span>
                          <span className="mx-1.5 text-[#616061]">&rarr;</span>
                          <span className="font-bold text-[#007a5a]">{i.corrected}</span>
                        </div>
                        <div className="mt-0.5 text-[#454245]">{i.explanation}</div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function ScoreCard({ label, value, big }: { label: string; value: number; big?: boolean }) {
  const v = Math.max(0, Math.min(100, Math.round(value)));
  return (
    <div className={cn("rounded-xl border bg-white p-5", big && "border-2")}>
      <div className="text-xs font-bold tracking-wide text-[#616061] uppercase">{label}</div>
      <div className="mt-1 flex items-baseline gap-1">
        <span className={cn("font-black tabular-nums", big ? "text-5xl" : "text-3xl", scoreColor(v).split(" ")[0])}>{v}</span>
        <span className="text-sm text-[#616061]">/100</span>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-[#eee]">
        <div
          className={cn("h-full rounded-full", v >= 80 ? "bg-emerald-500" : v >= 60 ? "bg-amber-500" : "bg-red-500")}
          style={{ width: `${v}%` }}
        />
      </div>
    </div>
  );
}

export default function ReportPage() {
  const { id } = useParams<{ id: string }>();
  const { status } = useRequireAuth();
  const [details, setDetails] = useState<SessionDetails | null>(null);
  const [evaluation, setEvaluation] = useState<Evaluation | null | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const evalRef = useRef<Evaluation | null | undefined>(undefined);

  useEffect(() => {
    evalRef.current = evaluation;
  }, [evaluation]);

  useEffect(() => {
    if (status !== "authed") return;
    let stopped = false;
    const fetchEval = async () => {
      try {
        const ev = await api.evaluation(id);
        if (!stopped) setEvaluation(ev);
      } catch (e) {
        if (!stopped) setError(errorMessage(e));
      }
    };
    api
      .session(id)
      .then((d) => !stopped && setDetails(d))
      .catch((e) => !stopped && setError(errorMessage(e)));
    void fetchEval();

    // poll while evaluation is not finished
    const poll = setInterval(() => {
      const cur = evalRef.current;
      if (cur && cur.status !== "PENDING") return;
      void fetchEval();
    }, POLL_MS);

    // also listen on WS for evaluation.updated
    const ws = new WsClient({
      getToken,
      onEvent: (ev) => {
        if (ev.type === "evaluation.updated" && ev.data.sessionId === id) setEvaluation(ev.data);
        if (ev.type === "session.updated" && ev.data.id === id)
          setDetails((d) => (d ? { ...d, session: ev.data } : d));
      },
    });
    ws.subscribe(id);
    ws.connect();
    return () => {
      stopped = true;
      clearInterval(poll);
      ws.close();
    };
  }, [id, status]);

  const session = details?.session;
  const personaName = (pid: string) => DEFAULT_PERSONAS.find((p) => p.id === pid)?.displayName ?? pid;
  const done = evaluation?.status === "DONE";

  return (
    <div className="flex flex-1 flex-col bg-[#f8f8f8]">
      <header className="flex h-14 shrink-0 items-center justify-between bg-[#3F0E40] px-4 text-white sm:px-6">
        <Link href="/" className="flex items-center gap-2">
          <Logo className="size-7" />
          <span className="text-lg font-black tracking-tight">WorkLingo</span>
        </Link>
        <div className="flex items-center gap-2">
          <Link
            href={`/session/${id}`}
            className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-white/10"
          >
            <MessagesSquare className="size-4" /> <span className="max-sm:hidden">View conversation</span>
          </Link>
          <Link href="/" className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-white/10">
            <ArrowLeft className="size-4" /> <span className="max-sm:hidden">Dashboard</span>
          </Link>
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6 sm:px-6 sm:py-8">
        <h1 className="text-2xl font-black text-[#1d1c1d]">Session report</h1>
        {session && (
          <p className="mt-1 text-sm text-[#616061]">
            {formatDateTime(session.startedAt)} · {durationBetween(session.startedAt, session.endedAt)} ·{" "}
            <span className={cn(session.warnings > 0 && "font-bold text-[#e01e5a]")}>
              {session.warnings} ghosting warning(s)
            </span>{" "}
            · {details?.tasks.length ?? 0} task(s)
          </p>
        )}
        {error && (
          <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>
        )}

        {session?.status === "ACTIVE" && (
          <div className="mt-6 rounded-xl border bg-white p-6 text-sm text-[#454245]">
            This session is still running.{" "}
            <Link href={`/session/${id}`} className="font-bold text-[#1264a3] hover:underline">
              Return to the workspace
            </Link>{" "}
            or end it to get your evaluation.
          </div>
        )}

        {session && session.status !== "ACTIVE" && (evaluation === null || evaluation?.status === "PENDING") && (
          <div className="mt-6 flex items-center gap-3 rounded-xl border bg-white p-6">
            <Loader2 className="size-6 animate-spin text-[#611f69]" />
            <div>
              <div className="font-bold text-[#1d1c1d]">Evaluating your conversation…</div>
              <div className="text-sm text-[#616061]">
                Our AI reviewer is checking grammar, tone and vocabulary. This usually takes under a minute.
              </div>
            </div>
          </div>
        )}

        {evaluation?.status === "FAILED" && (
          <div className="mt-6 flex items-center gap-3 rounded-xl border border-red-200 bg-red-50 p-6 text-red-800">
            <TriangleAlert className="size-6" />
            <div className="flex-1">
              <div className="font-bold">Evaluation failed</div>
              <div className="text-sm">Something went wrong while scoring this session.</div>
            </div>
            <Button variant="outline" onClick={() => api.evaluation(id).then(setEvaluation).catch(() => {})}>
              <RefreshCw /> Refresh
            </Button>
          </div>
        )}

        {done && evaluation && (
          <>
            <div className="mt-6 grid grid-cols-2 gap-3 sm:gap-4 md:grid-cols-4">
              <ScoreCard label="Overall" value={evaluation.overall} big />
              <ScoreCard label="Grammar" value={evaluation.grammar} />
              <ScoreCard label="Tone" value={evaluation.tone} />
              <ScoreCard label="Vocabulary" value={evaluation.vocabulary} />
            </div>
            <section className="mt-6 rounded-xl border bg-white p-6">
              <h2 className="text-lg font-bold text-[#1d1c1d]">Summary</h2>
              <p className="mt-2 text-[15px] leading-relaxed whitespace-pre-wrap text-[#454245]">
                {evaluation.summary || "No summary provided."}
              </p>
            </section>
            <TasksSection reviews={evaluation.taskReviews ?? []} personaName={personaName} />
            <MessageReviewSection reviews={evaluation.messageReviews ?? []} />
          </>
        )}
        {status === "authed" && !details && !error && (
          <div className="mt-10 flex justify-center text-[#616061]">
            <Loader2 className="size-5 animate-spin" />
          </div>
        )}
      </main>
    </div>
  );
}
