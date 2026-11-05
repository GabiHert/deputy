import { useEffect, useState } from "react";
import * as A from "../wailsjs/go/app/App";
import { EventsOn } from "../wailsjs/runtime/runtime";

// Plain mirrors of the Go DTOs in internal/app/dto.go. Times are Unix ms.

export type WhatsApp = {
  state: "starting" | "qr" | "pairing" | "connected" | "disconnected" | "error" | string;
  qr: string;
  qrExpiresAt: number;
  phone: string;
  pushName: string;
  since: number;
  error: string;
};
export type Claude = { found: boolean; path: string; version: string; error: string };
export type Status = { whatsapp: WhatsApp; claude: Claude; version: string; waiting: number };

export type Chat = {
  jid: string;
  name: string;
  isGroup: boolean;
  members: number;
  lastText: string;
  lastAt: number;
  agentId: string;
  agentName: string;
  mode: string;
};

export type RepoStats = { exists: boolean; files: number; commits: number; lastLearned: number; remote: string };

export type Agent = {
  id: string;
  name: string;
  enabled: boolean;
  repoPath: string;
  instructions: string;
  chats: string[];
  mode: "auto" | "approve" | "never" | string;
  trigger: "all" | "mention" | string;
  waitSeconds: number;
  maxPerHour: number;
  prefix: string;
  push: boolean;
  readImages: boolean;
  transcribeVoice: boolean;
  sendImages: boolean;
  voiceReplies: boolean;
  web: boolean;
  bash: boolean;
  model: string;
  language: string;
  stats: RepoStats;
};

export type Message = {
  id: string;
  at: number;
  senderName: string;
  fromMe: boolean;
  kind: string;
  text: string;
  mediaUrl: string;
  seconds: number;
  transcript: string;
  mentionsMe: boolean;
};
export type Attachment = { path: string; url: string; bytes: number };

export type RunStatus = "running" | "sent" | "waiting" | "discarded" | "quiet" | "learned" | "failed";
export type Run = {
  id: string;
  at: number;
  agentId: string;
  agentName: string;
  chatJid: string;
  chatName: string;
  isGroup: boolean;
  status: RunStatus | string;
  incoming: Message[] | null;
  reply: string;
  images: Attachment[] | null;
  voice: boolean;
  note: string;
  commit: string;
  files: string[] | null;
  error: string;
  sentAt: number;
};

export type Stats = {
  sent: number;
  waiting: number;
  quiet: number;
  learned: number;
  failed: number;
  photos: number;
  voice: number;
  imagesSent: number;
};

export type Defaults = { mode: string; maxPerHour: number; waitSeconds: number; prefix: string };
export type Settings = {
  defaults: Defaults;
  mediaRetentionDays: number;
  openAtLogin: boolean;
  notify: boolean;
  mediaDir: string;
  mediaBytes: number;
  mediaFiles: number;
};
export type SpeechModel = {
  language: string;
  label: string;
  installed: boolean;
  sizeBytes: number;
  downloading: boolean;
  done: number;
  total: number;
  phase: string;
  error: string;
};

/* eslint-disable @typescript-eslint/no-explicit-any */
const arr = <T,>(p: Promise<any>): Promise<T[]> => p.then((v) => (v ?? []) as T[]);

export const api = {
  getStatus: () => A.GetStatus() as Promise<Status>,
  linkWithPhone: (phone: string) => A.LinkWithPhone(phone),
  logout: () => A.Logout(),
  listChats: () => arr<Chat>(A.ListChats()),
  assignChat: (jid: string, agentId: string) => A.AssignChat(jid, agentId),
  listAgents: () => arr<Agent>(A.ListAgents()),
  getAgent: (id: string) => A.GetAgent(id) as Promise<Agent>,
  newAgent: () => A.NewAgent() as Promise<Agent>,
  saveAgent: (a: Agent) => A.SaveAgent(a as any) as Promise<Agent>,
  deleteAgent: (id: string) => A.DeleteAgent(id),
  chooseFolder: () => A.ChooseFolder(),
  openRepoInClaude: (id: string) => A.OpenRepoInClaude(id),
  listActivity: (filter: string) => arr<Run>(A.ListActivity(filter)),
  todayStats: () => A.TodayStats() as Promise<Stats>,
  sendDraft: (runId: string, text: string) => A.SendDraft(runId, text),
  discardDraft: (runId: string) => A.DiscardDraft(runId),
  retryRun: (runId: string) => A.RetryRun(runId),
  getSettings: () => A.GetSettings() as Promise<Settings>,
  saveSettings: (s: Settings) => A.SaveSettings(s as any),
  listSpeechModels: () => arr<SpeechModel>(A.ListSpeechModels()),
  downloadSpeechModel: (lang: string) => A.DownloadSpeechModel(lang),
  cancelSpeechDownload: (lang: string) => A.CancelSpeechDownload(lang),
  showMediaFolder: () => A.ShowMediaFolder(),
  recheckClaude: () => A.RecheckClaude() as Promise<Status>,
};

/** on subscribes to a backend event; returns the unsubscribe function. */
export function on(event: string, cb: (...data: any[]) => void): () => void {
  try {
    return EventsOn(event, cb);
  } catch {
    return () => {};
  }
}

export function errText(e: unknown): string {
  if (!e) return "Something went wrong.";
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  return String(e);
}

/** useStatus keeps the app status fresh from the "status" event. */
export function useStatus(): [Status | null, (s: Status) => void] {
  const [status, setStatus] = useState<Status | null>(null);
  useEffect(() => {
    let alive = true;
    api.getStatus().then((s) => alive && setStatus(s)).catch(() => {});
    const off = on("status", (s: Status) => setStatus(s));
    return () => {
      alive = false;
      off();
    };
  }, []);
  return [status, setStatus];
}

