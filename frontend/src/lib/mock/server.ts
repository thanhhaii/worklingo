/**
 * In-browser fake backend used when NEXT_PUBLIC_MOCK=1.
 * Implements the REST + WS contract from docs/CONTRACT.md with canned persona behaviour:
 * streamed replies, a surprise task (+deadline), a short @mention deadline that expires
 * into a warning + SYSTEM message, session expiry and a fake evaluation.
 * State is persisted to localStorage so page reloads keep working.
 */
import type { HttpRequest, HttpResponse, Transport } from "../api";
import type { SocketLike } from "../ws";
import type {
  Channel,
  Deadline,
  Evaluation,
  Message,
  MessageIssue,
  MessageReview,
  ServerEvent,
  Session,
  TalkingPoint,
  Task,
  TaskReview,
  User,
  Warning,
  WorkContext,
} from "../types";
import { DEFAULT_PERSONAS, CHANNEL_ORDER } from "../personas";

const STORE_KEY = "worklingo.mockdb.v1";
const SESSION_MINUTES = 60;
const RESPONSE_TIMEOUT_MS = 5 * 60_000;
/** The @mention deadline in mock mode is shortened so you can watch it expire. */
const MOCK_SHORT_DEADLINE_MS = 75_000;

type DB = {
  users: (User & { password: string })[];
  sessions: (Session & { userId: string; scenarioDone: string[] })[];
  channels: Channel[];
  messages: Message[];
  tasks: Task[];
  deadlines: Deadline[];
  warnings: Warning[];
  evaluations: Evaluation[];
  /** ids of tasks whose single follow-up was already sent */
  followUps: string[];
};

const emptyDb = (): DB => ({
  users: [],
  sessions: [],
  channels: [],
  messages: [],
  tasks: [],
  deadlines: [],
  warnings: [],
  evaluations: [],
  followUps: [],
});

const db: DB = load();

function load(): DB {
  if (typeof window === "undefined") return emptyDb();
  try {
    const raw = window.localStorage.getItem(STORE_KEY);
    if (raw) return { ...emptyDb(), ...(JSON.parse(raw) as DB) };
  } catch {
    // ignore
  }
  return emptyDb();
}

let saveTimer: ReturnType<typeof setTimeout> | null = null;
function save() {
  if (typeof window === "undefined") return;
  if (saveTimer) return;
  saveTimer = setTimeout(() => {
    saveTimer = null;
    try {
      // don't persist in-flight streaming state
      const snapshot: DB = { ...db, messages: db.messages.map((m) => ({ ...m, streaming: undefined })) };
      window.localStorage.setItem(STORE_KEY, JSON.stringify(snapshot));
    } catch {
      // quota etc.
    }
  }, 200);
}

const uuid = () =>
  typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
      });
const nowIso = () => new Date().toISOString();
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function ok(body: unknown, status = 200): HttpResponse {
  return { status, body };
}
function err(status: number, code: string, message: string): HttpResponse {
  return { status, body: { error: { code, message } } };
}

function publicUser(u: User & { password: string }): User {
  return { id: u.id, username: u.username, email: u.email, createdAt: u.createdAt };
}
function publicSession(s: DB["sessions"][number]): Session {
  return {
    id: s.id,
    mode: s.mode,
    status: s.status,
    startedAt: s.startedAt,
    expiresAt: s.expiresAt,
    endedAt: s.endedAt,
    warnings: s.warnings,
    workContext: s.workContext ?? null,
  };
}

const MOCK_WORK_CONTEXT: WorkContext = {
  project: "HR Portal",
  projectSummary: "Cổng nhân sự nội bộ để quản lý nhân viên, nghỉ phép và chấm công cho khách hàng của công ty.",
  feature: "Tính năng quản lý user (danh sách, tạo mới, phân quyền)",
  progress: "Xong khoảng 70%: API và UI danh sách user đã xong, còn phần phân quyền",
  blocker: "Đang chờ team DevOps cấp quyền truy cập môi trường staging để test phân quyền",
  notes: [
    "Demo cho khách hàng vào thứ Sáu tuần này",
    "Minh (backend) đang nghỉ phép đến thứ Tư",
    "Stack: Go, PostgreSQL, Next.js",
  ],
};

