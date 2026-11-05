import { bytes, clock, duration, initials, plural, secondsLeft, when } from "./format";

describe("format", () => {
  const now = new Date(2026, 10, 4, 15, 0, 0).getTime();

  it("shows today's times as a clock", () => {
    expect(clock(new Date(2026, 10, 4, 14, 3).getTime())).toBe("14:03");
    expect(when(new Date(2026, 10, 4, 9, 5).getTime(), now)).toBe("09:05");
  });

  it("names yesterday and older days", () => {
    expect(when(new Date(2026, 10, 3, 23, 59).getTime(), now)).toBe("Yesterday");
    expect(when(new Date(2026, 9, 1, 12, 0).getTime(), now)).toBe("1 Oct");
    expect(when(0, now)).toBe("");
  });

  it("formats sizes, durations and counts", () => {
    expect(bytes(412_000_000)).toBe("412 MB");
    expect(bytes(1_100_000_000)).toBe("1.1 GB");
    expect(duration(24)).toBe("0:24");
    expect(duration(125)).toBe("2:05");
    expect(plural(1, "member")).toBe("1 member");
    expect(plural(214, "member")).toBe("214 members");
    expect(secondsLeft(now + 13_200, now)).toBe(14);
    expect(secondsLeft(now - 5000, now)).toBe(0);
  });

  it("makes initials from chat names", () => {
    expect(initials("Fraude nas urnas")).toBe("FU");
    expect(initials("Mãe")).toBe("M");
    expect(initials("BHLabs · time")).toBe("BT");
    expect(initials("")).toBe("?");
  });
});