/** useAgents keeps the agent list fresh from the "agents" event. */
export function useAgents(): [Agent[], () => void] {
  const [agents, setAgents] = useState<Agent[]>([]);
  const reload = () => {
    api.listAgents().then(setAgents).catch(() => {});
  };
  useEffect(() => {
    reload();
    return on("agents", reload);
  }, []);
  return [agents, reload];
}

// ---------- v2: board and Claude chats (internal/app/dto_v2.go) ----------

export type BoardSource = { query: string; params: Record<string, any> | null };
export type BoardColumn = { key: string; label: string; numeric: boolean };
export type QueryShape = "groups" | "runs" | "messages" | "cells" | "rows" | string;
export type QueryResult = { query: string; shape: QueryShape; items: any };
export type Group = { key: string; label: string; value: number };
export type Cell = { day: number; hour: number; count: number };
export type BoardMessage = Message & { chatJid: string; chatName: string };

export type Question = { id: string; label: string; kind: "choice" | "multi" | "input" | "confirm" | string; options: string[] | null; placeholder: string };
export type Form = { title: string; questions: Question[] | null };

export type BoardBlock = {
  id: string;
  type: string;
  title: string;
  span: number;
  source: BoardSource | null;
  columns: BoardColumn[] | null;
  text: string;
  html: string;
  js: string;
  height: number;
  agentId: string;
  prompt: string;
  schedule: string;
  form: Form | null;
  data: QueryResult | null;
  agent: Agent | null;
  error: string;
  textAt: number;
  refreshing: boolean;
};
export type Board = { blocks: BoardBlock[] | null; layoutAt: number; chatId: string; building: boolean };

export type AssistantChat = {
  id: string;
  title: string;
  purpose: "general" | "new-agent" | "agent" | "board" | string;
  agentId: string;
  preview: string;
  createdAt: number;
  updatedAt: number;
  busy: boolean;
};

export type ChatRef = { jid: string; name: string; isGroup: boolean; members: number };
export type RepoFile = { path: string; content: string; summary: string };
export type AgentProposal = {
  name: string;
  chats: ChatRef[] | null;
  mode: string;
  trigger: string;
  maxPerHour: number;
  readImages: boolean;
  transcribeVoice: boolean;
  voiceReplies: boolean;
  sendImages: boolean;
  language: string;
  repoPath: string;
  instructions: string;
  knowledge: RepoFile[] | null;
  createdAgentId: string;
};
export type SettingChange = { field: string; label: string; from: string; to: string };
export type ChangeProposal = {
  agentId: string;
  agentName: string;
  summary: string;
  oldInstructions: string;
  instructions: string;
  knowledge: RepoFile[] | null;
  settings: SettingChange[] | null;
  commit: string;
};
export type Card = {
  kind: "ask" | "agent" | "change" | string;
  status: "open" | "done" | "dismissed" | string;
  result: string;
  ask: Form | null;
  answers: Record<string, string> | null;
  agent: AgentProposal | null;
  change: ChangeProposal | null;
};
export type AssistantMessage = {
  id: string;
  chatId: string;
  role: "user" | "assistant" | "progress" | "card" | "note" | "error" | string;
  text: string;
  at: number;
  card: Card | null;
};

export const api2 = {
  getBoard: () => A.GetBoard() as Promise<Board>,
  runBoardQuery: (query: string, params: Record<string, any>) => A.RunBoardQuery(query, params) as Promise<QueryResult>,
  refreshBoardBlock: (id: string) => A.RefreshBoardBlock(id),
  changeBoard: (text: string) => A.ChangeBoard(text),
  resetBoard: () => A.ResetBoard(),
  listChats: () => arr<AssistantChat>(A.ListAssistantChats()),
  newChat: (purpose: string, agentId = "") => A.NewAssistantChat(purpose, agentId) as Promise<AssistantChat>,
  messages: (chatId: string) => arr<AssistantMessage>(A.AssistantMessages(chatId)),
  send: (chatId: string, text: string) => A.SendAssistant(chatId, text),
  stop: (chatId: string) => A.StopAssistant(chatId),
  rename: (chatId: string, title: string) => A.RenameAssistantChat(chatId, title),
  remove: (chatId: string) => A.DeleteAssistantChat(chatId),
  answerCard: (messageId: string, answers: Record<string, string>) => A.AnswerCard(messageId, answers),
  dismissCard: (messageId: string) => A.DismissCard(messageId),
  createAgentFromCard: (messageId: string, p: AgentProposal) => A.CreateAgentFromCard(messageId, p as any) as Promise<Agent>,
  applyChangeCard: (messageId: string) => A.ApplyChangeCard(messageId),
};

/** useAssistantChats keeps the saved Claude chats fresh from the "assistant" event. */
export function useAssistantChats(): [AssistantChat[], () => void] {
  const [chats, setChats] = useState<AssistantChat[]>([]);
  const reload = () => {
    api2.listChats().then(setChats).catch(() => {});
  };
  useEffect(() => {
    reload();
    return on("assistant", reload);
  }, []);
  return [chats, reload];
}

/** useChatMessages loads one chat's messages and refreshes them when the backend says it changed. */
export function useChatMessages(chatId: string): [AssistantMessage[], () => void] {
  const [msgs, setMsgs] = useState<AssistantMessage[]>([]);
  const reload = () => {
    if (!chatId) {
      setMsgs([]);
      return;
    }
    api2.messages(chatId).then(setMsgs).catch(() => {});
  };
  useEffect(() => {
    reload();
    return on("assistant", (id: string) => {
      if (!id || id === chatId) reload();
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chatId]);
  return [msgs, reload];
}
