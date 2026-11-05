import type { AssistantChat } from "../api";
import { filterChats, groupChats } from "./chatgroups";

const chat = (id: string, at: number, title = id): AssistantChat => ({ id, title, purpose: "general", agentId: "", preview: "", createdAt: at, updatedAt: at, busy: false });

describe("groupChats", () => {
  const now = new Date(2026, 10, 4, 15, 0).getTime();
  it("groups by day, newest first, and drops empty groups", () => {
    const g = groupChats(
      [chat("old", new Date(2026, 9, 20).getTime()), chat("t1", new Date(2026, 10, 4, 9).getTime()), chat("t2", new Date(2026, 10, 4, 14).getTime()), chat("y", new Date(2026, 10, 3, 23).getTime())],
      now,
    );
    expect(g.map((x) => x.label)).toEqual(["Today", "Yesterday", "Earlier"]);
    expect(g[0].chats.map((c) => c.id)).toEqual(["t2", "t1"]);
    expect(groupChats([chat("y", new Date(2026, 10, 3, 1).getTime())], now).map((x) => x.label)).toEqual(["Yesterday"]);
  });
  it("filters by title", () => {
    expect(filterChats([chat("a", 1, "Tom do Checagem"), chat("b", 1, "Família")], "checa").map((c) => c.id)).toEqual(["a"]);
  });
});
