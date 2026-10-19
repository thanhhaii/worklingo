// Types mirror docs/CONTRACT.md exactly.

export type User = { id: string; username: string; email: string; createdAt: string };

export type Persona = {
  id: string;
  handle: string;
  displayName: string;
  role: string;
  avatarColor: string;
};

export type SessionStatus = "ACTIVE" | "COMPLETED" | "ABORTED";

export type Session = {
  id: string;
  mode: "AI_SOLO";
  status: SessionStatus;
  startedAt: string;
  expiresAt: string;
  endedAt: string | null;
  warnings: number;
};

export type Channel = {
  id: string;
  sessionId: string;
  kind: "CHANNEL" | "DM";
  name: string;
  personaId: string | null;
};

export type AuthorType = "USER" | "PERSONA" | "SYSTEM";

export type Message = {
  id: string;
  sessionId: string;
  channelId: string;
  parentId: string | null;
  authorType: AuthorType;
  userId: string | null;
  personaId: string | null;
  content: string;
  createdAt: string;
  streaming?: boolean;
  /** client-only: optimistic message not yet acknowledged by the server */
  pending?: boolean;
  /** client-only: optimistic message failed to send */
  failed?: boolean;
};

export type Task = {
  id: string;
  sessionId: string;
  channelId: string;
  messageId: string;
  personaId: string;
  title: string;
  description: string;
  priority: "NORMAL" | "CRITICAL";
  status: "OPEN" | "ACKNOWLEDGED" | "DONE";
  createdAt: string;
};

export type Deadline = {
  id: string;
  sessionId: string;
  channelId: string;
  messageId: string;
  taskId: string | null;
  status: "PENDING" | "RESOLVED" | "EXPIRED";
  deadlineAt: string;
  createdAt: string;
  resolvedAt: string | null;
};

export type Warning = {
  id: string;
  sessionId: string;
  deadlineId: string;
  reason: string;
  createdAt: string;
};

export type CorrectionCategory = "GRAMMAR" | "TONE" | "VOCABULARY";

export type Correction = {
  original: string;
  corrected: string;
  explanation: string;
  category: CorrectionCategory;
};

export type Evaluation = {
  id: string;
  sessionId: string;
  status: "PENDING" | "DONE" | "FAILED";
  grammar: number;
  tone: number;
  vocabulary: number;
  overall: number;
  summary: string;
  corrections: Correction[];
  createdAt: string;
};

export type AuthResponse = { token: string; user: User };

export type SessionDetails = {
  session: Session;
  channels: Channel[];
  tasks: Task[];
  deadlines: Deadline[];
  warnings: Warning[];
};

// ---- WebSocket ----

export type ClientEvent =
  | { type: "subscribe"; data: { sessionId: string } }
  | { type: "typing"; data: { sessionId: string; channelId: string } }
  | { type: "ping"; data: Record<string, never> };

export type ServerEvent =
  | { type: "pong"; data: Record<string, never> }
  | { type: "message.new"; data: Message }
  | { type: "message.delta"; data: { messageId: string; channelId: string; delta: string } }
  | { type: "message.done"; data: Message }
  | { type: "typing"; data: { channelId: string; personaId: string } }
  | { type: "task.new"; data: Task }
  | { type: "task.updated"; data: Task }
  | { type: "deadline.new"; data: Deadline }
  | { type: "deadline.updated"; data: Deadline }
  | { type: "warning.new"; data: { warning: Warning; totalWarnings: number } }
  | { type: "session.updated"; data: Session }
  | { type: "evaluation.updated"; data: Evaluation }
  | { type: "error"; data: { code: string; message: string } };
