import type { ChatMessage, InboxChat, MessageKind } from "../../api";
import { clock, duration } from "../../format";

/* Pure helpers for the inbox: times, day groups, previews. No React, no backend calls. */

const DAY = 86400000;

function startOfDay(ms: number): number {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** daysBetween counts calendar days from ms to now (0 = same day). */
export function daysBetween(ms: number, now: number): number {
  return Math.round((startOfDay(now) - startOfDay(ms)) / DAY);
}

/** listTime is the chat list's time: "14:03" today, "Yesterday", "Tuesday" this week, else "4 Nov" (with the year when it is not this year). */
export function listTime(ms: number, now: number = Date.now()): string {
  if (!ms) return "";
  const days = daysBetween(ms, now);
  if (days <= 0) return clock(ms);
  if (days === 1) return "Yesterday";
  const d = new Date(ms);
  if (days < 7) return d.toLocaleDateString("en", { weekday: "long" });
  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return d.toLocaleDateString("en-GB", sameYear ? { day: "numeric", month: "short" } : { day: "numeric", month: "short", year: "numeric" });
}

/** dayLabel names a day separator: "Today", "Yesterday", "Tuesday", "4 November" or "4 November 2024". */
export function dayLabel(ms: number, now: number = Date.now()): string {
  const days = daysBetween(ms, now);
  if (days <= 0) return "Today";
  if (days === 1) return "Yesterday";
  const d = new Date(ms);
  if (days < 7) return d.toLocaleDateString("en", { weekday: "long" });
  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return d.toLocaleDateString("en-GB", sameYear ? { day: "numeric", month: "long" } : { day: "numeric", month: "long", year: "numeric" });
}

/** lastSeen gives "last seen today at 14:03", "last seen yesterday at 09:10" or "last seen 4 Nov at 09:10". */
export function lastSeen(ms: number, now: number = Date.now()): string {
  if (!ms) return "";
  const days = daysBetween(ms, now);
  const at = clock(ms);
  if (days <= 0) return `last seen today at ${at}`;
  if (days === 1) return `last seen yesterday at ${at}`;
  return `last seen ${new Date(ms).toLocaleDateString("en-GB", { day: "numeric", month: "short" })} at ${at}`;
}

export type ThreadItem =
  | { type: "day"; key: string; label: string }
  | { type: "msg"; key: string; message: ChatMessage; /** same sender as the message before, same day */ continued: boolean };

/** groupByDay turns messages (oldest first) into day separators and messages. */
export function groupByDay(messages: ChatMessage[], now: number = Date.now()): ThreadItem[] {
  const out: ThreadItem[] = [];
  let day = -1;
  let prev: ChatMessage | null = null;
  for (const m of messages) {
    const d = startOfDay(m.at);
    if (d !== day) {
      day = d;
      prev = null;
      out.push({ type: "day", key: `day-${d}`, label: dayLabel(m.at, now) });
    }
    const continued = !!prev && prev.fromMe === m.fromMe && prev.senderJid === m.senderJid && m.at - prev.at < 5 * 60000;
    out.push({ type: "msg", key: m.id, message: m, continued });
    prev = m;
  }
  return out;
}

/** kindLabel mirrors the backend's snippet labels (internal/backend/inbox.go). */
export function kindLabel(kind: MessageKind | "" | string, seconds = 0, fileName = ""): string {
  switch (kind) {
    case "image":
      return "📷 Photo";
    case "video":
      return "🎥 Video";
    case "voice":
      return `🎤 Voice note (${duration(seconds)})`;
    case "audio":
      return "🎵 Audio";
    case "document":
      return `📄 ${fileName || "Document"}`;
    case "sticker":
      return "Sticker";
    default:
      return "";
  }
}

/** messageSnippet is a one-line preview of a message without its sender, like the backend's. */
export function messageSnippet(m: Pick<ChatMessage, "revoked" | "text" | "kind" | "media">): string {
  if (m.revoked) return "This message was deleted";
  let text = (m.text || "").trim();
  const fileName = m.media?.fileName ?? "";
  const label = kindLabel(m.kind, m.media?.seconds ?? 0, fileName);
  if (m.kind === "document" && text === fileName) text = "";
  const chars = Array.from(text);
  if (chars.length > 200) text = chars.slice(0, 200).join("") + "…";
  if (!label) return text;
  if (!text) return label;
  return `${label} ${text}`;
}

/** firstName gives "Ana" for "Ana Souza". */
export function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] || name;
}

/** chatPreview is the chat row's second line: "You: …", "Tiago: …" in groups, or just the text. */
export function chatPreview(c: Pick<InboxChat, "lastText" | "lastFromMe" | "lastSender" | "isGroup" | "lastAt">): string {
  const text = c.lastText || (c.lastAt ? "" : "No messages yet");
  if (!text) return "";
  if (c.lastFromMe) return `You: ${text}`;
  if (c.isGroup && c.lastSender) return `${firstName(c.lastSender)}: ${text}`;
  return text;
}

