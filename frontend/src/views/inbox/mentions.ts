import { useEffect, useState } from "react";
import { inbox, type Member } from "../../api";

/** The user part of a JID: "5551999@s.whatsapp.net" → "5551999". */
export const userOf = (jid: string) => jid.split("@")[0].split(":")[0];

/** The @word being typed at the caret, if any: where its @ is and the text after it. */
export function mentionAt(text: string, caret: number): { start: number; query: string } | null {
  const before = text.slice(0, caret);
  const at = before.lastIndexOf("@");
  if (at < 0) return null;
  if (at > 0 && !/\s/.test(before[at - 1])) return null;
  const query = before.slice(at + 1);
  if (/[\s@]/.test(query) || query.length > 30) return null;
  return { start: at, query };
}

/** Members whose name or number matches the query, best first, never the owner. */
export function matchMembers(members: Member[], query: string, limit = 8): Member[] {
  const q = query.toLowerCase();
  const scored = members
    .filter((m) => !m.me)
    .map((m) => {
      const name = m.name.toLowerCase();
      const words = name.split(/\s+/);
      let score = -1;
      if (!q) score = 1;
      else if (name.startsWith(q)) score = 3;
      else if (words.some((w) => w.startsWith(q))) score = 2;
      else if (name.includes(q) || m.phone.replace(/\D/g, "").includes(q)) score = 1;
      return { m, score };
    })
    .filter((x) => x.score > 0);
  scored.sort((a, b) => b.score - a.score || a.m.name.localeCompare(b.m.name));
  return scored.slice(0, limit).map((x) => x.m);
}

/** Turns the "@Name" the owner sees into WhatsApp's "@number" and lists who is mentioned. */
export function encodeMentions(text: string, picked: { name: string; jid: string }[]): { text: string; mentions: string[] } {
  const mentions: string[] = [];
  let out = text;
  // Longest names first, so "@Ana Paula" is not taken as "@Ana".
  for (const p of [...picked].sort((a, b) => b.name.length - a.name.length)) {
    const token = "@" + p.name;
    if (!out.includes(token)) continue;
    out = out.split(token).join("@" + userOf(p.jid));
    if (!mentions.includes(p.jid)) mentions.push(p.jid);
  }
  return { text: out, mentions };
}

/** Splits text into plain parts and @mentions shown by name. */
export function mentionParts(text: string, mentions: string[], members: Member[]): { text: string; mention?: boolean }[] {
  if (!mentions?.length) return [{ text }];
  const names = new Map<string, string>();
  for (const j of mentions) names.set(userOf(j), "");
  for (const m of members) for (const u of m.users) if (names.has(u)) names.set(u, m.name);
  const parts: { text: string; mention?: boolean }[] = [];
  let last = 0;
  for (const hit of text.matchAll(/@(\d{5,})/g)) {
    if (!names.has(hit[1])) continue;
    const i = hit.index ?? 0;
    if (i > last) parts.push({ text: text.slice(last, i) });
    const name = names.get(hit[1]) || hit[0];
    parts.push({ text: name.startsWith("@") ? name : "@" + name, mention: true });
    last = i + hit[0].length;
  }
  if (last < text.length) parts.push({ text: text.slice(last) });
  return parts;
}

const cache = new Map<string, Promise<Member[]>>();

/** A group's members, loaded once per chat while the app runs ([] for direct chats or on error). */
export function useMembers(chat: string): Member[] {
  const [list, setList] = useState<Member[]>([]);
  useEffect(() => {
    setList([]);
    if (!chat.endsWith("@g.us")) return;
    let p = cache.get(chat);
    if (!p) {
      p = inbox.members(chat).catch(() => {
        cache.delete(chat);
        return [] as Member[];
      });
      cache.set(chat, p);
    }
    let live = true;
    p.then((m) => live && setList(m));
    return () => {
      live = false;
    };
  }, [chat]);
  return list;
}
