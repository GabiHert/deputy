import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessage, InboxChat } from "../../api";

const now = Date.now();
const chats: InboxChat[] = [
  {
    jid: "ana@s.whatsapp.net",
    name: "Ana Souza",
    initials: "AS",
    isGroup: false,
    members: 0,
    avatarUrl: "",
    lastAt: now - 60000,
    lastId: "m2",
    lastText: "Chego umas 19h então",
    lastKind: "text",
    lastSender: "",
    lastFromMe: false,
    lastStatus: "",
    unread: 2,
    muted: false,
    mutedUntil: 0,
    pinned: false,
    archived: false,
    agentId: "ag1",
    agentName: "Assessor",
    disappearing: 86400,
  },
  {
    jid: "fam@g.us",
    name: "Familia",
    initials: "F",
    isGroup: true,
    members: 9,
    avatarUrl: "",
    lastAt: now - 3600000,
    lastId: "g1",
    lastText: "Alguém vai sábado?",
    lastKind: "text",
    lastSender: "Tiago Herter",
    lastFromMe: false,
    lastStatus: "",
    unread: 0,
    muted: true,
    mutedUntil: 0,
    pinned: false,
    archived: false,
    agentId: "",
    agentName: "",
    disappearing: 0,
  },
];

const message = (p: Partial<ChatMessage>): ChatMessage => ({
  id: "m1",
  chatJid: "ana@s.whatsapp.net",
  senderJid: "ana@s.whatsapp.net",
  senderName: "Ana Souza",
  fromMe: false,
  fromAgent: false,
  agentName: "",
  fromUi: false,
  at: now - 120000,
  kind: "text",
  text: "Oi!",
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
});

const viewOnceMedia = (downloadable: boolean) => ({ mime: "image/jpeg", seconds: 0, fileName: "", size: 0, url: "", thumbUrl: "", downloadable });
const getMedia = vi.fn(async () => "/media/v2.jpg");

const sendText = vi.fn(async (m: { text: string }) => message({ id: "sent1", fromMe: true, senderJid: "", senderName: "", text: m.text, at: Date.now(), status: "sent" }));
const markRead = vi.fn(async () => {});
const setActiveChat = vi.fn(async () => {});

vi.mock("../../api", async (orig) => {
  const real = await orig<typeof import("../../api")>();
  return {
    ...real,
    on: () => () => {},
    useAssistantChats: () => [[], () => {}],
    api: {
      ...real.api,
      listChats: async () => [
        { jid: "ana@s.whatsapp.net", name: "Ana Souza", isGroup: false, members: 0, lastText: "", lastAt: 0, agentId: "ag1", agentName: "Assessor", mode: "approve", viaAll: false, skipped: false },
      ],
      listActivity: async () => [],
      assignChat: async () => {},
    },
    inbox: {
      ...real.inbox,
      list: async () => chats,
      chat: async (jid: string) => chats.find((c) => c.jid === jid)!,
      messages: async () => ({
        messages: [
          message({ id: "v2", kind: "image", text: "", viewOnce: true, media: viewOnceMedia(true), at: now - 30000 }),
          message({ id: "v1", kind: "video", text: "", viewOnce: true, media: viewOnceMedia(false), at: now - 40000 }),
          message({ id: "m2", text: "Chego umas 19h então", at: now - 60000 }),
          message({ id: "m1", text: "Oi!", fromMe: true, senderJid: "", senderName: "", status: "read" }),
        ],
        cursor: "",
        hasMore: false,
      }),
      markRead,
      setActiveChat,
      setTyping: async () => {},
      avatar: async () => "",
      sendText,
      getMedia,
      search: async () => [],
    },
  };
});

const { Inbox } = await import("./Inbox");

describe("Inbox", () => {
  beforeEach(() => {
    localStorage.clear();
    sendText.mockClear();
  });

  it("lists chats with agent tags and previews, opens one and sends", async () => {
    render(<Inbox agents={[]} open={null} onOpenAgent={() => {}} onNewAgent={() => {}} />);
    expect(await screen.findByText("Ana Souza")).toBeInTheDocument();
    expect(screen.getByText("Assessor")).toBeInTheDocument();
    expect(screen.getByText("Tiago: Alguém vai sábado?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Unread 1" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Ana Souza/ }));
    expect(await screen.findByText("Oi!")).toBeInTheDocument();
    expect(screen.getByText("Today")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Read" })).toBeInTheDocument();
    expect(screen.getByText("Disappearing messages: 24 hours")).toBeInTheDocument();

    // View-once: one only on the phone, one Deputy can show, but only when asked.
    expect(screen.getAllByText("View once")).toHaveLength(2);
    expect(screen.getByText("View once video")).toBeInTheDocument();
    expect(screen.getByText("Open it on your phone")).toBeInTheDocument();
    expect(getMedia).not.toHaveBeenCalled();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Tap to view the view once photo" }));
    });
    expect(getMedia).toHaveBeenCalledWith("ana@s.whatsapp.net", "v2");
    expect(await screen.findByRole("img", { name: "View once photo" })).toHaveAttribute("src", "/media/v2.jpg");
    expect(screen.getByRole("button", { name: /Agent for this chat: Assessor, After I approve/ })).toBeInTheDocument();
    await waitFor(() => expect(setActiveChat).toHaveBeenCalled());

    const input = screen.getByLabelText("Message Ana Souza");
    fireEvent.change(input, { target: { value: "Perfeito" } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(sendText).toHaveBeenCalledWith({ chat: "ana@s.whatsapp.net", text: "Perfeito", quotedId: "", mentions: [] });
    expect(await screen.findByText("Perfeito")).toBeInTheDocument();
  });

  it("filters to groups and searches by name", async () => {
    render(<Inbox agents={[]} open={null} onOpenAgent={() => {}} onNewAgent={() => {}} />);
    await screen.findByText("Ana Souza");
    fireEvent.click(screen.getByRole("button", { name: "Groups" }));
    expect(screen.queryByText("Ana Souza")).not.toBeInTheDocument();
    expect(screen.getByText("Familia")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search chats and messages"), { target: { value: "ana" } });
    expect(screen.getByText("Ana Souza")).toBeInTheDocument();
    expect(screen.queryByText("Familia")).not.toBeInTheDocument();
  });
});