/** applyMessage updates a chat row with a message that just arrived or was sent. */
export function applyMessage(c: InboxChat, m: ChatMessage): InboxChat {
  if (m.at < c.lastAt && m.id !== c.lastId) return c;
  return {
    ...c,
    lastAt: m.at,
    lastId: m.id,
    lastText: messageSnippet(m),
    lastKind: m.kind,
    lastSender: m.fromMe ? "" : m.senderName,
    lastFromMe: m.fromMe,
    lastStatus: m.status,
  };
}

/** sortChats puts pinned chats first, then the newest. */
export function sortChats(cs: InboxChat[]): InboxChat[] {
  return [...cs].sort((a, b) => (a.pinned !== b.pinned ? (a.pinned ? -1 : 1) : b.lastAt - a.lastAt));
}

export type ListFilter = "all" | "unread" | "groups" | "direct" | "agent" | "archived";

/** matchesFilter mirrors ListInbox's filters, plus "agent" (chats an agent answers). */
export function matchesFilter(c: InboxChat, f: ListFilter, agentId = c.agentId): boolean {
  switch (f) {
    case "archived":
      return c.archived;
    case "unread":
      return c.unread > 0;
    case "groups":
      return c.isGroup && !c.archived;
    case "direct":
      return !c.isGroup && !c.archived;
    case "agent":
      return !!agentId && !c.archived;
    default:
      return !c.archived && (c.lastAt > 0 || c.pinned);
  }
}

/** normalize folds case and accents, for searching names. */
export function normalize(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();
}

/** tone picks a stable 0..n-1 colour for a JID. */
export function tone(jid: string, n = 5): number {
  let h = 0;
  for (let i = 0; i < jid.length; i++) h = (h * 31 + jid.charCodeAt(i)) | 0;
  return Math.abs(h) % n;
}

/** cursorOf builds the ListMessages cursor ("<at ms>:<id>") that pages back from m. */
export function cursorOf(m: Pick<ChatMessage, "at" | "id">): string {
  return `${m.at}:${m.id}`;
}

/** canEdit: WhatsApp lets the owner edit their own text messages for 15 minutes. */
export function canEdit(m: ChatMessage, now: number = Date.now()): boolean {
  return m.fromMe && !m.revoked && m.kind === "text" && now - m.at < 15 * 60000 && m.status !== "failed" && m.status !== "pending";
}

/** phoneDigits gives the digits of a typed number when it looks like one ("+55 51 9999-0000"), else "". */
export function phoneDigits(s: string): string {
  if (!/^[\s\d()+\-.]+$/.test(s.trim())) return "";
  const d = s.replace(/\D/g, "");
  return d.length >= 8 && d.length <= 15 ? d : "";
}

/** mergeMessages adds messages to a thread (oldest first), replacing ones with the same id. */
export function mergeMessages(existing: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  if (incoming.length === 0) return existing;
  const byId = new Map(existing.map((m) => [m.id, m]));
  for (const m of incoming) byId.set(m.id, m);
  return [...byId.values()].sort((a, b) => a.at - b.at || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/** pickRecorderMime chooses a MediaRecorder format WebKit can record and Positron can convert. */
export function pickRecorderMime(isSupported: (t: string) => boolean): string {
  for (const t of ["audio/mp4", "audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus", "audio/ogg"]) {
    try {
      if (isSupported(t)) return t;
    } catch {
      /* ignore */
    }
  }
  return "";
}

/** fileExt gives a file extension for a mime type, for pasted images without a name. */
export function fileExt(mime: string): string {
  const m = mime.split(";")[0].trim().toLowerCase();
  const known: Record<string, string> = {
    "image/png": ".png",
    "image/jpeg": ".jpg",
    "image/gif": ".gif",
    "image/webp": ".webp",
    "image/heic": ".heic",
    "video/mp4": ".mp4",
    "video/quicktime": ".mov",
    "audio/mpeg": ".mp3",
    "audio/mp4": ".m4a",
    "application/pdf": ".pdf",
  };
  return known[m] ?? "";
}

/** baseName gives "photo.jpg" for "/Users/me/photo.jpg". */
export function baseName(p: string): string {
  const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
  return i >= 0 ? p.slice(i + 1) : p;
}

/** disappearingLabel names a disappearing-messages timer: "24 hours", "7 days", "90 days"; "" when off. */
export function disappearingLabel(seconds: number): string {
  if (!seconds || seconds <= 0) return "";
  const hours = Math.round(seconds / 3600);
  if (hours < 1) return `${Math.max(1, Math.round(seconds / 60))} minutes`;
  if (hours % 24 !== 0 || hours === 24) return hours === 1 ? "1 hour" : `${hours} hours`;
  const days = hours / 24;
  return days === 1 ? "1 day" : `${days} days`;
}

/** viewOnceLabel is how a view-once message's media is named: "photo", "video", "voice message". */
export function viewOnceLabel(kind: MessageKind | string): string {
  if (kind === "video") return "video";
  if (kind === "voice" || kind === "audio") return "voice message";
  return "photo";
}
