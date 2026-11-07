import { describe, expect, it } from "vitest";
import { completeEmoji, emojiAt, matchEmoji } from "./emoji";

describe("emoji codes", () => {
  it("finds the :code at the caret", () => {
    expect(emojiAt("oi :smi", 7)).toEqual({ start: 3, query: "smi" });
    expect(emojiAt(":+1", 3)).toEqual({ start: 0, query: "+1" });
    expect(emojiAt("oi :s", 5)).toBeNull(); // one letter is too early
    expect(emojiAt("10:30", 5)).toBeNull(); // times are not codes
    expect(emojiAt("http://x", 8)).toBeNull();
  });
  it("matches by code first", () => {
    const hits = matchEmoji("smile");
    expect(hits[0]).toEqual({ emoji: "😄", name: "smile" });
    expect(matchEmoji("thumbs").map((h) => h.emoji)).toContain("👍");
    expect(matchEmoji("heart").length).toBe(8);
  });
  it("turns a finished :code: into its emoji", () => {
    expect(completeEmoji("oi :heart:", 10)).toEqual({ text: "oi ❤️", caret: 3 + "❤️".length });
    expect(completeEmoji(":+1: ok", 4)).toEqual({ text: "👍 ok", caret: "👍".length });
    expect(completeEmoji("oi :nope_nope:", 14)).toBeNull();
    expect(completeEmoji("10:30:", 6)).toBeNull();
  });
});
