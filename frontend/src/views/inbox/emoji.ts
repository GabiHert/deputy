import { gemoji } from "gemoji";

export type EmojiHit = { emoji: string; name: string };

const byName = new Map<string, string>();
for (const g of gemoji) for (const n of g.names) byName.set(n, g.emoji);

/** The :code being typed at the caret: where its colon is and the letters after it (2 or more). */
export function emojiAt(text: string, caret: number): { start: number; query: string } | null {
  const m = /(^|[\s(])(:([a-z0-9_+-]{2,30}))$/i.exec(text.slice(0, caret));
  if (!m) return null;
  return { start: caret - m[2].length, query: m[3].toLowerCase() };
}

/** Emoji whose code starts with the query first, then codes, tags or descriptions containing it. */
export function matchEmoji(query: string, limit = 8): EmojiHit[] {
  const q = query.toLowerCase();
  const scored: { hit: EmojiHit; score: number }[] = [];
  for (const g of gemoji) {
    let best = 0;
    let name = g.names[0];
    for (const n of g.names) {
      const s = n === q ? 4 : n.startsWith(q) ? 3 : n.includes(q) ? 2 : 0;
      if (s > best) [best, name] = [s, n];
    }
    if (!best && (g.tags.some((t) => t.startsWith(q)) || g.description.includes(q))) best = 1;
    if (best) scored.push({ hit: { emoji: g.emoji, name }, score: best });
  }
  scored.sort((a, b) => b.score - a.score || a.hit.name.length - b.hit.name.length);
  return scored.slice(0, limit).map((s) => s.hit);
}

/** Turns a finished :code: just before the caret into its emoji. */
export function completeEmoji(text: string, caret: number): { text: string; caret: number } | null {
  const m = /(^|[\s(]):([a-z0-9_+-]{1,30}):$/i.exec(text.slice(0, caret));
  if (!m) return null;
  const emoji = byName.get(m[2].toLowerCase());
  if (!emoji) return null;
  const start = caret - m[2].length - 2;
  return { text: text.slice(0, start) + emoji + text.slice(caret), caret: start + emoji.length };
}
