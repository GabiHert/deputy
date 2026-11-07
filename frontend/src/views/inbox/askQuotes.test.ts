import { describe, expect, it } from "vitest";
import type { ChatMessage } from "../../api";
import { quoteOf, quotesForSelection } from "./askQuotes";

const msg = (p: Partial<ChatMessage>): ChatMessage => ({ id: "x", kind: "text", text: "", at: 1, fromMe: false, senderName: "Luis", ...p }) as ChatMessage;

describe("ask quotes", () => {
  it("quotes a message with who said it", () => {
    expect(quoteOf(msg({ id: "a", text: "oi" }))).toEqual({ id: "a", who: "Luis", at: 1, text: "oi" });
    expect(quoteOf(msg({ id: "b", fromMe: true, text: "ok" })).who).toBe("You");
    expect(quoteOf(msg({ id: "c", kind: "voice", transcript: "falei", media: { seconds: 5 } } as any)).text).toContain("falei");
  });
  it("keeps the highlighted words inside one message, whole messages across several", () => {
    const byId = new Map([
      ["a", msg({ id: "a", at: 1, text: "o aviso apareceu muito rápido" })],
      ["b", msg({ id: "b", at: 2, text: "e não parou o sistema" })],
    ]);
    expect(quotesForSelection(["a"], "muito rápido", byId)[0].text).toBe("muito rápido");
    expect(quotesForSelection(["a", "b"], "rápido e não", byId).map((q) => q.text)).toEqual([
      "o aviso apareceu muito rápido",
      "e não parou o sistema",
    ]);
  });
});
