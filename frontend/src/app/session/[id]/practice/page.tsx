"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Logo } from "@/components/logo";
import { api, errorMessage } from "@/lib/api";
import { MOCK } from "@/lib/config";
import { restoreDraft, submissionFor, type PracticeDraft } from "@/lib/practice";
import type { PracticeQueue } from "@/lib/types";
import { useRequireAuth } from "@/hooks/use-require-auth";

export default function PracticePage() {
  const { id } = useParams<{ id: string }>();
  const { status, user } = useRequireAuth();
  const [queue, setQueue] = useState<PracticeQueue | null>(null);
  const [draft, setDraft] = useState<PracticeDraft>({ answers: {}, selected: [], pending: null });
  const [revealed, setRevealed] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const storageKey = `worklingo.practice.${user?.id}.${id}`;

  useEffect(() => {
    if (status !== "authed" || !user) return;
    let alive = true;
    let saved: PracticeDraft;
    try { saved = restoreDraft(localStorage.getItem(storageKey)); } catch { saved = restoreDraft(null); }
    void api.practice(id).then((loaded) => {
      if (!alive) return;
      const answers = { ...saved.answers };
      for (const batch of loaded.pendingBatches) { for (const answer of batch.answers) { if (answers[answer.exerciseId] === undefined) answers[answer.exerciseId] = answer.answer; } }
      for (const item of loaded.items) {
        const latest = item.attempts.at(-1);
        if (answers[item.id] === undefined && latest) answers[item.id] = latest.answer;
      }
      const selected = saved.selected.length ? saved.selected.filter((key) => loaded.items.some((i) => i.id === key)).slice(0, 5) : loaded.items.slice(0, 5).map((i) => i.id);
      setDraft({ ...saved, answers, selected, pending: saved.pending ?? loaded.pendingBatches[0] ?? null }); setQueue(loaded); setError(null);
    }).catch((e) => { if (alive) setError(errorMessage(e)); });
    return () => { alive = false; };
  }, [id, status, storageKey, user]);

  function updateDraft(next: PracticeDraft) {
    setDraft(next);
    try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* The current page still retains the draft. */ }
  }
  const selected = queue?.items.filter((item) => draft.selected.includes(item.id)) ?? [];
  const processing = (queue?.pendingBatches.length ?? 0) > 0;
  const valid = !processing && selected.length >= 1 && selected.length <= 5 && selected.every((item) => {
    const text = (draft.answers[item.id] ?? "").trim(); return text.length > 0 && [...text].length <= 4000;
  });

  async function refresh() {
    setBusy(true); setError(null);
    try { setQueue(await api.practice(id)); } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  }

  async function submit(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault(); if (!valid || busy) return;
    const answers = selected.map((item) => ({ exerciseId: item.id, answer: draft.answers[item.id].trim() }));
    const submission = submissionFor(answers, draft.pending);
    updateDraft({ ...draft, pending: submission }); setBusy(true); setError(null); setNotice(null);
    try {
      const result = await api.submitPractice(id, submission);
      updateDraft({ ...draft, pending: null });
      setQueue((current) => current ? { ...current, items: current.items.map((item) => {
        const attempt = result.attempts.find((a) => a.exerciseId === item.id);
        return attempt ? { ...item, attempts: [...item.attempts.filter((a) => a.attemptedAt !== attempt.attemptedAt), attempt] } : item;
      }) } : current);
      setNotice(result.attempts.some((a) => a.verdict === "UNAVAILABLE") ? "Feedback is unavailable. Your answers are saved. Submit again when ready to retry." : "Feedback saved. Review your answers below.");
    } catch (e) {
      setError(`${errorMessage(e)} Your drafts are kept. Submit unchanged answers again to check the same request without starting another assessment.`);
      try { setQueue(await api.practice(id)); } catch { /* Keep the draft and original request ID. */ }
    } finally { setBusy(false); }
  }

  return <div className="min-h-screen bg-[#f8f8f8] text-[#1d1c1d]">
    <header className="flex h-14 items-center justify-between bg-[#3F0E40] px-4 text-white sm:px-6">
      <Link href="/" className="flex items-center gap-2 font-black"><Logo className="size-7" />WorkLingo</Link>
      <Link href={`/session/${id}/report`} className="rounded px-3 py-2 text-sm hover:bg-white/10">Back to report</Link>
    </header>
    <main className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <h1 className="text-2xl font-black">Rewrite practice</h1>
      <p className="mt-2 text-sm text-[#616061]">Rewrite each full message, preserving its meaning and a professional tone. Select 1–5 exercises per submission. Equivalent wording is welcome.</p>
      {MOCK && <p className="mt-2 text-sm text-amber-800">Demo mode uses sample answer fixtures. Real assessments accept equivalent wording through semantic review.</p>}
      {error && <p role="alert" className="mt-4 rounded border border-red-200 bg-red-50 p-3 text-sm text-red-800">{error}</p>}
      {notice && <p role="status" className="mt-4 rounded border bg-white p-3 text-sm">{notice}</p>}
      {queue && <div className="mt-4 flex flex-wrap items-center gap-3"><Button type="button" variant="outline" onClick={() => void refresh()} disabled={busy}>Reload saved feedback</Button>{processing && <p role="status" className="text-sm text-[#616061]">Your submission is processing. Reload feedback to check its result.</p>}</div>}
      {!queue && !error && <p className="mt-6" role="status">Loading practice…</p>}
      {queue?.sourceReviewUnavailable && <p className="mt-4 rounded border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">Some message reviews are unavailable. More exercises may appear after you <Link href={`/session/${id}/report`} className="font-bold underline">retry the report review</Link>.</p>}
      {queue && queue.items.length === 0 && <p className="mt-6 rounded border bg-white p-5">No rewrite exercises are available from reviewed messages yet. <Link href={`/session/${id}/report`} className="font-bold text-[#1264a3] underline">View your report</Link>.</p>}
      {queue && queue.items.length > 0 && <form onSubmit={submit} className="mt-6 space-y-5">
        {queue.items.map((item, index) => {
          const latest = item.attempts.at(-1);
          const showSample = revealed.includes(item.id);
          const checked = draft.selected.includes(item.id);
          return <section key={item.id} className="rounded-xl border bg-white p-4 sm:p-6">
            <label className="flex items-center gap-2 font-bold"><input type="checkbox" checked={checked} disabled={busy || processing || !checked && selected.length >= 5} onChange={(e) => updateDraft({ ...draft, selected: e.target.checked ? [...draft.selected, item.id] : draft.selected.filter((key) => key !== item.id) })} /> Exercise {index + 1} <span className="text-xs font-normal text-[#616061]">{item.category} · {item.channelLabel}</span></label>
            {item.context.length > 0 && <div className="mt-3 border-l-2 pl-3 text-sm text-[#616061]"><p className="font-semibold">Context</p>{item.context.map((text, n) => <p key={n} className="mt-1 whitespace-pre-wrap break-words">{text}</p>)}</div>}
            <p className="mt-4 text-xs font-bold text-[#616061]">Original message</p><p className="mt-1 whitespace-pre-wrap break-words">{item.original}</p>
            <p className="mt-2 text-sm text-[#616061]">Focus on: <span className="font-semibold">{item.target}</span></p>
            <label htmlFor={`answer-${item.id}`} className="mt-4 block text-sm font-semibold">Your full rewrite</label>
            <textarea id={`answer-${item.id}`} value={draft.answers[item.id] ?? ""} disabled={busy || processing} rows={3} onChange={(e) => updateDraft({ ...draft, answers: { ...draft.answers, [item.id]: e.target.value } })} className="mt-1 w-full rounded-md border border-[#b5b5b5] p-3 text-sm focus:outline-2 focus:outline-[#611f69]" />
            <p className="text-right text-xs text-[#616061]">{[...(draft.answers[item.id] ?? "")].length}/4000 characters</p>
            {latest && <div className={`mt-3 rounded border p-3 text-sm ${latest.verdict === "ACCEPTED" ? "border-green-200 bg-green-50" : latest.verdict === "UNAVAILABLE" ? "border-amber-200 bg-amber-50" : "bg-[#f8f8f8]"}`}>
              <p className="font-bold">{latest.verdict === "ACCEPTED" ? "Accepted" : latest.verdict === "NEEDS_WORK" ? "Needs another rewrite" : "Feedback unavailable"}</p><p className="mt-1 whitespace-pre-wrap">{latest.feedback}</p>
              <p className="mt-2 text-xs text-[#616061]">Saved {new Date(latest.attemptedAt).toLocaleString()}</p>
            </div>}
            <button type="button" onClick={() => setRevealed(showSample ? revealed.filter((key) => key !== item.id) : [...revealed, item.id])} className="mt-3 text-sm font-semibold text-[#1264a3] underline">{showSample ? "Hide sample" : "Reveal sample and explanation"}</button>
            {showSample && <div className="mt-2 rounded bg-[#f4f9f6] p-3 text-sm"><p className="whitespace-pre-wrap break-words">{item.correction}</p><p className="mt-2 text-[#616061]">{item.explanation}</p></div>}
            {item.attempts.length > 0 && <details className="mt-3 text-sm"><summary className="cursor-pointer font-semibold">Saved attempts ({item.attempts.length})</summary><ol className="mt-2 space-y-3">{[...item.attempts].reverse().map((attempt, n) => <li key={n} className="rounded border p-3"><p className="whitespace-pre-wrap break-words">{attempt.answer}</p><p className="mt-1 font-semibold">{attempt.verdict === "ACCEPTED" ? "Accepted" : attempt.verdict === "NEEDS_WORK" ? "Needs another rewrite" : "Feedback unavailable"}</p><p>{attempt.feedback}</p></li>)}</ol></details>}
          </section>;
        })}
        <div className="sticky bottom-0 flex items-center justify-between gap-4 rounded-xl border bg-white p-4 shadow-sm"><p className="text-sm">{selected.length}/5 selected</p><Button type="submit" disabled={!valid || busy}>{busy ? "Checking…" : draft.pending ? "Check saved submission" : "Check rewrites"}</Button></div>
      </form>}
    </main>
  </div>;
}
