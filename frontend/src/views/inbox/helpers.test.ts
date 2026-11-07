import { describe, expect, it } from "vitest";
import type { ChatMessage, InboxChat } from "../../api";
import {
  applyMessage,
  canEdit,
  chatPreview,
  cursorOf,
  dayLabel,
  disappearingLabel,
  groupByDay,
  lastSeen,
  listTime,
  matchesFilter,
  mergeMessages,
  messageSnippet,
  normalize,
  phoneDigits,
  pickRecorderMime,
  sortChats,
  viewOnceLabel,
} from "./helpers";

const now = new Date(2026, 10, 6, 15, 0).getTime(); // Friday 6 Nov 2026, 15:00
const at = (d: number, h = 10, m = 0, month = 10, y = 2026) => new Date(y, month, d, h, m).getTime();

function msg(p: Partial<ChatMessage>): ChatMessage {
  return {
    id: "m1",
    chatJid: "c@s.whatsapp.net",
    senderJid: "a@s.whatsapp.net",
    senderName: "Ana",
    fromMe: false,
    fromAgent: false,
    agentName: "",
    fromUi: false,
    at: at(6),
    kind: "text",
    text: "oi",
    transcript: "",
    mentions: [],
    mentionsMe: false,
    quoted: null,
    reactions: [],
    media: null,
    status: "",
    edited: false,
    revoked: false,
    history: false,
    viewOnce: false,
    ...p,
  };
}

function chat(p: Partial<InboxChat>): InboxChat {
  return {
    jid: "c@s.whatsapp.net",
    name: "Ana",
    initials: "A",
    isGroup: false,
    members: 0,
    avatarUrl: "",
    lastAt: at(6),
    lastId: "m0",
    lastText: "hello",
    lastKind: "text",
    lastSender: "",
    lastFromMe: false,
    lastStatus: "",
    unread: 0,
    muted: false,
    mutedUntil: 0,
    pinned: false,
    archived: false,
    agentId: "",
    agentName: "",
    disappearing: 0,
    ...p,
  };
}

describe("listTime", () => {
  it("shows the clock today, Yesterday, the weekday, then the date", () => {
    expect(listTime(at(6, 9, 5), now)).toBe("09:05");
    expect(listTime(at(5, 23, 59), now)).toBe("Yesterday");
    expect(listTime(at(3), now)).toBe("Tuesday");
    expect(listTime(at(20, 10, 0, 9), now)).toBe("20 Oct");
    expect(listTime(at(20, 10, 0, 9, 2024), now)).toBe("20 Oct 2024");
    expect(listTime(0, now)).toBe("");
  });
});

describe("dayLabel and lastSeen", () => {
  it("names days", () => {
    expect(dayLabel(at(6), now)).toBe("Today");
    expect(dayLabel(at(5), now)).toBe("Yesterday");
    expect(dayLabel(at(2), now)).toBe("Monday");
    expect(dayLabel(at(4, 10, 0, 9), now)).toBe("4 October");
  });
  it("words last seen", () => {
    expect(lastSeen(at(6, 14, 3), now)).toBe("last seen today at 14:03");
    expect(lastSeen(at(5, 9, 10), now)).toBe("last seen yesterday at 09:10");
    expect(lastSeen(0, now)).toBe("");
  });
});

describe("groupByDay", () => {
  it("puts a separator before each day and marks runs by the same sender", () => {
    const items = groupByDay(
      [
        msg({ id: "a", at: at(5, 10) }),
        msg({ id: "b", at: at(6, 10) }),
        msg({ id: "c", at: at(6, 10, 1) }),
        msg({ id: "d", at: at(6, 10, 2), fromMe: true, senderJid: "" }),
      ],
      now,
    );
    expect(items.map((i) => (i.type === "day" ? i.label : i.key))).toEqual(["Yesterday", "a", "Today", "b", "c", "d"]);
    const flags = items.filter((i) => i.type === "msg").map((i) => (i.type === "msg" ? i.continued : null));
    expect(flags).toEqual([false, false, true, false]);
  });
  it("handles an empty thread", () => {
    expect(groupByDay([], now)).toEqual([]);
  });
});

