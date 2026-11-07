import { useCallback, useEffect, useRef, useState } from "react";
import {
  errText,
  inbox,
  inboxEvents,
  on,
  type ChatMessage,
  type HistoryEvent,
  type MessageEvent,
  type ReceiptEvent,
} from "../../api";
import { cursorOf, mergeMessages } from "./helpers";

const PAGE = 50;

export type Thread = {
  /** oldest first */
  messages: ChatMessage[];
  loading: boolean;
  /** Deputy has older messages stored */
  hasMore: boolean;
  loadingOlder: boolean;
  /** waiting for the phone to send older messages */
  askingPhone: boolean;
  /** the phone sent nothing older (or could not be asked) */
  phoneError: string;
  /** nothing older on the phone either */
  reachedStart: boolean;
  error: string;
  /** the chat whose first page is loaded ("" while loading) */
  loadedFor: string;
  loadOlder: () => Promise<void>;
  /** loads older pages until the message is there; false when it is not found */
  ensure: (id: string) => Promise<boolean>;
  /** adds or replaces messages (e.g. one just sent) */
  upsert: (m: ChatMessage) => void;
};

/** useThread keeps one chat's messages: newest page first, older pages on demand, live changes. */
export function useThread(jid: string, onNew?: (m: ChatMessage) => void): Thread {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [askingPhone, setAskingPhone] = useState(false);
  const [phoneError, setPhoneError] = useState("");
  const [reachedStart, setReachedStart] = useState(false);
  const [error, setError] = useState("");
  const [loadedFor, setLoadedFor] = useState("");
  const msgsRef = useRef<ChatMessage[]>([]);
  // merge updates the ref at once, so ensure() sees pages before React renders them.
  const merge = useCallback((fn: (ms: ChatMessage[]) => ChatMessage[]) => {
    const next = fn(msgsRef.current);
    msgsRef.current = next;
    setMessages(next);
  }, []);
  const state = useRef({ cursor: "", hasMore: false, busy: false, jid });
  state.current.jid = jid;
  const onNewRef = useRef(onNew);
  onNewRef.current = onNew;

  useEffect(() => {
    let alive = true;
    msgsRef.current = [];
    setMessages([]);
    setLoading(true);
    setLoadedFor("");
    setError("");
    setAskingPhone(false);
    setPhoneError("");
    setReachedStart(false);
    state.current = { cursor: "", hasMore: false, busy: false, jid };
    inbox
      .messages(jid, "", PAGE)
      .then((p) => {
        if (!alive) return;
        merge(() => mergeMessages([], [...p.messages].reverse()));
        state.current.cursor = p.cursor;
        state.current.hasMore = p.hasMore;
        setHasMore(p.hasMore);
      })
      .catch((e) => alive && setError(errText(e)))
      .finally(() => {
        if (!alive) return;
        setLoading(false);
        setLoadedFor(jid);
      });

    const offs = [
      on(inboxEvents.message, (ev: MessageEvent) => {
        if (ev.chat !== jid) return;
        const isNew = ev.change === "new" && !msgsRef.current.some((m) => m.id === ev.message.id);
        merge((ms) => mergeMessages(ms, [ev.message]));
        if (isNew) onNewRef.current?.(ev.message);
      }),
      on(inboxEvents.receipt, (ev: ReceiptEvent) => {
        if (ev.chat !== jid) return;
        const ids = new Set(ev.ids);
        merge((ms) => ms.map((m) => (ids.has(m.id) && m.fromMe ? { ...m, status: ev.status } : m)));
      }),
      on(inboxEvents.history, (ev: HistoryEvent) => {
        if (ev.chat !== jid) return;
        setAskingPhone(false);
        if (ev.error) {
          setPhoneError(ev.error);
          return;
        }
        if (ev.kind === "on-demand" && ev.added === 0) {
          setReachedStart(true);
          return;
        }
        // Older messages are stored now: page back from the oldest one shown.
        const oldest = msgsRef.current[0];
        if (!oldest) return;
        inbox
          .messages(jid, cursorOf(oldest), PAGE)
          .then((p) => {
            if (state.current.jid !== jid) return;
            merge((ms) => mergeMessages(ms, p.messages));
            state.current.cursor = p.cursor;
            state.current.hasMore = p.hasMore;
                setHasMore(p.hasMore);
            if (p.messages.length === 0 && ev.kind === "on-demand") setReachedStart(true);
          })
          .catch(() => {});
      }),
    ];
    return () => {
      alive = false;
      offs.forEach((f) => f());
    };
  }, [jid]);

  const loadOlder = useCallback(async () => {
    const s = state.current;
    if (s.busy) return;
    if (s.hasMore && s.cursor) {
      s.busy = true;
      setLoadingOlder(true);
      try {
        const p = await inbox.messages(jid, s.cursor, PAGE);
        if (state.current.jid !== jid) return;
        merge((ms) => mergeMessages(ms, p.messages));
        s.cursor = p.cursor;
        s.hasMore = p.hasMore;
        setHasMore(p.hasMore);
      } catch (e) {
        setError(errText(e));
      } finally {
        s.busy = false;
        setLoadingOlder(false);
      }
      return;
    }
    if (msgsRef.current.length === 0) return;
    setPhoneError("");
    setAskingPhone(true);
    try {
      await inbox.loadOlder(jid);
    } catch (e) {
      setAskingPhone(false);
      setPhoneError(errText(e));
    }
  }, [jid]);

  const ensure = useCallback(
    async (id: string) => {
      for (let i = 0; i < 20; i++) {
        if (msgsRef.current.some((m) => m.id === id)) return true;
        const s = state.current;
        if (!s.hasMore || !s.cursor || state.current.jid !== jid) return false;
        await loadOlder();
      }
      return msgsRef.current.some((m) => m.id === id);
    },
    [jid, loadOlder],
  );

  const upsert = useCallback((m: ChatMessage) => {
    if (m.chatJid && m.chatJid !== state.current.jid) return;
    merge((ms) => mergeMessages(ms, [m]));
  }, [merge]);

  return { messages, loading, loadedFor, hasMore, loadingOlder, askingPhone, phoneError, reachedStart, error, loadOlder, ensure, upsert };
}
