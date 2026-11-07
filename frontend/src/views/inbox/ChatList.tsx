import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { inbox, type Chat, type InboxChat, type SearchHit } from "../../api";
import { Icon, SrLabel } from "../../components/ui";
import { Avatar } from "./Avatar";
import { chatPreview, listTime, matchesFilter, messageSnippet, normalize, type ListFilter } from "./helpers";

const ROW = 68; // px, fixed so the list can render only what is on screen
const OVERSCAN = 6;

export type AgentInfo = Map<string, Chat>; // ListChats rows by JID: agent, mode, all-direct, left alone

export function ChatList({
  chats,
  agentInfo,
  selected,
  typing,
  onOpen,
  onNewChat,
}: {
  chats: InboxChat[];
  agentInfo: AgentInfo;
  selected: string;
  /** chat JID -> "typing…" / "recording audio…" */
  typing: Map<string, string>;
  onOpen: (jid: string, messageId?: string) => void;
  onNewChat: () => void;
}) {
  const [filter, setFilter] = useState<ListFilter>("all");
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const agentOf = (c: InboxChat) => agentInfo.get(c.jid)?.agentId ?? c.agentId;

  const unreadCount = useMemo(() => chats.filter((c) => c.unread > 0).length, [chats]);
  const shown = useMemo(() => {
    const needle = normalize(q.trim());
    return chats.filter((c) => {
      if (needle) return normalize(c.name).includes(needle);
      return matchesFilter(c, filter, agentOf(c));
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chats, q, filter, agentInfo]);

  // Messages search, debounced.
  useEffect(() => {
    const needle = q.trim();
    if (needle.length < 2) {
      setHits([]);
      setSearching(false);
      return;
    }
    setSearching(true);
    let alive = true;
    const t = setTimeout(() => {
      inbox
        .search(needle, "", 30)
        .then((h) => alive && setHits(h))
        .catch(() => alive && setHits([]))
        .finally(() => alive && setSearching(false));
    }, 250);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [q]);

  // Virtual rows.
  const scroller = useRef<HTMLDivElement>(null);
  const rowsEl = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, height: 800 });
  const onScroll = () => {
    const el = scroller.current;
    if (el) setView({ top: el.scrollTop, height: el.clientHeight });
  };
  useLayoutEffect(() => {
    onScroll();
    const el = scroller.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(onScroll);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const offset = rowsEl.current?.offsetTop ?? 0;
  const first = Math.max(0, Math.floor((view.top - offset) / ROW) - OVERSCAN);
  const last = Math.min(shown.length, Math.ceil((view.top - offset + view.height) / ROW) + OVERSCAN);

  // Roving focus: arrows move, Enter opens.
  const [cursor, setCursor] = useState(0);
  const focusNext = useRef(false);
  useEffect(() => {
    if (cursor >= shown.length) setCursor(Math.max(0, shown.length - 1));
  }, [shown.length, cursor]);
  useEffect(() => {
    const i = shown.findIndex((c) => c.jid === selected);
    if (i >= 0) setCursor(i);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected]);
  const moveTo = (i: number) => {
    const n = Math.max(0, Math.min(shown.length - 1, i));
    setCursor(n);
    const el = scroller.current;
    if (el) {
      const y = offset + n * ROW;
      if (y < el.scrollTop) el.scrollTop = y;
      else if (y + ROW > el.scrollTop + el.clientHeight) el.scrollTop = y + ROW - el.clientHeight;
    }
    focusNext.current = true;
  };
  useEffect(() => {
    if (!focusNext.current) return;
    focusNext.current = false;
    rowsEl.current?.querySelector<HTMLElement>(`[data-index="${cursor}"]`)?.focus();
  });
  const onRowsKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      moveTo(cursor + 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (cursor === 0) document.getElementById("inbox-search")?.focus();
      else moveTo(cursor - 1);
    } else if (e.key === "Home") {
      e.preventDefault();
      moveTo(0);
    } else if (e.key === "End") {
      e.preventDefault();
      moveTo(shown.length - 1);
    }
  };

  const filters: { id: ListFilter; label: string }[] = [
    { id: "all", label: "All" },
    { id: "unread", label: unreadCount ? `Unread ${unreadCount}` : "Unread" },
    { id: "groups", label: "Groups" },
    { id: "direct", label: "Direct" },
    { id: "agent", label: "With an agent" },
    { id: "archived", label: "Archived" },
  ];

  const searchingNow = q.trim() !== "";
  return (
    <section className="ib-list" aria-label="Chats">
      <div className="ib-list__top">
        <div className="ib-list__title">
          <h1 className="ib-h1">WhatsApp chats</h1>
          <button type="button" className="ib-iconbtn ib-iconbtn--boxed" onClick={onNewChat} aria-label="New chat" title="New chat">
            <Icon name="plus" size={16} strokeWidth={2} />
          </button>
        </div>
        <div className="search ib-search">
          <Icon name="search" size={15} strokeWidth={2} />
          <SrLabel htmlFor="inbox-search">Search chats and messages</SrLabel>
          <input
            id="inbox-search"
            type="search"
            placeholder="Search chats and messages"
            value={q}
            autoComplete="off"
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" && shown.length) {
                e.preventDefault();
                moveTo(0);
              } else if (e.key === "Enter" && shown.length) {
                onOpen(shown[0].jid);
              } else if (e.key === "Escape") setQ("");
            }}
          />
        </div>
        {!searchingNow && (
          <div className="ib-chips" role="group" aria-label="Show">
            {filters.map((f) => (
              <button
                key={f.id}
                type="button"
                aria-pressed={filter === f.id}
                className={filter === f.id ? "ib-chip ib-chip--on" : "ib-chip"}
                onClick={() => setFilter(f.id)}
              >
                {f.label}
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="ib-list__scroll" ref={scroller} onScroll={onScroll}>
        {searchingNow && <div className="ib-section">Chats</div>}
        {shown.length === 0 && (
          <p className="ib-empty muted small">
            {searchingNow
              ? "No chats with that name."
              : chats.length === 0
                ? "No chats yet. They appear here as soon as someone writes."
                : filter === "unread"
                  ? "You're all caught up."
                  : filter === "archived"
                    ? "No archived chats."
                    : "No chats here."}
          </p>
        )}
        <div
          ref={rowsEl}
          className="ib-rows"
          role="list"
          aria-label={searchingNow ? "Chats that match" : "Chats"}
          style={{ height: shown.length * ROW }}
          onKeyDown={onRowsKey}
        >
          {shown.slice(first, last).map((c, k) => {
            const i = first + k;
            const info = agentInfo.get(c.jid);
            return (
              <ChatRow
                key={c.jid}
                chat={c}
                index={i}
                top={i * ROW}
                on={c.jid === selected}
                focusable={i === cursor}
                agentName={info ? (info.skipped ? "" : info.agentName) : c.agentName}
                viaAll={!!info?.viaAll}
                typing={typing.get(c.jid) ?? ""}
                onOpen={() => {
                  setCursor(i);
                  onOpen(c.jid);
                }}
              />
            );
          })}
        </div>
        {searchingNow && (
          <>
            <div className="ib-section">
              Messages
              {searching && <span className="spinner spinner--sm" aria-hidden="true" />}
            </div>
            {q.trim().length < 2 ? (
              <p className="ib-empty muted small">Type at least two letters to search messages.</p>
            ) : !searching && hits.length === 0 ? (
              <p className="ib-empty muted small">No messages found.</p>
            ) : (
              <ul className="ib-hits" aria-live="polite" aria-label="Messages that match">
                {hits.map((h) => (
                  <li key={`${h.chatJid}/${h.message.id}`}>
                    <button type="button" className="ib-hit" onClick={() => onOpen(h.chatJid, h.message.id)}>
                      <span className="ib-hit__top">
                        <span className="strong ellipsis">{h.chatName}</span>
                        <span className="muted xsmall tabular">{listTime(h.message.at)}</span>
                      </span>
                      <span className="ib-hit__text">
                        {h.message.fromMe ? "You: " : h.isGroup && h.message.senderName ? `${h.message.senderName}: ` : ""}
                        <Highlight text={messageSnippet(h.message)} q={q.trim()} />
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </div>
    </section>
  );
}

function Highlight({ text, q }: { text: string; q: string }) {
  const i = normalize(text).indexOf(normalize(q));
  // Only safe when normalising kept the length (no decomposed accents in the hit).
  if (i < 0 || normalize(text).length !== text.length) return <>{text}</>;
  return (
    <>
      {text.slice(0, i)}
      <mark>{text.slice(i, i + q.length)}</mark>
      {text.slice(i + q.length)}
    </>
  );
}

function ChatRow({
  chat: c,
  index,
  top,
  on,
  focusable,
  agentName,
  viaAll,
  typing,
  onOpen,
}: {
  chat: InboxChat;
  index: number;
  top: number;
  on: boolean;
  focusable: boolean;
  agentName: string;
  viaAll: boolean;
  typing: string;
  onOpen: () => void;
}) {
  const unread = c.unread > 0;
  const preview = typing || chatPreview(c);
  const label = [c.name, unread ? `${c.unread} unread` : "", agentName ? `answered by ${agentName}` : "", preview, listTime(c.lastAt)]
    .filter(Boolean)
    .join(", ");
  return (
    <div role="listitem" className="ib-rowwrap" style={{ transform: `translateY(${top}px)`, height: ROW }}>
      <button
        type="button"
        data-index={index}
        tabIndex={focusable ? 0 : -1}
        className={["ib-row", on ? "ib-row--on" : "", unread ? "ib-row--unread" : ""].join(" ")}
        aria-current={on ? "true" : undefined}
        aria-label={label}
        onClick={onOpen}
      >
        <Avatar jid={c.jid} url={c.avatarUrl} initials={c.initials} size={40} />
        <span className="ib-row__main">
          <span className="ib-row__name">
            <span className="ellipsis">{c.name}</span>
            {agentName && (
              <span className="ib-bot" title={viaAll ? `${agentName} answers all direct chats` : `${agentName} answers this chat`}>
                {agentName}
              </span>
            )}
          </span>
          <span className={typing ? "ib-row__prev ib-row__prev--typing" : "ib-row__prev"}>{preview}</span>
        </span>
        <span className="ib-row__meta">
          <span className="tabular">{listTime(c.lastAt)}</span>
          <span className="ib-row__flags">
            {c.muted && (
              <span title="Muted" className="ib-flag">
                <Icon name="muted" size={13} />
              </span>
            )}
            {c.pinned && (
              <span title="Pinned" className="ib-flag">
                <Icon name="pin" size={13} />
              </span>
            )}
            {unread && <span className={c.muted ? "ib-unread ib-unread--muted" : "ib-unread"}>{c.unread > 999 ? "999+" : c.unread}</span>}
          </span>
        </span>
      </button>
    </div>
  );
}
