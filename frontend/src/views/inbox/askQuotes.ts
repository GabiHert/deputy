import type { ChatMessage } from "../../api";
import type { Quote } from "../../lib/askContext";
import { kindLabel } from "./helpers";

/** A whole message as a quote for the agent. */
export function quoteOf(m: ChatMessage): Quote {
  const who = m.fromMe ? (m.fromAgent ? m.agentName || "Agent" : "You") : m.senderName || "Someone";
  const media = m.kind !== "text" ? `[${kindLabel(m.kind, m.media?.seconds ?? 0, m.media?.fileName ?? "")}]` : "";
  const text = [media, m.text || m.transcript].filter(Boolean).join(" ");
  return { id: m.id, who, at: m.at, text };
}

/**
 * The quotes for a highlighted selection: the selected words when it sits inside one message,
 * otherwise every message it touches, whole.
 */
export function quotesForSelection(ids: string[], selected: string, byId: Map<string, ChatMessage>): Quote[] {
  const msgs = ids.map((id) => byId.get(id)).filter((m): m is ChatMessage => !!m);
  if (msgs.length === 1) {
    const q = quoteOf(msgs[0]);
    const part = selected.trim();
    return [part && part.length < q.text.length ? { ...q, text: part } : q];
  }
  return msgs.map(quoteOf);
}
