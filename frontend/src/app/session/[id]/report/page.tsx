"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, Loader2, MessagesSquare, RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Logo } from "@/components/logo";
import { scoreColor } from "@/components/score";
import { api, errorMessage, getToken } from "@/lib/api";
import { durationBetween, formatDateTime } from "@/lib/format";
import type { CorrectionCategory, Evaluation, SessionDetails } from "@/lib/types";
import { WsClient } from "@/lib/ws";
import { cn } from "@/lib/utils";
import { useRequireAuth } from "@/hooks/use-require-auth";

const POLL_MS = 4000;

const CATEGORY_STYLE: Record<CorrectionCategory, string> = {
  GRAMMAR: "bg-[#e8f1fa] text-[#1264a3] border-[#bcd7ef]",
  TONE: "bg-[#fdeef3] text-[#c01a4d] border-[#f3c4d2]",
  VOCABULARY: "bg-[#e5f4ee] text-[#007a5a] border-[#b5e0cf]",
};

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
  const done = evaluation?.status === "DONE";

  return (
    <div className="flex flex-1 flex-col bg-[#f8f8f8]">
      <header className="flex h-14 shrink-0 items-center justify-between bg-[#3F0E40] px-6 text-white">
        <Link href="/" className="flex items-center gap-2">
          <Logo className="size-7" />
          <span className="text-lg font-black tracking-tight">WorkLingo</span>
        </Link>
        <div className="flex items-center gap-2">
          <Link
            href={`/session/${id}`}
            className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-white/10"
          >
            <MessagesSquare className="size-4" /> View conversation
          </Link>
          <Link href="/" className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-white/10">
            <ArrowLeft className="size-4" /> Dashboard
          </Link>
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-8">
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
            <div className="mt-6 grid grid-cols-2 gap-4 md:grid-cols-4">
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
            <section className="mt-6 overflow-hidden rounded-xl border bg-white">
              <h2 className="px-6 pt-5 pb-3 text-lg font-bold text-[#1d1c1d]">
                Corrections <span className="text-sm font-normal text-[#616061]">({evaluation.corrections.length})</span>
              </h2>
              {evaluation.corrections.length === 0 ? (
                <p className="px-6 pb-6 text-sm text-[#616061]">No corrections. Excellent work!</p>
              ) : (
                <Table className="table-fixed">
                  <TableHeader>
                    <TableRow className="bg-[#f8f8f8]">
                      <TableHead className="w-[120px] pl-6">Category</TableHead>
                      <TableHead>You wrote</TableHead>
                      <TableHead>Better</TableHead>
                      <TableHead className="pr-6">Why</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {evaluation.corrections.map((c, i) => (
                      <TableRow key={i} className="align-top">
                        <TableCell className="pl-6">
                          <span
                            className={cn(
                              "inline-block rounded border px-1.5 py-0.5 text-[10px] font-black tracking-wide uppercase",
                              CATEGORY_STYLE[c.category] ?? "bg-gray-100",
                            )}
                          >
                            {c.category}
                          </span>
                        </TableCell>
                        <TableCell className="whitespace-normal break-words text-[#9b1b45] line-through decoration-[#e01e5a]/40">
                          {c.original}
                        </TableCell>
                        <TableCell className="whitespace-normal break-words font-semibold text-[#007a5a]">
                          {c.corrected}
                        </TableCell>
                        <TableCell className="whitespace-normal break-words pr-6 text-[#454245]">{c.explanation}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </section>
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
