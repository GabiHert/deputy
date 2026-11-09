import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { attachQuotes } from "../../lib/askContext";
import {
  api,
  api2,
  errText,
  inbox,
  inboxEvents,
  on,
  type Agent,
  type Chat,
  type InboxChat,
  type MessageEvent,
  type PresenceEvent,
  type Run,
  type UnreadEvent,
} from "../../api";
import { SessionsAside } from "../../components/SessionsAside";
import { ChatList, type AgentInfo } from "./ChatList";
import { Conversation, type Presence } from "./Conversation";
import { applyMessage, firstName, sortChats } from "./helpers";
import { NewChatDialog } from "./NewChatDialog";
import "./inbox.css";

const LAST_CHAT = "positron.inbox.chat";
const PANEL = "positron.claudePanel.inbox";

/** debounce runs fn at most once per ms, at the end. */
function useDebounced(fn: () => void, ms: number): () => void {
  const t = useRef<ReturnType<typeof setTimeout>>();
  const f = useRef(fn);
  f.current = fn;
  useEffect(() => () => clearTimeout(t.current), []);
  return useCallback(() => {
    clearTimeout(t.current);
    t.current = setTimeout(() => f.current(), ms);
  }, [ms]);
}

/** Inbox is the WhatsApp chats page: chat list, conversation and the sessions rail. */
export function Inbox({
  agents,
  open,
  onOpenAgent,
  onNewAgent,
}: {
  agents: Agent[];
  /** a chat to show (notification click), changes n each time */
  open: { jid: string; n: number } | null;
  onOpenAgent: (id: string) => void;
  onNewAgent: () => void;
}) {
  const [chats, setChats] = useState<InboxChat[]>([]);
  const [extra, setExtra] = useState<InboxChat | null>(null); // the open chat when the list does not have it
  const [agentInfo, setAgentInfo] = useState<AgentInfo>(new Map());
  const [selected, setSelected] = useState(() => open?.jid || localStorage.getItem(LAST_CHAT) || "");
  const [jump, setJump] = useState<{ id: string; n: number } | null>(null);
  const [newChat, setNewChat] = useState(false);
  const [presence, setPresence] = useState<Presence>({ typing: new Map(), online: new Map() });
  const [runs, setRuns] = useState<Run[]>([]);
  const [err, setErr] = useState("");
  const [askFocus, setAskFocus] = useState<{ id: string; n: number } | null>(null);
  const chatsRef = useRef(chats);
  chatsRef.current = chats;

  // ---- data ----
  const loadChats = useCallback(() => {
    inbox
      .list("all")
      .then((cs) => setChats(sortChats(cs)))
      .catch(() => {});
  }, []);
  const loadAgentInfo = useCallback(() => {
    api
      .listChats()
      .then((cs) => setAgentInfo(new Map(cs.map((c) => [c.jid, c]))))
      .catch(() => {});
  }, []);
  const loadRuns = useCallback(() => {
    api
      .listActivity("needs")
      .then((rs) => setRuns(rs.filter((r) => r.status === "waiting")))
      .catch(() => {});
  }, []);
  const soonChats = useDebounced(loadChats, 400);
  const soonInfo = useDebounced(loadAgentInfo, 400);
  const soonRuns = useDebounced(loadRuns, 250);

  useEffect(() => {
    loadChats();
    loadAgentInfo();
    loadRuns();
    const typingTimers = new Map<string, ReturnType<typeof setTimeout>>();
    const clearTyping = (chat: string) =>
      setPresence((p) => {
        if (!p.typing.has(chat)) return p;
        const typing = new Map(p.typing);
        typing.delete(chat);
        return { ...p, typing };
      });
    const offs = [
      on(inboxEvents.chats, () => {
        soonChats();
        soonInfo();
      }),
      on("agents", soonInfo),
      on("activity", soonRuns),
      on(inboxEvents.message, (ev: MessageEvent) => {
        const known = chatsRef.current.some((c) => c.jid === ev.chat);
        setChats((cs) => {
          const i = cs.findIndex((c) => c.jid === ev.chat);
          if (i < 0) return cs;
          const next = applyMessage(cs[i], ev.message);
          if (next === cs[i]) return cs;
          const copy = cs.slice();
          copy[i] = next;
          return ev.change === "new" ? sortChats(copy) : copy;
        });
        if (!known && ev.change === "new") soonChats();
        if (ev.change === "new" && !ev.message.fromMe) clearTyping(ev.chat);
      }),
      on(inboxEvents.unread, (ev: UnreadEvent) => {
        if (!ev.chat) {
          soonChats();
          return;
        }
        setChats((cs) => cs.map((c) => (c.jid === ev.chat && c.unread !== ev.unread ? { ...c, unread: ev.unread } : c)));
      }),
      on(inboxEvents.presence, (ev: PresenceEvent) => {
        if (ev.state === "composing" || ev.state === "recording") {
          if (!ev.chat) return;
          setPresence((p) => {
            const typing = new Map(p.typing);
            typing.set(ev.chat, { name: ev.name, state: ev.state as "composing" | "recording" });
            return { ...p, typing };
          });
          clearTimeout(typingTimers.get(ev.chat));
          typingTimers.set(
            ev.chat,
            setTimeout(() => clearTyping(ev.chat), 15000),
          );
        } else if (ev.state === "paused") {
          if (ev.chat) clearTyping(ev.chat);
        } else {
          setPresence((p) => {
            const online = new Map(p.online);
            const v = { online: ev.state === "online", lastSeen: ev.lastSeen || (ev.state === "offline" ? Date.now() : 0) };
            online.set(ev.jid, v);
            if (ev.chat && ev.chat !== ev.jid) online.set(ev.chat, v);
            return { ...p, online };
          });
          if (ev.state === "offline" && ev.chat) clearTyping(ev.chat);
        }
      }),
    ];
    return () => {
      offs.forEach((f) => f());
      typingTimers.forEach((t) => clearTimeout(t));
    };
  }, [loadChats, loadAgentInfo, loadRuns, soonChats, soonInfo, soonRuns]);

  // ---- which chat is open ----
  const openChat = useCallback((jid: string, messageId?: string) => {
    setSelected(jid);
    localStorage.setItem(LAST_CHAT, jid);
    setJump(messageId ? { id: messageId, n: Date.now() } : null);
  }, []);
  useEffect(() => {
    if (open?.jid) openChat(open.jid);
  }, [open?.n]); // eslint-disable-line react-hooks/exhaustive-deps

  const listed = chats.find((c) => c.jid === selected);
  useEffect(() => {
    if (!selected || listed || chats.length === 0) return;
    if (extra?.jid === selected) return;
    let alive = true;
    inbox
      .chat(selected)
      .then((c) => alive && setExtra(c))
      .catch(() => {
        if (!alive) return;
        // The saved chat is gone.
        setSelected("");
        localStorage.removeItem(LAST_CHAT);
      });
    return () => {
      alive = false;
    };
  }, [selected, listed, chats.length, extra?.jid]);
  const current = listed ?? (extra?.jid === selected ? extra : null);

  // ---- agent for a chat ----
  const allAgent = agents.find((a) => a.allDirect);
  const assign = async (jid: string, agentId: string) => {
    setErr("");
    const c = current;
    setAgentInfo((m) => {
      const prev = m.get(jid);
      if (!prev) return m;
      const isGroup = prev.isGroup;
      const a = agents.find((x) => x.id === agentId);
      const next = new Map(m);
      next.set(jid, {
        ...prev,
        agentId,
        agentName: a?.name ?? "",
        mode: a?.mode ?? "",
        viaAll: !isGroup && !!allAgent && agentId === allAgent.id,
        skipped: !agentId && !isGroup && !!allAgent,
      });
      return next;
    });
    try {
      await api.assignChat(jid, agentId);
    } catch (e) {
      setErr(`Could not change the agent for ${c?.name ?? "this chat"}: ${errText(e)}`);
    }
    loadAgentInfo();
    soonChats();
  };

  // ---- presence for the list ----
  const typingLabels = useMemo(() => {
    const m = new Map<string, string>();
    presence.typing.forEach((v, chat) => {
      const isGroup = chat.endsWith("@g.us");
      const what = v.state === "recording" ? "recording audio…" : "typing…";
      m.set(chat, isGroup && v.name ? `${firstName(v.name)} is ${what}` : what);
    });
    return m;
  }, [presence.typing]);

  const drafts = useMemo(() => (current ? runs.filter((r) => r.chatJid === current.jid).sort((a, b) => a.at - b.at) : []), [runs, current]);

  return (
    <div className="ib">
      <ChatList chats={chats} agentInfo={agentInfo} selected={selected} typing={typingLabels} onOpen={openChat} onNewChat={() => setNewChat(true)} />
      <div className="ib-main">
        {err && (
          <div className="banner banner--bad banner--tight" role="alert">
            {err}
          </div>
        )}
        {current ? (
          <Conversation
            key={current.jid}
            chat={current}
            info={agentInfo.get(current.jid) ?? fallbackInfo(current)}
            agents={agents}
            presence={presence}
            drafts={drafts}
            onDraftDone={(id) => {
              setRuns((rs) => rs.filter((r) => r.id !== id));
              soonRuns();
            }}
            onAssign={(agentId) => assign(current.jid, agentId)}
            onAsk={(quotes) =>
              api2
                .askAgentChat(current.jid)
                .then((c) => {
                  if (quotes.length) attachQuotes(c.id, current.name, quotes);
                  setAskFocus((f) => ({ id: c.id, n: (f?.n ?? 0) + 1 }));
                })
                .catch((e) => setErr(errText(e)))
            }
            jump={jump}
          />
        ) : (
          <div className="ib-none">
            <p className="ib-none__title">Pick a chat</p>
            <p className="muted small">Read and answer your WhatsApp here. Your phone stays in sync.</p>
            <button type="button" className="btn btn--sm" onClick={() => setNewChat(true)}>
              New chat
            </button>
          </div>
        )}
      </div>
      <SessionsAside storageKey={PANEL} defaultOpen={false} onOpenAgent={onOpenAgent} onNewAgent={onNewAgent} focus={askFocus} />
      {newChat && (
        <NewChatDialog
          onClose={() => setNewChat(false)}
          onOpen={(jid) => {
            setNewChat(false);
            openChat(jid);
            soonChats();
          }}
        />
      )}
    </div>
  );
}

/** fallbackInfo is the agent part of a chat that ListChats has not listed (e.g. a new empty chat). */
function fallbackInfo(c: InboxChat): Chat | undefined {
  return {
    jid: c.jid,
    name: c.name,
    isGroup: c.isGroup,
    members: c.members,
    lastText: c.lastText,
    lastAt: c.lastAt,
    agentId: c.agentId,
    agentName: c.agentName,
    mode: "",
    viaAll: false,
    skipped: false,
  };
}
