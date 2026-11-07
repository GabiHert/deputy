import { describe, expect, it } from "vitest";
import { attachQuotes, clearQuotes, formatQuotes, peekQuotes, quotesLabel, removeQuote } from "./askContext";

describe("ask context", () => {
  it("collects quotes in time order without repeats", () => {
    attachQuotes("c1", "Brienze", [{ id: "m2", who: "You", at: 2000, text: "que aviso?" }]);
    attachQuotes("c1", "Brienze", [
      { id: "m1", who: "Luis", at: 1000, text: "Tudo app\nnossa" },
      { id: "m2", who: "You", at: 2000, text: "que aviso?" },
      { id: "m3", who: "Luis", at: 3000, text: "   " },
    ]);
    expect(peekQuotes("c1")?.quotes.map((q) => q.id)).toEqual(["m1", "m2"]);
    removeQuote("c1", 0);
    expect(peekQuotes("c1")?.quotes.map((q) => q.id)).toEqual(["m2"]);
    removeQuote("c1", 0);
    expect(peekQuotes("c1")).toBeNull();
    attachQuotes("c1", "Brienze", [{ id: "m9", who: "Luis", at: 1, text: "x" }]);
    clearQuotes("c1");
    expect(peekQuotes("c1")).toBeNull();
  });

  it("formats them before the question", () => {
    const ctx = {
      chatName: "Brienze",
      quotes: [
        { id: "m1", who: "Luis", at: 1000, text: "Tudo app\nnossa" },
        { id: "m2", who: "You", at: 2000, text: "que aviso?" },
      ],
    };
    const out = formatQuotes(ctx);
    expect(out.startsWith("About these 2 messages from the WhatsApp chat with Brienze:")).toBe(true);
    expect(out).toContain("> **Luis");
    expect(out).toContain("Tudo app\n> nossa");
    expect(out).toContain("_(message m2)_");
    expect(quotesLabel(ctx)).toBe("2 messages from Brienze");
    expect(quotesLabel({ chatName: "B", quotes: [ctx.quotes[1]] })).toBe("“que aviso?”");
  });
});