describe("previews", () => {
  it("labels media like the backend", () => {
    expect(messageSnippet(msg({ kind: "image", text: "" }))).toBe("📷 Photo");
    expect(messageSnippet(msg({ kind: "image", text: "look" }))).toBe("📷 Photo look");
    expect(
      messageSnippet(
        msg({ kind: "voice", text: "", media: { mime: "", seconds: 18, fileName: "", size: 0, url: "", thumbUrl: "", downloadable: true } }),
      ),
    ).toBe("🎤 Voice note (0:18)");
    expect(
      messageSnippet(
        msg({ kind: "document", text: "a.pdf", media: { mime: "", seconds: 0, fileName: "a.pdf", size: 0, url: "", thumbUrl: "", downloadable: true } }),
      ),
    ).toBe("📄 a.pdf");
    expect(messageSnippet(msg({ revoked: true }))).toBe("This message was deleted");
  });
  it("prefixes the sender in groups and You for the owner", () => {
    expect(chatPreview(chat({ lastText: "hi", lastFromMe: true }))).toBe("You: hi");
    expect(chatPreview(chat({ isGroup: true, lastSender: "Tiago Herter", lastText: "sábado?" }))).toBe("Tiago: sábado?");
    expect(chatPreview(chat({ isGroup: false, lastSender: "Ana", lastText: "x" }))).toBe("x");
    expect(chatPreview(chat({ lastText: "", lastAt: 0 }))).toBe("No messages yet");
  });
  it("updates a row with a newer message only", () => {
    const c = chat({ lastAt: at(6, 12) });
    expect(applyMessage(c, msg({ id: "old", at: at(6, 11) }))).toBe(c);
    const n = applyMessage(c, msg({ id: "new", at: at(6, 13), text: "yo", fromMe: true, status: "sent" }));
    expect(n.lastText).toBe("yo");
    expect(n.lastFromMe).toBe(true);
    expect(n.lastSender).toBe("");
  });
});

describe("chat list", () => {
  it("sorts pinned first, then newest", () => {
    const s = sortChats([chat({ jid: "a", lastAt: 1 }), chat({ jid: "b", lastAt: 3 }), chat({ jid: "p", lastAt: 0, pinned: true })]);
    expect(s.map((c) => c.jid)).toEqual(["p", "b", "a"]);
  });
  it("filters like ListInbox", () => {
    const g = chat({ isGroup: true });
    const arch = chat({ archived: true, unread: 2 });
    expect(matchesFilter(g, "groups")).toBe(true);
    expect(matchesFilter(g, "direct")).toBe(false);
    expect(matchesFilter(arch, "all")).toBe(false);
    expect(matchesFilter(arch, "archived")).toBe(true);
    expect(matchesFilter(arch, "unread")).toBe(true);
    expect(matchesFilter(chat({ lastAt: 0 }), "all")).toBe(false);
    expect(matchesFilter(chat({}), "agent", "ag1")).toBe(true);
    expect(matchesFilter(chat({}), "agent", "")).toBe(false);
  });
  it("folds accents for search", () => {
    expect(normalize("Mãe Ação")).toBe("mae acao");
  });
});

describe("disappearing and view once", () => {
  it("names the timers WhatsApp offers", () => {
    expect(disappearingLabel(86400)).toBe("24 hours");
    expect(disappearingLabel(604800)).toBe("7 days");
    expect(disappearingLabel(7776000)).toBe("90 days");
    expect(disappearingLabel(0)).toBe("");
  });
  it("names view-once media", () => {
    expect(viewOnceLabel("image")).toBe("photo");
    expect(viewOnceLabel("video")).toBe("video");
    expect(viewOnceLabel("voice")).toBe("voice message");
  });
});

describe("misc", () => {
  it("builds cursors and merges pages", () => {
    expect(cursorOf({ at: 12, id: "X" })).toBe("12:X");
    const merged = mergeMessages([msg({ id: "b", at: 2 }), msg({ id: "c", at: 3 })], [msg({ id: "a", at: 1 }), msg({ id: "c", at: 3, text: "edited" })]);
    expect(merged.map((m) => m.id)).toEqual(["a", "b", "c"]);
    expect(merged[2].text).toBe("edited");
  });
  it("allows edits of own text for 15 minutes", () => {
    expect(canEdit(msg({ fromMe: true, at: now - 60000, status: "sent" }), now)).toBe(true);
    expect(canEdit(msg({ fromMe: true, at: now - 16 * 60000, status: "sent" }), now)).toBe(false);
    expect(canEdit(msg({ fromMe: false, at: now }), now)).toBe(false);
    expect(canEdit(msg({ fromMe: true, kind: "image", at: now }), now)).toBe(false);
  });
  it("reads phone numbers", () => {
    expect(phoneDigits("+55 51 99999-0000")).toBe("5551999990000");
    expect(phoneDigits("Ana")).toBe("");
    expect(phoneDigits("123")).toBe("");
  });
  it("picks a recorder format", () => {
    expect(pickRecorderMime((t) => t === "audio/webm;codecs=opus")).toBe("audio/webm;codecs=opus");
    expect(pickRecorderMime((t) => t.startsWith("audio/"))).toBe("audio/mp4");
    expect(pickRecorderMime(() => false)).toBe("");
  });
});