function makePoints(contents: string[]): TalkingPoint[] {
  return contents.map((content, position) => ({
    id: uuid(),
    position,
    content,
    status: "PENDING" as const,
    reason: "",
  }));
}

function userFromToken(token: string | null) {
  if (!token || !token.startsWith("mock.")) return null;
  const id = token.slice(5);
  return db.users.find((u) => u.id === id) ?? null;
}

// ---------------------------------------------------------------- WS hub

const sockets = new Set<MockSocket>();

function emit(sessionId: string, ev: ServerEvent) {
  for (const s of sockets) {
    if (s.subscribed === sessionId && s.readyState === 1) s.deliver(ev);
  }
}

export class MockSocket implements SocketLike {
  readyState = 0;
  onopen: ((ev: unknown) => void) | null = null;
  onclose: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  subscribed: string | null = null;
  private user: ReturnType<typeof userFromToken>;

  constructor(url: string) {
    const token = new URL(url, "http://x").searchParams.get("token");
    this.user = userFromToken(token);
    setTimeout(() => {
      if (!this.user) {
        this.readyState = 3;
        this.onclose?.({ code: 4401 });
        return;
      }
      this.readyState = 1;
      sockets.add(this);
      this.onopen?.({});
    }, 60);
  }

  deliver(ev: ServerEvent) {
    // async like a real socket
    setTimeout(() => this.onmessage?.({ data: JSON.stringify(ev) }), 0);
  }

  send(data: string) {
    if (this.readyState !== 1) return;
    let ev: { type: string; data: Record<string, string> };
    try {
      ev = JSON.parse(data);
    } catch {
      return;
    }
    if (ev.type === "ping") this.deliver({ type: "pong", data: {} });
    else if (ev.type === "subscribe") {
      const s = db.sessions.find((x) => x.id === ev.data.sessionId && x.userId === this.user?.id);
      if (!s) {
        this.deliver({ type: "error", data: { code: "not_found", message: "session not found" } });
        return;
      }
      this.subscribed = s.id;
      ensureScheduler(s.id);
    }
  }

  close() {
    if (this.readyState === 3) return;
    this.readyState = 3;
    sockets.delete(this);
    setTimeout(() => this.onclose?.({ code: 1000 }), 0);
  }
}

// Expose a helper to simulate a dropped connection from the devtools console.
if (typeof window !== "undefined") {
  (window as unknown as { __mockDropWs?: () => void }).__mockDropWs = () => {
    for (const s of [...sockets]) {
      s.readyState = 3;
      sockets.delete(s);
      s.onclose?.({ code: 1006 });
    }
  };
}

// ---------------------------------------------------------------- domain helpers

function channelByName(sessionId: string, name: string) {
  return db.channels.find((c) => c.sessionId === sessionId && c.kind === "CHANNEL" && c.name === name)!;
}
function dmFor(sessionId: string, personaId: string) {
  return db.channels.find((c) => c.sessionId === sessionId && c.kind === "DM" && c.personaId === personaId)!;
}

function addMessage(m: Omit<Message, "id" | "createdAt">): Message {
  const msg: Message = { ...m, id: uuid(), createdAt: nowIso() };
  db.messages.push(msg);
  save();
  return msg;
}

function sessionActive(sessionId: string) {
  return db.sessions.find((s) => s.id === sessionId)?.status === "ACTIVE";
}

