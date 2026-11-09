import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { Quote } from "../../lib/askContext";
import { quoteOf, quotesForSelection } from "./askQuotes";
import { inbox, type Agent, type Chat, type ChatMessage, type InboxChat, type Run } from "../../api";
import { plural } from "../../format";
import { Icon } from "../../components/ui";
import { AgentPicker } from "./AgentPicker";
import { Avatar } from "./Avatar";
import { Composer, releaseTray, trayFromFiles, type TrayItem } from "./Composer";
import { DraftCard } from "./DraftCard";
import { disappearingLabel, groupByDay, lastSeen } from "./helpers";
import { MessageBubble, type BubbleActions } from "./MessageBubble";
import { useThread } from "./useThread";

export type Presence = {
  /** chat JID -> who is typing or recording there */
  typing: Map<string, { name: string; state: "composing" | "recording" }>;
  /** contact JID -> online / last seen */
  online: Map<string, { online: boolean; lastSeen: number }>;
};

/** useWindowActive is true while the window has focus and is visible. */
function useWindowActive(): boolean {
  const get = () => (typeof document === "undefined" ? true : document.visibilityState === "visible" && document.hasFocus());
  const [active, setActive] = useState(get);
  useEffect(() => {
    const update = () => setActive(get());
    window.addEventListener("focus", update);
    window.addEventListener("blur", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      window.removeEventListener("focus", update);
      window.removeEventListener("blur", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);
  return active;
}

export function Conversation({
  chat,
  info,
  agents,
  presence,
  drafts,
  onDraftDone,
  onAssign,
  onAsk,
  jump,
}: {
  /** open a private Claude chat with this chat's agent */
  onAsk: (quotes: Quote[]) => void;
  chat: InboxChat;
  info: Chat | undefined;
  agents: Agent[];
  presence: Presence;
  drafts: Run[];
  onDraftDone: (id: string) => void;
  onAssign: (agentId: string) => void;
  /** scroll to this message once (from search) */
  jump: { id: string; n: number } | null;
}) {
  const jid = chat.jid;
  const scroller = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);
  const [newCount, setNewCount] = useState(0);
  const [highlight, setHighlight] = useState("");
  const [replyTo, setReplyTo] = useState<ChatMessage | null>(null);
  const [editing, setEditing] = useState<ChatMessage | null>(null);
  const [tray, setTrayState] = useState<TrayItem[]>([]);
  const [dragging, setDragging] = useState(false);
  const [err, setErr] = useState("");
  const [focusKey, setFocusKey] = useState(jid);
  const windowActive = useWindowActive();
  const activeRef = useRef(windowActive);
  activeRef.current = windowActive;

  const markRead = useCallback(() => {
    if (activeRef.current) inbox.markRead(jid).catch(() => {});
  }, [jid]);

  const thread = useThread(
    jid,
    useCallback(
      (m: ChatMessage) => {
        if (m.fromMe || stuck.current) {
          stuck.current = true;
        } else {
          setNewCount((n) => n + 1);
        }
        if (!m.fromMe) markRead();
      },
      [markRead],
    ),
  );

  // Open/close: the backend's active chat, read receipts, a clean composer.
  useEffect(() => {
    stuck.current = true;
    setNewCount(0);
    setReplyTo(null);
    setEditing(null);
    setErr("");
    setHighlight("");
    setFocusKey(`${jid}-${Date.now()}`);
    setTrayState((t) => {
      releaseTray(t);
      return [];
    });
  }, [jid]);
  useEffect(() => {
    if (windowActive) {
      inbox.setActiveChat(jid);
      if (chat.unread > 0) markRead();
    } else inbox.setActiveChat("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jid, windowActive]);
  useEffect(() => () => void inbox.setActiveChat(""), []);
  useEffect(() => {
    if (chat.unread > 0 && windowActive) markRead();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chat.unread]);

  // ---- scrolling ----
  const lastHeight = useRef(0);
  const firstId = useRef("");
  const toBottom = () => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  };
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const first = thread.messages[0]?.id ?? "";
    const prepended = firstId.current !== "" && first !== firstId.current && thread.messages.some((m) => m.id === firstId.current);
    if (prepended && !stuck.current) el.scrollTop += el.scrollHeight - lastHeight.current;
    if (stuck.current) el.scrollTop = el.scrollHeight;
    firstId.current = first;
    lastHeight.current = el.scrollHeight;
  }, [thread.messages, drafts.length]);
  useEffect(() => {
    const el = content.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => {
      if (stuck.current) toBottom();
      if (scroller.current) lastHeight.current = scroller.current.scrollHeight;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  useEffect(() => {
    firstId.current = "";
    lastHeight.current = 0;
  }, [jid]);

  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
    stuck.current = atBottom;
    if (atBottom && newCount) setNewCount(0);
    if (thread.loading || thread.loadingOlder) return;
    if (el.scrollTop < 240 && thread.hasMore) void thread.loadOlder();
    // Nothing older stored: at the very top, ask the phone (once until it answers).
    else if (
      el.scrollTop < 8 &&
      !thread.hasMore &&
      !thread.askingPhone &&
      !thread.phoneError &&
      !thread.reachedStart &&
      thread.messages.length > 0 &&
      el.scrollHeight > el.clientHeight
    )
      void thread.loadOlder();
  };

  const ensure = thread.ensure;
  const jumpTo = useCallback(
    async (id: string) => {
      const found = await ensure(id);
      if (!found) {
        setErr("That message is older than what Positron has. Scroll up to load more.");
        return;
      }
      stuck.current = false;
      requestAnimationFrame(() => {
        const el = document.getElementById(`msg-${id}`);
        el?.scrollIntoView({ block: "center" });
        setHighlight(id);
        setTimeout(() => setHighlight((h) => (h === id ? "" : h)), 2500);
      });
    },
    [ensure],
  );
  useEffect(() => {
    if (jump && thread.loadedFor === jid) void jumpTo(jump.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jump?.n, thread.loadedFor]);

  const actions: BubbleActions = useMemo(
    () => ({
      onReply: (m) => {
        setEditing(null);
        setReplyTo(m);
        setFocusKey(`reply-${m.id}`);
      },
      onEdit: (m) => {
        setReplyTo(null);
        setEditing(m);
        setFocusKey(`edit-${m.id}`);
      },
      onJump: (id) => void jumpTo(id),
      onError: setErr,
      onAsk: info?.agentId ? (m) => onAsk([quoteOf(m)]) : undefined,
      askLabel: `Ask ${info?.agentName || "the agent"} about this`,
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [jumpTo, info?.agentId, info?.agentName],
  );

  // Highlighting text in the conversation offers "Ask <agent>", like asking Claude about selected code.
  const [sel, setSel] = useState<{ top: number; left: number; ids: string[]; text: string } | null>(null);
  useEffect(() => {
    if (!info?.agentId) return;
    const onChange = () => {
      const s = document.getSelection();
      const root = scroller.current;
      if (!s || s.isCollapsed || !root || !s.rangeCount) return setSel(null);
      const range = s.getRangeAt(0);
      if (!root.contains(range.commonAncestorContainer)) return setSel(null);
      const ids: string[] = [];
      root.querySelectorAll<HTMLElement>(".ib-msg[data-id]").forEach((el) => {
        if (range.intersectsNode(el) && el.dataset.id) ids.push(el.dataset.id);
      });
      const text = s.toString();
      if (!ids.length || !text.trim()) return setSel(null);
      const r = range.getBoundingClientRect();
      const box = root.getBoundingClientRect();
      setSel({ top: Math.max(r.top - box.top + root.scrollTop - 40, root.scrollTop + 4), left: Math.min(Math.max(r.left - box.left + r.width / 2, 70), box.width - 70), ids, text });
    };
    document.addEventListener("selectionchange", onChange);
    return () => document.removeEventListener("selectionchange", onChange);
  }, [info?.agentId]);
  const askSelection = () => {
    if (!sel) return;
    const byId = new Map(thread.messages.map((m) => [m.id, m]));
    onAsk(quotesForSelection(sel.ids, sel.text, byId));
    document.getSelection()?.removeAllRanges();
    setSel(null);
  };

  const items = useMemo(() => groupByDay(thread.messages), [thread.messages]);

  // ---- header line ----
  const typing = presence.typing.get(jid);
  const contact = presence.online.get(jid);
  let status = "";
  let statusLive = false;
  if (chat.isGroup) {
    if (typing) {
      status = `${typing.name || "Someone"} is ${typing.state === "recording" ? "recording audio" : "typing"}…`;
      statusLive = true;
    } else status = chat.members ? plural(chat.members, "member") : "Group";
  } else if (typing) {
    status = typing.state === "recording" ? "recording audio…" : "typing…";
    statusLive = true;
  } else if (contact?.online) {
    status = "online";
    statusLive = true;
  } else if (contact?.lastSeen) status = lastSeen(contact.lastSeen);

  // ---- drag and drop ----
  const onDragOver = (e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes("Files")) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    if (!dragging) setDragging(true);
  };
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const files = Array.from(e.dataTransfer.files ?? []);
    if (files.length) setTrayState((t) => [...t, ...trayFromFiles(files)]);
  };

  const older = (() => {
    if (thread.loading) return null;
    if (thread.loadingOlder) return <span className="ib-older">Loading older messages…</span>;
    if (thread.askingPhone)
      return (
        <span className="ib-older" role="status">
          <span className="spinner spinner--sm" aria-hidden="true" /> Asking your phone for older messages…
        </span>
      );
    if (thread.phoneError)
      return (
        <span className="ib-older ib-older--bad" role="alert">
          {thread.phoneError}
          <button type="button" className="btn btn--quiet btn--sm" onClick={() => void thread.loadOlder()}>
            Try again
          </button>
        </span>
      );
    if (thread.hasMore) return null;
    if (thread.reachedStart) return <span className="ib-older">This is the start of the chat on your phone.</span>;
    if (thread.messages.length === 0) return null;
    return (
      <button type="button" className="ib-older ib-older--btn" onClick={() => void thread.loadOlder()}>
        Load older messages · asks your phone
      </button>
    );
  })();

  return (
    <section
      className={dragging ? "ib-conv ib-conv--drop" : "ib-conv"}
      aria-label={`Conversation with ${chat.name}`}
      onDragOver={onDragOver}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node)) setDragging(false);
      }}
      onDrop={onDrop}
    >
      <header className="ib-conv__head">
        <Avatar jid={jid} url={chat.avatarUrl} initials={chat.initials} size={36} />
        <div className="ib-conv__who">
          <h2 className="ib-conv__name ellipsis">{chat.name}</h2>
          <span className="ib-conv__line">
            <span className={statusLive ? "ib-conv__status ib-conv__status--live" : "ib-conv__status"}>{status}</span>
            {chat.disappearing > 0 && (
              <span className="ib-conv__timer" title="Messages in this chat disappear after this time. Positron's messages follow it too.">
                <Icon name="timer" size={12} />
                Disappearing messages: {disappearingLabel(chat.disappearing)}
              </span>
            )}
          </span>
        </div>
        {info?.agentId && (
          <button
            type="button"
            className="btn btn--sm ib-ask"
            onClick={() => onAsk([])}
            title={`Ask ${info.agentName || "the agent"} something about this chat, privately`}
          >
            <Icon name="chats" size={15} />
            <span className="ellipsis">Ask {info.agentName || "agent"}</span>
          </button>
        )}
        <AgentPicker info={info} isGroup={chat.isGroup} agents={agents} onAssign={onAssign} />
      </header>

      {err && (
        <div className="banner banner--bad banner--tight ib-err" role="alert">
          <span>{err}</span>
          <button type="button" className="ib-iconbtn" aria-label="Dismiss" onClick={() => setErr("")}>
            <Icon name="close" size={14} strokeWidth={2} />
          </button>
        </div>
      )}

      <div className="ib-msgs" ref={scroller} onScroll={onScroll} tabIndex={-1}>
        {sel && (
          <button
            type="button"
            className="ib-selask"
            style={{ top: sel.top, left: sel.left }}
            onMouseDown={(e) => e.preventDefault()}
            onClick={askSelection}
          >
            <Icon name="chats" size={14} />
            Ask {info?.agentName || "agent"}
          </button>
        )}
        <div className="ib-msgs__inner" ref={content}>
          {older}
          {thread.loading && (
            <span className="ib-older">
              <span className="spinner spinner--sm" aria-hidden="true" /> Loading messages…
            </span>
          )}
          {thread.error && <span className="ib-older ib-older--bad">{thread.error}</span>}
          {!thread.loading && thread.messages.length === 0 && !thread.error && (
            <p className="ib-nomsgs muted small">No messages here yet. Say hello.</p>
          )}
          {items.map((it) =>
            it.type === "day" ? (
              <span key={it.key} className="ib-day" role="separator">
                {it.label}
              </span>
            ) : (
              <MessageBubble key={it.key} m={it.message} isGroup={chat.isGroup} continued={it.continued} highlight={highlight === it.key} actions={actions} />
            ),
          )}
          {drafts.map((r) => (
            <DraftCard key={r.id} run={r} onDone={onDraftDone} />
          ))}
        </div>
      </div>

      {newCount > 0 && (
        <button
          type="button"
          className="ib-newpill"
          onClick={() => {
            stuck.current = true;
            toBottom();
            setNewCount(0);
          }}
        >
          <Icon name="down" size={14} strokeWidth={2} /> {newCount} new
        </button>
      )}
      <span className="sr-only" aria-live="polite">
        {newCount > 0 ? `${plural(newCount, "new message")} below` : ""}
      </span>

      {dragging && (
        <div className="ib-dropzone" aria-hidden="true">
          Drop files to send them to {chat.name}
        </div>
      )}

      <Composer
        chat={jid}
        chatName={chat.name}
        replyTo={replyTo}
        onCancelReply={() => setReplyTo(null)}
        editing={editing}
        onCancelEdit={() => setEditing(null)}
        tray={tray}
        setTray={setTrayState}
        onSent={(m) => {
          stuck.current = true;
          thread.upsert(m);
        }}
        onError={setErr}
        focusKey={focusKey}
      />
    </section>
  );
}
