const pad = (n: number) => String(n).padStart(2, "0");

/** clock gives "14:03". */
export function clock(ms: number): string {
  const d = new Date(ms);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** when gives "14:03" today, "Yesterday", the weekday this week, else "4 Nov". "" for 0. */
export function when(ms: number, now: number = Date.now()): string {
  if (!ms) return "";
  const days = Math.round((startOfDay(new Date(now)) - startOfDay(new Date(ms))) / 86400000);
  if (days <= 0) return clock(ms);
  if (days === 1) return "Yesterday";
  const d = new Date(ms);
  if (days < 7) return d.toLocaleDateString("en", { weekday: "long" });
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

/** bytes gives "412 MB". */
export function bytes(n: number): string {
  if (!n || n < 0) return "0 KB";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

/** duration gives "0:24". */
export function duration(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  return `${Math.floor(s / 60)}:${pad(s % 60)}`;
}

/** initials gives "FU" for "Fraude nas urnas", "M" for "Mãe". */
export function initials(name: string): string {
  const words = name.replace(/[^\p{L}\p{N}\s]/gu, " ").split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  const big = words.filter((w) => w.length > 3);
  const use = big.length >= 2 ? big : words;
  return (use[0][0] + (use.length > 1 ? use[1][0] : "")).toUpperCase();
}

/** plural gives "1 member" or "9 members". */
export function plural(n: number, word: string): string {
  return `${n.toLocaleString("en")} ${word}${n === 1 ? "" : "s"}`;
}

/** secondsLeft gives whole seconds until ms, never negative. */
export function secondsLeft(ms: number, now: number = Date.now()): number {
  return Math.max(0, Math.ceil((ms - now) / 1000));
}

/** ago gives "just now", "5 minutes ago", "3 hours ago", "2 days ago", else the date. "" for 0. */
export function ago(ms: number, now: number = Date.now()): string {
  if (!ms) return "";
  const m = Math.floor((now - ms) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${plural(m, "minute")} ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${plural(h, "hour")} ago`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${plural(d, "day")} ago`;
  return new Date(ms).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

/** tokens shows a token count short: 950, 4.3k, 33k, 1.2M. */
export function tokens(n: number): string {
  if (n < 1000) return String(Math.round(n));
  if (n < 1e6) return `${(n / 1000).toFixed(n < 1e4 ? 1 : 0).replace(/\.0$/, "")}k`;
  return `${(n / 1e6).toFixed(1).replace(/\.0$/, "")}M`;
}

/** usd shows a cost in dollars, with a third decimal for amounts under a cent. */
export function usd(n: number): string {
  if (n <= 0) return "$0";
  if (n < 0.01) return `$${n.toFixed(3)}`;
  return `$${n.toFixed(2)}`;
}

/** usageLine is one short line about what a run used. */
export function usageLine(u: { tokens: number; cacheRead: number; costUsd: number }): string {
  const cached = u.tokens > 0 ? Math.round((u.cacheRead / u.tokens) * 100) : 0;
  return `${tokens(u.tokens)} tokens · ${cached}% from cache · ≈ ${usd(u.costUsd)}`;
}