/** Stream a persona message: typing → message.new(streaming) → deltas → message.done */
async function streamPersona(
  sessionId: string,
  channelId: string,
  personaId: string,
  content: string,
  parentId: string | null = null,
): Promise<Message | null> {
  if (!sessionActive(sessionId)) return null;
  const owner = db.users.find((u) => u.id === db.sessions.find((x) => x.id === sessionId)?.userId);
  content = content.replaceAll("@you", `@${owner?.username ?? "you"}`);
  emit(sessionId, { type: "typing", data: { channelId, personaId } });
  await sleep(900 + Math.random() * 900);
  if (!sessionActive(sessionId)) return null;
  const msg = addMessage({
    sessionId,
    channelId,
    parentId,
    authorType: "PERSONA",
    userId: null,
    personaId,
    content: "",
  });
  emit(sessionId, { type: "message.new", data: { ...msg, streaming: true } });
  const chunks = content.match(/\S+\s*|\s+/g) ?? [content];
  for (const chunk of chunks) {
    await sleep(25 + Math.random() * 45);
    msg.content += chunk;
    emit(sessionId, { type: "message.delta", data: { messageId: msg.id, channelId, delta: chunk } });
  }
  msg.content = content;
  save();
  emit(sessionId, { type: "message.done", data: { ...msg, streaming: false } });
  return msg;
}

function createDeadline(sessionId: string, channelId: string, messageId: string, taskId: string | null, ms: number) {
  const d: Deadline = {
    id: uuid(),
    sessionId,
    channelId,
    messageId,
    taskId,
    status: "PENDING",
    deadlineAt: new Date(Date.now() + ms).toISOString(),
    createdAt: nowIso(),
    resolvedAt: null,
  };
  db.deadlines.push(d);
  save();
  emit(sessionId, { type: "deadline.new", data: d });
  scheduleDeadline(d);
  return d;
}

const deadlineTimers = new Map<string, ReturnType<typeof setTimeout>>();
function scheduleDeadline(d: Deadline) {
  if (deadlineTimers.has(d.id) || d.status !== "PENDING") return;
  const ms = Math.max(0, new Date(d.deadlineAt).getTime() - Date.now());
  deadlineTimers.set(
    d.id,
    setTimeout(() => {
      deadlineTimers.delete(d.id);
      expireDeadline(d.id);
    }, ms),
  );
}

function expireDeadline(id: string) {
  const d = db.deadlines.find((x) => x.id === id);
  if (!d || d.status !== "PENDING" || !sessionActive(d.sessionId)) return;
  d.status = "EXPIRED";
  d.resolvedAt = nowIso();
  const s = db.sessions.find((x) => x.id === d.sessionId)!;
  s.warnings += 1;
  const w: Warning = {
    id: uuid(),
    sessionId: s.id,
    deadlineId: d.id,
    reason: "No response within 5 minutes to a direct request.",
    createdAt: nowIso(),
  };
  db.warnings.push(w);
  save();
  emit(s.id, { type: "deadline.updated", data: d });
  emit(s.id, { type: "warning.new", data: { warning: w, totalWarnings: s.warnings } });
  emit(s.id, { type: "session.updated", data: publicSession(s) });
  const sys = addMessage({
    sessionId: s.id,
    channelId: d.channelId,
    parentId: null,
    authorType: "SYSTEM",
    userId: null,
    personaId: null,
    content: `**Ghosting warning (${s.warnings}/3):** you didn't respond to a teammate's request within 5 minutes. In a real workplace, acknowledge requests promptly, even if it's just _"On it, I'll get back to you by 3pm."_`,
  });
  emit(s.id, { type: "message.new", data: sys });
}

function resolveDeadlines(sessionId: string, channelId: string, content: string) {
  if (content.trim().split(/\s+/).length < 2) return; // "valid" reply heuristic
  for (const d of db.deadlines) {
    if (d.sessionId === sessionId && d.channelId === channelId && d.status === "PENDING") {
      d.status = "RESOLVED";
      d.resolvedAt = nowIso();
      clearTimeout(deadlineTimers.get(d.id));
      deadlineTimers.delete(d.id);
      emit(sessionId, { type: "deadline.updated", data: d });
      if (d.taskId) {
        const task = db.tasks.find((x) => x.id === d.taskId);
        if (task && task.status === "OPEN") {
          task.status = "ACKNOWLEDGED";
          emit(sessionId, { type: "task.updated", data: task });
        }
      }
    }
  }
  save();
}

