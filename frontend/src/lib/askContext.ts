import { useEffect, useState } from "react";

/** A piece of a WhatsApp conversation the owner picked to ask an agent about. */
export type Quote = {
  /** message id; "" for a highlighted excerpt that spans messages */
  id: string;
  who: string;
  at: number;
  text: string;
};

export type AskContext = { chatName: string; quotes: Quote[] };

const store = new Map<string, AskContext>(); // Claude chat id → what is attached to its next message
const subs = new Set<() => void>();
const changed = () => subs.forEach((f) => f());

/** Attaches quotes to the next message of Claude chat chatId, keeping what is there and skipping repeats. */
export function attachQuotes(chatId: string, chatName: string, quotes: Quote[]) {
  const cur = store.get(chatId) ?? { chatName, quotes: [] };
  const key = (q: Quote) => q.id || q.text;
  const seen = new Set(cur.quotes.map(key));
  const next = [...cur.quotes, ...quotes.filter((q) => q.text.trim() && !seen.has(key(q)))];
  next.sort((a, b) => a.at - b.at);
  store.set(chatId, { chatName, quotes: next });
  changed();
}

export function removeQuote(chatId: string, i: number) {
  const cur = store.get(chatId);
  if (!cur) return;
  const quotes = cur.quotes.filter((_, j) => j !== i);
  if (quotes.length) store.set(chatId, { ...cur, quotes });
  else store.delete(chatId);
  changed();
}

/** What is attached to chatId's next message, outside React. */
export function peekQuotes(chatId: string): AskContext | null {
  return store.get(chatId) ?? null;
}

export function clearQuotes(chatId: string) {
  if (store.delete(chatId)) changed();
}

export function useAskContext(chatId: string): AskContext | null {
  const [, tick] = useState(0);
  useEffect(() => {
    const f = () => tick((n) => n + 1);
    subs.add(f);
    return () => {
      subs.delete(f);
    };
  }, []);
  return store.get(chatId) ?? null;
}

const clock = (at: number) =>
  at ? new Date(at).toLocaleString([], { weekday: "short", hour: "2-digit", minute: "2-digit" }) : "";

/** The quotes as the block that goes before the owner's question. */
export function formatQuotes(ctx: AskContext): string {
  const lines = ctx.quotes.map((q) => {
    const head = [q.who, clock(q.at)].filter(Boolean).join(", ");
    const body = q.text.trim().replace(/\n/g, "\n> ");
    return `> ${head ? `**${head}:** ` : ""}${body}${q.id ? ` _(message ${q.id})_` : ""}`;
  });
  const what = ctx.quotes.length === 1 ? "this message" : `these ${ctx.quotes.length} messages`;
  return `About ${what} from the WhatsApp chat with ${ctx.chatName}:\n\n${lines.join("\n>\n")}`;
}

/** A short label for the attachment chip. */
export function quotesLabel(ctx: AskContext): string {
  const n = ctx.quotes.length;
  if (n === 1) {
    const t = ctx.quotes[0].text.trim().replace(/\s+/g, " ");
    return `“${t.length > 40 ? t.slice(0, 39) + "…" : t}”`;
  }
  return `${n} messages from ${ctx.chatName}`;
}
