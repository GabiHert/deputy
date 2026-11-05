import type { AssistantChat } from "../api";

function startOfDay(ms: number): number {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

export type ChatGroup = { label: "Today" | "Yesterday" | "Earlier"; chats: AssistantChat[] };

/** groupChats sorts chats newest first and groups them by the day they last changed. */
export function groupChats(chats: AssistantChat[], now: number = Date.now()): ChatGroup[] {
  const today = startOfDay(now);
  const yesterday = today - 86_400_000;
  const groups: ChatGroup[] = [
    { label: "Today", chats: [] },
    { label: "Yesterday", chats: [] },
    { label: "Earlier", chats: [] },
  ];
  [...chats]
    .sort((a, b) => (b.updatedAt || b.createdAt) - (a.updatedAt || a.createdAt))
    .forEach((c) => {
      const t = c.updatedAt || c.createdAt;
      groups[t >= today ? 0 : t >= yesterday ? 1 : 2].chats.push(c);
    });
  return groups.filter((g) => g.chats.length > 0);
}

/** filterChats keeps chats whose title or preview contains the search text. */
export function filterChats(chats: AssistantChat[], q: string): AssistantChat[] {
  const s = q.trim().toLocaleLowerCase();
  if (!s) return chats;
  return chats.filter((c) => (c.title + " " + c.preview).toLocaleLowerCase().includes(s));
}