/** Naive coverage: each valid reply covers the next pending point; one follow-up per task. */
function coverTalkingPoints(sessionId: string, ch: Channel, msg: Message): { followUp: Task | null } {
  let followUp: Task | null = null;
  if (msg.content.trim().split(/\s+/).length < 2) return { followUp };
  let candidates = db.tasks.filter(
    (t) =>
      t.sessionId === sessionId &&
      t.channelId === ch.id &&
      t.status !== "DONE" &&
      t.createdAt < msg.createdAt &&
      (t.talkingPoints ?? []).some((p) => p.status === "PENDING"),
  );
  if (msg.parentId) candidates = candidates.filter((t) => t.messageId === msg.parentId);
  else candidates = candidates.sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 2);
  for (const t of candidates) {
    const next = [...(t.talkingPoints ?? [])].sort((a, b) => a.position - b.position).find((p) => p.status === "PENDING");
    if (!next) continue;
    next.status = "COVERED";
    next.reason = "Bạn đã truyền đạt đúng ý này.";
    const pending = (t.talkingPoints ?? []).some((p) => p.status === "PENDING");
    if (!pending) t.status = "DONE";
    emit(sessionId, { type: "task.updated", data: t });
    if (pending && t.status === "ACKNOWLEDGED" && !db.followUps.includes(t.id) && !followUp) {
      db.followUps.push(t.id);
      followUp = t;
    }
  }
  save();
  return { followUp };
}

async function sendFollowUp(sessionId: string, ch: Channel, t: Task) {
  const msg = await streamPersona(
    sessionId,
    ch.id,
    t.personaId,
    "Thanks, that helps. One thing is still missing though: could you also cover the rest of what I asked for? A short reply here in the thread is fine.",
    t.messageId,
  );
  if (msg) createDeadline(sessionId, ch.id, msg.id, t.id, RESPONSE_TIMEOUT_MS);
}

// ---------------------------------------------------------------- scenario

type Step = { key: string; atMs: number; run: (sessionId: string) => Promise<void> | void };

