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
