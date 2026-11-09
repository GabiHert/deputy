import { useEffect, useState } from "react";
import * as A from "../wailsjs/go/app/App";
import { EventsOn } from "../wailsjs/runtime/runtime";

// Plain mirrors of the Go DTOs in internal/app/dto.go. Times are Unix ms.

export type WhatsApp = {
  state:
    | "starting"
    | "qr"
    | "pairing"
    | "connected"
    | "disconnected"
    | "error"
    | string;
  qr: string;
  qrExpiresAt: number;
  phone: string;
  pushName: string;
  since: number;
  error: string;
};
export type Claude = {
  found: boolean;
  path: string;
  version: string;
  error: string;
};
export type Status = {
  whatsapp: WhatsApp;
  claude: Claude;
  version: string;
  waiting: number;
};

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
  viaAll: boolean;
  skipped: boolean;
};

export type RepoStats = {
  exists: boolean;
  files: number;
  commits: number;
  lastLearned: number;
  remote: string;
};

export type Member = { jid: string; name: string; phone: string; users: string[]; me: boolean; admin: boolean };

export type MCPServer = { name: string; target: string; status: string };

export type Agent = {
  id: string;
  name: string;
  enabled: boolean;
  repoPath: string;
  instructions: string;
  chats: string[];
  allDirect: boolean;
  except: string[];
  answerMe: boolean;
  mcp: string[];
  folderAccess: "" | "read" | "edit" | string;
  folders: string[];
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

export type RunStatus =
  "running" | "sent" | "waiting" | "discarded" | "quiet" | "learned" | "failed";
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
  request: string;
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

export type Defaults = {
  mode: string;
  maxPerHour: number;
  waitSeconds: number;
  prefix: string;
};
export type Settings = {
  defaults: Defaults;
  mediaRetentionDays: number;
  openAtLogin: boolean;
  notify: boolean;
  mcpEnabled: boolean;
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

export type UpdateRelease = { tag: string; name: string; notes: string; date: number; url: string };
export type UpdateStatus = {
  version: string;
  repo: string;
  latest: UpdateRelease | null;
  upToDate: boolean;
  hasToken: boolean;
  state: "idle" | "checking" | "downloading" | "verifying" | "installing" | "failed" | string;
  message: string;
  lines: string[];
  seq: number;
  installedApp: string;
};

export type MCPStatus = {
  enabled: boolean;
  running: boolean;
  url: string;
  error: string;
  command: string;
};

/* eslint-disable @typescript-eslint/no-explicit-any */
const arr = <T>(p: Promise<any>): Promise<T[]> =>
  p.then((v) => (v ?? []) as T[]);

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
  listMCPServers: (refresh = false) => arr<MCPServer>(A.ListMCPServers(refresh)),
  getUpdateStatus: () => A.GetUpdateStatus() as Promise<UpdateStatus>,
  checkForUpdate: () => A.CheckForUpdate() as Promise<UpdateStatus>,
  startUpdate: () => A.StartUpdate(),
  setUpdateToken: (token: string) => A.SetUpdateToken(token) as Promise<UpdateStatus>,
  getMCPStatus: () => A.GetMCPStatus() as Promise<MCPStatus>,
  setMCPEnabled: (on: boolean) => A.SetMCPEnabled(on) as Promise<MCPStatus>,
  addMCPToClaude: () => A.AddMCPToClaude(),
  rotateMCPToken: () => A.RotateMCPToken() as Promise<MCPStatus>,
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
    api
      .getStatus()
      .then((s) => alive && setStatus(s))
      .catch(() => {});
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
    api
      .listAgents()
      .then(setAgents)
      .catch(() => {});
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
export type QueryShape =
  "groups" | "runs" | "messages" | "cells" | "rows" | string;
export type QueryResult = { query: string; shape: QueryShape; items: any };
export type Group = { key: string; label: string; value: number };
export type Cell = { day: number; hour: number; count: number };
export type BoardMessage = Message & { chatJid: string; chatName: string };

export type Question = {
  id: string;
  label: string;
  kind: "choice" | "multi" | "input" | "confirm" | string;
  options: string[] | null;
  placeholder: string;
};
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
export type Board = {
  blocks: BoardBlock[] | null;
  layoutAt: number;
  chatId: string;
  building: boolean;
};

export type AssistantChat = {
  chatJid?: string;
  id: string;
  title: string;
  purpose: "general" | "new-agent" | "agent" | "board" | string;
  agentId: string;
  preview: string;
  createdAt: number;
  updatedAt: number;
  busy: boolean;
};

export type ChatRef = {
  jid: string;
  name: string;
  isGroup: boolean;
  members: number;
};
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
export type SettingChange = {
  field: string;
  label: string;
  from: string;
  to: string;
};
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
  message: MessageProposal | null;
};
export type MessageProposal = {
  chatJid: string;
  chatName: string;
  agentId: string;
  agentName: string;
  prefix: string;
  text: string;
  reason: string;
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
  dismissRun: (runId: string, dismissed: boolean) =>
    A.DismissRun(runId, dismissed),
  runBoardQuery: (query: string, params: Record<string, any>) =>
    A.RunBoardQuery(query, params) as Promise<QueryResult>,
  refreshBoardBlock: (id: string) => A.RefreshBoardBlock(id),
  changeBoard: (text: string) => A.ChangeBoard(text),
  resetBoard: () => A.ResetBoard(),
  listChats: () => arr<AssistantChat>(A.ListAssistantChats()),
  askAgentChat: (jid: string) => A.AskAgentChat(jid) as Promise<AssistantChat>,
  newChat: (purpose: string, agentId = "") =>
    A.NewAssistantChat(purpose, agentId) as Promise<AssistantChat>,
  messages: (chatId: string) =>
    arr<AssistantMessage>(A.AssistantMessages(chatId)),
  send: (chatId: string, text: string) => A.SendAssistant(chatId, text),
  stop: (chatId: string) => A.StopAssistant(chatId),
  rename: (chatId: string, title: string) =>
    A.RenameAssistantChat(chatId, title),
  remove: (chatId: string) => A.DeleteAssistantChat(chatId),
  answerCard: (messageId: string, answers: Record<string, string>) =>
    A.AnswerCard(messageId, answers),
  dismissCard: (messageId: string) => A.DismissCard(messageId),
  createAgentFromCard: (messageId: string, p: AgentProposal) =>
    A.CreateAgentFromCard(messageId, p as any) as Promise<Agent>,
  applyChangeCard: (messageId: string) => A.ApplyChangeCard(messageId),
  sendMessageCard: (messageId: string, text: string) =>
    A.SendMessageCard(messageId, text),
};

/** useAssistantChats keeps the saved Claude chats fresh from the "assistant" event. */
export function useAssistantChats(): [AssistantChat[], () => void] {
  const [chats, setChats] = useState<AssistantChat[]>([]);
  const reload = () => {
    api2
      .listChats()
      .then(setChats)
      .catch(() => {});
  };
  useEffect(() => {
    reload();
    return on("assistant", reload);
  }, []);
  return [chats, reload];
}

/** useChatMessages loads one chat's messages and refreshes them when the backend says it changed. */
export function useChatMessages(
  chatId: string,
): [AssistantMessage[], () => void] {
  const [msgs, setMsgs] = useState<AssistantMessage[]>([]);
  const reload = () => {
    if (!chatId) {
      setMsgs([]);
      return;
    }
    api2
      .messages(chatId)
      .then(setMsgs)
      .catch(() => {});
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

// ---- inbox: Positron as the owner's own WhatsApp client (internal/app/dto_inbox.go) ----

export type MessageKind =
  | "text"
  | "image"
  | "voice"
  | "audio"
  | "video"
  | "document"
  | "sticker"
  | "other";
export type MessageStatus =
  | ""
  | "pending"
  | "sent"
  | "delivered"
  | "read"
  | "played"
  | "failed";

export type InboxChat = {
  jid: string;
  name: string;
  /** one or two letters for an avatar placeholder ("#" for a bare number) */
  initials: string;
  isGroup: boolean;
  members: number;
  /** cached picture, "" until getAvatar fetched one (or there is none) */
  avatarUrl: string;
  lastAt: number;
  lastId: string;
  /** preview of the last message, without the sender */
  lastText: string;
  lastKind: MessageKind | "";
  /** who sent the last message in a group, "" for the owner */
  lastSender: string;
  lastFromMe: boolean;
  lastStatus: MessageStatus;
  unread: number;
  muted: boolean;
  mutedUntil: number;
  pinned: boolean;
  archived: boolean;
  agentId: string;
  agentName: string;
  /** disappearing-messages timer in seconds (86400 = 24 hours), 0 when off or not known yet */
  disappearing: number;
};

export type Quoted = {
  id: string;
  senderJid: string;
  /** "You" for the owner */
  senderName: string;
  fromMe: boolean;
  kind: MessageKind | "";
  snippet: string;
  /** the quoted message is stored: the UI can scroll to it */
  found: boolean;
};

export type Reaction = {
  emoji: string;
  count: number;
  /** names; "You" for the owner */
  senders: string[];
  /** the owner reacted with this emoji */
  fromMe: boolean;
};

export type Media = {
  mime: string;
  seconds: number;
  fileName: string;
  size: number;
  /** "" until inbox.getMedia downloaded it */
  url: string;
  /** small JPEG preview for photos, videos and documents, "" without */
  thumbUrl: string;
  /** inbox.getMedia can still fetch it from WhatsApp */
  downloadable: boolean;
};

export type ChatMessage = {
  id: string;
  chatJid: string;
  senderJid: string;
  /** "" for the owner */
  senderName: string;
  fromMe: boolean;
  fromAgent: boolean;
  agentName: string;
  /** typed by the owner in Positron */
  fromUi: boolean;
  at: number;
  kind: MessageKind;
  /** body or caption ("" when deleted) */
  text: string;
  transcript: string;
  mentions: string[];
  mentionsMe: boolean;
  quoted: Quoted | null;
  reactions: Reaction[];
  media: Media | null;
  /** the owner's messages only; in groups, the furthest receipt */
  status: MessageStatus;
  edited: boolean;
  /** deleted for everyone */
  revoked: boolean;
  /** came from the phone's history, not live */
  history: boolean;
  /** a view-once photo, video or voice note; media.downloadable says whether Positron can show it */
  viewOnce: boolean;
};

export type MessagePage = {
  /** newest first */
  messages: ChatMessage[];
  /** pass as `before` for the next (older) page; "" when nothing older is stored */
  cursor: string;
  /** false: call inbox.loadOlder(chat) to ask the phone for more */
  hasMore: boolean;
};

export type SearchHit = {
  chatJid: string;
  chatName: string;
  isGroup: boolean;
  message: ChatMessage;
};

export type Contact = {
  jid: string;
  name: string;
  pushName: string;
  /** "+5551999991234", "" when unknown */
  phone: string;
  initials: string;
  isGroup: boolean;
  /** Positron already has messages in this chat */
  hasChat: boolean;
};

export type SendText = {
  chat: string;
  text: string;
  /** reply to this message, "" for none */
  quotedId?: string;
  /** JIDs mentioned with @ in the text */
  mentions?: string[];
};

/** Give either path (from inbox.chooseFiles) or dataBase64 (dropped/pasted bytes) with fileName. */
export type SendFile = {
  chat: string;
  path?: string;
  dataBase64?: string;
  fileName?: string;
  /** "" = guess from the name and content */
  mime?: string;
  caption?: string;
  quotedId?: string;
  /** force how it is sent; "" picks from the file type */
  kind?: "" | "image" | "video" | "audio" | "document";
};

/** A recording from MediaRecorder; Positron converts it to Ogg Opus with ffmpeg when needed. */
export type SendVoice = {
  chat: string;
  dataBase64: string;
  /** e.g. "audio/webm;codecs=opus" or "audio/mp4" */
  mime: string;
  /** the recording's length; 0 = measure it */
  seconds?: number;
  quotedId?: string;
};

export type InboxFilter =
  | ""
  | "archived"
  | "unread"
  | "groups"
  | "direct"
  | "all";

export type MessageEvent = {
  chat: string;
  change: "new" | "update";
  message: ChatMessage;
};
export type UnreadEvent = {
  /** "" when many chats changed: reload the inbox */
  chat: string;
  unread: number;
  /** all chats that are not muted (the Dock badge) */
  total: number;
};
export type ReceiptEvent = {
  chat: string;
  ids: string[];
  status: "delivered" | "read" | "played";
};
export type PresenceEvent = {
  /** the chat it shows in; "" for a contact going online/offline */
  chat: string;
  jid: string;
  name: string;
  state: "composing" | "recording" | "paused" | "online" | "offline";
  lastSeen: number;
};
export type HistoryEvent = {
  chat: string;
  added: number;
  kind: "on-demand" | "initial" | "recent" | "full" | string;
  /** "" or why nothing came (e.g. the phone did not answer within 45 s) */
  error: string;
};

/** Event names for `on(...)`. The chat list itself changes on "chats" (reload inbox.list). */
export const inboxEvents = {
  /** MessageEvent: a message arrived, was sent, or changed */
  message: "wa:message",
  /** UnreadEvent */
  unread: "wa:unread",
  /** ReceiptEvent */
  receipt: "wa:receipt",
  /** PresenceEvent */
  presence: "wa:presence",
  /** HistoryEvent: older messages arrived (after loadOlder), or none came */
  history: "wa:history",
  /** string chat JID: the owner clicked a notification; open that chat */
  openChat: "open-chat",
  /** no payload: the chat list changed; reload inbox.list */
  chats: "chats",
  /** string view name from the menu bar, e.g. "inbox" */
  navigate: "navigate",
} as const;

export const inbox = {
  list: (filter: InboxFilter = "") => arr<InboxChat>(A.ListInbox(filter)),
  chat: (jid: string) => A.GetInboxChat(jid) as Promise<InboxChat>,
  /** newest first; before "" for the newest page, then the returned cursor */
  messages: (chat: string, before = "", limit = 50) =>
    A.ListMessages(chat, before, limit).then((p: any) => ({
      ...p,
      messages: p?.messages ?? [],
    })) as Promise<MessagePage>,
  message: (chat: string, id: string) =>
    A.GetMessage(chat, id) as Promise<ChatMessage>,
  markRead: (chat: string) => A.MarkChatRead(chat),
  /** the chat open and visible; "" when none or the window is hidden */
  setActiveChat: (jid: string) => A.SetActiveChat(jid),
  unreadTotal: () => A.GetUnreadTotal() as Promise<number>,
  /** downloads when needed; returns the file's URL */
  getMedia: (chat: string, id: string) => A.GetMedia(chat, id),
  openMedia: (chat: string, id: string) => A.OpenMedia(chat, id),
  sendText: (m: SendText) =>
    A.SendText({ quotedId: "", mentions: [], ...m } as any) as Promise<ChatMessage>,
  sendFile: (f: SendFile) =>
    A.SendFile({
      path: "",
      dataBase64: "",
      fileName: "",
      mime: "",
      caption: "",
      quotedId: "",
      kind: "",
      ...f,
    } as any) as Promise<ChatMessage>,
  sendVoice: (v: SendVoice) =>
    A.SendVoice({ seconds: 0, quotedId: "", ...v } as any) as Promise<ChatMessage>,
  /** emoji "" takes the owner's reaction away */
  react: (chat: string, id: string, emoji: string) =>
    A.React(chat, id, emoji) as Promise<ChatMessage>,
  deleteForEveryone: (chat: string, id: string) =>
    A.DeleteForEveryone(chat, id) as Promise<ChatMessage>,
  /** own text messages, first 15 minutes */
  edit: (chat: string, id: string, text: string) =>
    A.EditMessage(chat, id, text) as Promise<ChatMessage>,
  contacts: (query = "") => arr<Contact>(A.ListContacts(query)),
  /** checks the number is on WhatsApp; returns its (possibly empty) chat */
  startChat: (phone: string) => A.StartChat(phone) as Promise<InboxChat>,
  /** chat "" searches every chat */
  search: (query: string, chat = "", limit = 50) =>
    arr<SearchHit>(A.SearchMessages(query, chat, limit)),
  /** returns at once; watch inboxEvents.history */
  loadOlder: (chat: string) => A.LoadOlder(chat),
  setTyping: (chat: string, on: boolean) => A.SetTyping(chat, on),
  /** "" when there is no picture */
  avatar: (jid: string) => A.GetAvatar(jid),
  members: (chat: string) => arr<Member>(A.GroupMembers(chat)),
  chooseFiles: () => arr<string>(A.ChooseFiles()),
};