const SCENARIO: Step[] = [
  {
    key: "greeting",
    atMs: 1_500,
    run: async (sid) => {
      await streamPersona(
        sid,
        channelByName(sid, "general").id,
        "sarah",
        "Good morning everyone! :wave: Welcome to the team, @you. Today we're finalising the **Q3 onboarding flow**.\n\nCould you introduce yourself briefly and tell us what you worked on before?",
      );
    },
  },
  {
    key: "alex-hello",
    atMs: 6_000,
    run: async (sid) => {
      await streamPersona(
        sid,
        channelByName(sid, "sprint-planning").id,
        "alex",
        "Morning. For this sprint I'd like us to agree on estimates for the login refactor. Here's the current endpoint:\n\n```go\nfunc (h *Handler) Login(w http.ResponseWriter, r *http.Request) {\n    var req LoginRequest\n    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {\n        http.Error(w, \"bad request\", 400)\n        return\n    }\n    // TODO: rate limiting\n}\n```\n\nThoughts on how long rate limiting would take?",
      );
    },
  },
  {
    key: "task-incident",
    atMs: 15_000,
    run: async (sid) => {
      const ch = channelByName(sid, "incident-room");
      const msg = await streamPersona(
        sid,
        ch.id,
        "alex",
        ":rotating_light: **P1 incident**: checkout API is returning `502` for ~8% of requests since 10:42 UTC.\n\n@you can you take point on the status update for stakeholders? Please post a short summary here: impact, current status, next update time.",
      );
      if (!msg) return;
      const task: Task = {
        id: uuid(),
        sessionId: sid,
        channelId: ch.id,
        messageId: msg.id,
        personaId: "alex",
        title: "Write stakeholder status update for checkout 502s",
        priority: "CRITICAL",
        status: "OPEN",
        createdAt: nowIso(),
        talkingPoints: makePoints([
          "Khoảng 8% request checkout bị lỗi 502 từ 10:42 UTC",
          "Đội đang điều tra, nghi do database connection pool bị đầy",
          "Sẽ cập nhật lại sau 30 phút",
        ]),
      };
      db.tasks.push(task);
      save();
      emit(sid, { type: "task.new", data: task });
      createDeadline(sid, ch.id, msg.id, task.id, RESPONSE_TIMEOUT_MS);
    },
  },
  {
    key: "task-progress",
    atMs: 40_000,
    run: async (sid) => {
      const ch = channelByName(sid, "general");
      const msg = await streamPersona(
        sid,
        ch.id,
        "sarah",
        "@you before the demo, could you give me a quick **progress update** on the user management feature? I need to know where we stand and whether anything is blocking you.",
      );
      if (!msg) return;
      const task: Task = {
        id: uuid(),
        sessionId: sid,
        channelId: ch.id,
        messageId: msg.id,
        personaId: "sarah",
        title: "Update Sarah on user management progress",
        priority: "NORMAL",
        status: "OPEN",
        createdAt: nowIso(),
        talkingPoints: makePoints([
          "Feature quản lý user đã xong khoảng 70%",
          "Đang bị chặn vì chờ quyền truy cập staging từ DevOps",
        ]),
      };
      db.tasks.push(task);
      save();
      emit(sid, { type: "task.new", data: task });
      createDeadline(sid, ch.id, msg.id, task.id, RESPONSE_TIMEOUT_MS);
    },
  },
  {
    key: "david-question",
    atMs: 25_000,
    run: async (sid) => {
      const ch = dmFor(sid, "david");
      const msg = await streamPersona(
        sid,
        ch.id,
        "david",
        "Hi there, hope you're well. Quick one: @you, could you confirm whether the **invoice export** feature will make it into Friday's release? Our finance team are rather keen to know.",
      );
      if (!msg) return;
      createDeadline(sid, ch.id, msg.id, null, MOCK_SHORT_DEADLINE_MS);
    },
  },
];

const schedulers = new Map<string, ReturnType<typeof setTimeout>[]>();

function ensureScheduler(sessionId: string) {
  if (schedulers.has(sessionId)) return;
  const s = db.sessions.find((x) => x.id === sessionId);
  if (!s || s.status !== "ACTIVE") return;
  const timers: ReturnType<typeof setTimeout>[] = [];
  const started = new Date(s.startedAt).getTime();
  for (const step of SCENARIO) {
    if (s.scenarioDone.includes(step.key)) continue;
    const delay = Math.max(0, started + step.atMs - Date.now());
    timers.push(
      setTimeout(() => {
        if (!sessionActive(sessionId) || s.scenarioDone.includes(step.key)) return;
        s.scenarioDone.push(step.key);
        save();
        void step.run(sessionId);
      }, delay),
    );
  }
  // session expiry
  timers.push(
    setTimeout(() => finishSession(sessionId), Math.max(0, new Date(s.expiresAt).getTime() - Date.now())),
  );
  for (const d of db.deadlines) if (d.sessionId === sessionId) scheduleDeadline(d);
  schedulers.set(sessionId, timers);
}

function finishSession(sessionId: string): Session | null {
  const s = db.sessions.find((x) => x.id === sessionId);
  if (!s) return null;
  if (s.status === "ACTIVE") {
    s.status = "COMPLETED";
    s.endedAt = nowIso();
    for (const t of schedulers.get(sessionId) ?? []) clearTimeout(t);
    schedulers.delete(sessionId);
    const ev: Evaluation = {
      id: uuid(),
      sessionId,
      status: "PENDING",
      grammar: 0,
      tone: 0,
      vocabulary: 0,
      overall: 0,
      summary: "",
      messageReviews: [],
      taskReviews: [],
      createdAt: nowIso(),
    };
    db.evaluations.push(ev);
    save();
    emit(sessionId, { type: "session.updated", data: publicSession(s) });
    emit(sessionId, { type: "evaluation.updated", data: ev });
    setTimeout(() => completeEvaluation(sessionId), 4_000);
  }
  return publicSession(s);
}

