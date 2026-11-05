import { compactDiff, lineDiff } from "./diff";

describe("lineDiff", () => {
  it("marks added, removed and kept lines", () => {
    const d = lineDiff("a\nb\nc", "a\nc\nd");
    expect(d).toEqual([
      { op: " ", text: "a" },
      { op: "-", text: "b" },
      { op: " ", text: "c" },
      { op: "+", text: "d" },
    ]);
  });

  it("handles empty sides", () => {
    expect(lineDiff("", "x\ny")).toEqual([
      { op: "+", text: "x" },
      { op: "+", text: "y" },
    ]);
    expect(lineDiff("x", "")).toEqual([{ op: "-", text: "x" }]);
    expect(lineDiff("same", "same")).toEqual([{ op: " ", text: "same" }]);
  });

  it("compacts long unchanged runs", () => {
    const before = ["1", "2", "3", "4", "5", "6", "7", "8"].join("\n");
    const after = ["1", "2", "3", "4", "5", "6", "7", "8!"].join("\n");
    const c = compactDiff(lineDiff(before, after), 1);
    expect(c[0]).toBeNull();
    expect(c.filter((l) => l && l.op !== " ").map((l) => l!.op)).toEqual(["-", "+"]);
    expect(c.filter((l) => l && l.op === " ").map((l) => l!.text)).toEqual(["7"]);
  });
});