function completeEvaluation(sessionId: string) {
  const ev = db.evaluations.find((e) => e.sessionId === sessionId);
  const s = db.sessions.find((x) => x.id === sessionId);
  if (!ev || !s) return;
  const userMsgs = db.messages.filter((m) => m.sessionId === sessionId && m.authorType === "USER");
  const penalty = s.warnings * 5;
  ev.status = "DONE";
  ev.grammar = Math.max(0, 78 - penalty);
  ev.tone = Math.max(0, 84 - penalty * 2);
  ev.vocabulary = Math.max(0, 71 - penalty);
  ev.overall = Math.round((ev.grammar + ev.tone + ev.vocabulary) / 3);
  ev.summary =
    `You sent ${userMsgs.length} message(s) and received ${s.warnings} ghosting warning(s). ` +
    "Your tone was generally polite and collaborative. Focus on article usage (a/the) and on using more precise business vocabulary when giving status updates. " +
    "When a stakeholder asks a direct question, acknowledge it quickly before giving a full answer.";
  ev.messageReviews = userMsgs
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
    .map((m, idx): MessageReview => {
      const ch = db.channels.find((c) => c.id === m.channelId);
      const channelLabel =
        ch?.kind === "DM"
          ? `DM: ${DEFAULT_PERSONAS.find((p) => p.id === ch.personaId)?.displayName ?? ch.name}`
          : `#${ch?.name ?? "unknown"}`;
      const issues: MessageIssue[] = [];
      const words = m.content.split(/\s+/).filter(Boolean);
      if (idx % 3 !== 2 && words.length >= 2) {
        issues.push({
          original: words.slice(0, Math.min(3, words.length)).join(" "),
          corrected: "I'll send you an update tomorrow.",
          explanation: "Thiếu mạo từ 'an' trước 'update'; dùng dạng rút gọn nghe tự nhiên hơn trong chat.",
          category: "GRAMMAR",
          severity: idx % 2 === 0 ? "MAJOR" : "MINOR",
        });
      }
      if (idx % 3 === 1 && words.length >= 1) {
        issues.push({
          original: words[words.length - 1]!,
          corrected: "Could you let me know when you're free?",
          explanation: "Câu hỏi trực tiếp có thể nghe hơi cộc; nói mềm hơn để giữ giọng hợp tác.",
          category: "TONE",
          severity: "MINOR",
        });
      }
      return { messageId: m.id, channelLabel, content: m.content, createdAt: m.createdAt, issues };
    });
  ev.taskReviews = db.tasks
    .filter((t) => t.sessionId === sessionId)
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
    .map((t): TaskReview => {
      const replies = userMsgs.filter(
        (m) => m.channelId === t.channelId && m.createdAt > t.createdAt && (!m.parentId || m.parentId === t.messageId),
      );
      let ri = 0;
      return {
        taskId: t.id,
        title: t.title,
        personaId: t.personaId,
        status: t.status,
        talkingPoints: (t.talkingPoints ?? []).map((p) => {
          const covered = p.status === "COVERED";
          const quote = covered ? (replies[ri++]?.content ?? "") : (replies[replies.length - 1]?.content ?? "");
          return {
            talkingPointId: p.id,
            content: p.content,
            covered,
            learnerQuote: quote,
            reason: covered
              ? "Bạn đã truyền đạt đúng ý này."
              : "Bạn chưa nhắc đến nội dung này trong câu trả lời.",
            sample: "About 8% of checkout requests have failed with 502 errors since 10:42 UTC.",
          };
        }),
      };
    });
  save();
  emit(sessionId, { type: "evaluation.updated", data: ev });
}

// ---------------------------------------------------------------- persona replies

const REPLIES: Record<string, string[]> = {
  sarah: [
    "Thanks for that! That's really helpful. Could you put together a short list of **acceptance criteria** so the team can pick it up?",
    "Great point. Let's align on the scope first:\n\n1. Must-haves for Friday\n2. Nice-to-haves\n3. Out of scope\n\nWhat would you put in bucket 1?",
    "Perfect, I'll loop in design. Just a small tip: in status updates it helps to lead with the *outcome*, then the details.",
  ],
  alex: [
    "Makes sense. My rough estimate would be **3 story points** for the rate limiter, assuming we reuse the existing Redis instance.",
    "Agreed. Could you also double-check the error handling? Something like:\n\n```ts\nif (!res.ok) {\n  throw new ApiError(res.status, await res.text());\n}\n```",
    "Good, that's clear. Ship it behind a feature flag and we can roll back if anything looks off.",
  ],
  david: [
    "Splendid, thank you for clarifying. Would you mind sending a short written summary I can forward to our finance director?",
    "Right, I see. That's a bit disappointing, to be honest. Is there any chance of a partial release?",
    "Cheers, much appreciated. Speak soon.",
  ],
};
const replyIdx: Record<string, number> = { sarah: 0, alex: 0, david: 0 };

function pickResponders(ch: Channel, content: string): string[] {
  if (ch.kind === "DM") return ch.personaId ? [ch.personaId] : [];
  const mentioned = DEFAULT_PERSONAS.filter((p) => content.includes(`@${p.handle}`)).map((p) => p.id);
  if (mentioned.length) return mentioned;
  switch (ch.name) {
    case "general":
      return ["sarah"];
    case "sprint-planning":
    case "incident-room":
      return ["alex"];
    default:
      return Math.random() < 0.5 ? ["david"] : [];
  }
}

async function replyTo(sessionId: string, ch: Channel, userMsg: Message) {
  const responders = pickResponders(ch, userMsg.content);
  for (const pid of responders) {
    const list = REPLIES[pid]!;
    const text = list[replyIdx[pid]! % list.length]!;
    replyIdx[pid]!++;
    await streamPersona(sessionId, ch.id, pid, text, userMsg.parentId);
  }
}

// ---------------------------------------------------------------- REST router

async function handle(req: HttpRequest): Promise<HttpResponse> {
  const url = new URL(req.path, "http://mock");
  const p = url.pathname.split("/").filter(Boolean);
  const body = (req.body ?? {}) as Record<string, string>;
  const M = req.method;

  if (M === "GET" && url.pathname === "/health") return ok({ ok: true });

  if (M === "POST" && url.pathname === "/auth/register") {
    if (!body.username || !body.email || !body.password)
      return err(400, "validation", "username, email and password are required");
    if (body.password.length < 6) return err(400, "validation", "password must be at least 6 characters");
    if (db.users.some((u) => u.email.toLowerCase() === body.email.toLowerCase()))
      return err(409, "email_taken", "email already registered");
    const u = { id: uuid(), username: body.username, email: body.email, password: body.password, createdAt: nowIso() };
    db.users.push(u);
    save();
    return ok({ token: `mock.${u.id}`, user: publicUser(u) }, 201);
  }
  if (M === "POST" && url.pathname === "/auth/login") {
    const u = db.users.find((x) => x.email.toLowerCase() === (body.email ?? "").toLowerCase());
    if (!u || u.password !== body.password) return err(401, "invalid_credentials", "invalid email or password");
    return ok({ token: `mock.${u.id}`, user: publicUser(u) });
  }

  const user = userFromToken(req.token);
  if (!user) return err(401, "unauthorized", "missing or invalid token");

  if (M === "GET" && url.pathname === "/me") return ok(publicUser(user));
  if (M === "GET" && url.pathname === "/personas") return ok(DEFAULT_PERSONAS);

  if (p[0] === "sessions") {
    const mine = db.sessions.filter((s) => s.userId === user.id);
    if (p.length === 1 && M === "POST") {
      if (mine.some((s) => s.status === "ACTIVE")) return err(409, "session_active", "you already have an active session");
      const start = new Date();
      const s = {
        id: uuid(),
        userId: user.id,
        mode: "AI_SOLO" as const,
        status: "ACTIVE" as const,
        startedAt: start.toISOString(),
        expiresAt: new Date(start.getTime() + SESSION_MINUTES * 60_000).toISOString(),
        endedAt: null,
        warnings: 0,
        workContext: MOCK_WORK_CONTEXT,
        scenarioDone: [] as string[],
      };
      db.sessions.push(s);
      for (const name of CHANNEL_ORDER)
        db.channels.push({ id: uuid(), sessionId: s.id, kind: "CHANNEL", name, personaId: null });
      for (const per of DEFAULT_PERSONAS)
        db.channels.push({ id: uuid(), sessionId: s.id, kind: "DM", name: per.id, personaId: per.id });
      save();
      ensureScheduler(s.id);
      return ok(publicSession(s), 201);
    }
    if (p.length === 1 && M === "GET")
      return ok(
        [...mine].sort((a, b) => b.startedAt.localeCompare(a.startedAt)).map(publicSession),
      );
    if (p[1] === "active" && p.length === 2 && M === "GET") {
      const a = mine.find((s) => s.status === "ACTIVE");
      return a ? ok(publicSession(a)) : err(404, "not_found", "no active session");
    }
    const s = mine.find((x) => x.id === p[1]);
    if (!s) return err(404, "not_found", "session not found");
    if (p.length === 2 && M === "GET")
      return ok({
        session: publicSession(s),
        channels: db.channels.filter((c) => c.sessionId === s.id),
        tasks: db.tasks.filter((t) => t.sessionId === s.id),
        deadlines: db.deadlines.filter((d) => d.sessionId === s.id),
        warnings: db.warnings.filter((w) => w.sessionId === s.id),
      });
    if (p[2] === "end" && M === "POST") return ok(finishSession(s.id));
    if (p[2] === "evaluation" && M === "GET") {
      const ev = db.evaluations.find((e) => e.sessionId === s.id);
      return ev ? ok(ev) : err(404, "not_found", "evaluation not started");
    }
    if (p[2] === "channels" && p[4] === "messages") {
      const ch = db.channels.find((c) => c.id === p[3] && c.sessionId === s.id);
      if (!ch) return err(404, "not_found", "channel not found");
      if (M === "GET") {
        const before = url.searchParams.get("before");
        const limit = Math.min(200, Number(url.searchParams.get("limit") ?? 50) || 50);
        let list = db.messages.filter((m) => m.channelId === ch.id);
        if (before) list = list.filter((m) => m.createdAt < before);
        list = list.sort((a, b) => a.createdAt.localeCompare(b.createdAt)).slice(-limit);
        return ok(list.map((m) => ({ ...m, streaming: undefined })));
      }
      if (M === "POST") {
        if (s.status !== "ACTIVE") return err(409, "session_closed", "session is not active");
        const content = (body.content ?? "").trim();
        if (!content) return err(400, "validation", "content is required");
        if (body.parentId && !db.messages.some((m) => m.id === body.parentId && m.channelId === ch.id))
          return err(400, "validation", "parent message not found");
        const msg = addMessage({
          sessionId: s.id,
          channelId: ch.id,
          parentId: body.parentId || null,
          authorType: "USER",
          userId: user.id,
          personaId: null,
          content,
        });
        emit(s.id, { type: "message.new", data: msg });
        resolveDeadlines(s.id, ch.id, content);
        const { followUp } = coverTalkingPoints(s.id, ch, msg);
        if (followUp) void sendFollowUp(s.id, ch, followUp);
        else void replyTo(s.id, ch, msg);
        return ok(msg, 201);
      }
    }
  }
  return err(404, "not_found", `no route for ${M} ${url.pathname}`);
}

export const mockTransport: Transport = async (req) => {
  await sleep(80 + Math.random() * 120);
  const res = await handle(req);
  // deep clone so callers can't mutate the mock DB
  return { status: res.status, body: res.body === undefined ? null : JSON.parse(JSON.stringify(res.body)) };
};
