import { ago, bytes, clock, duration, initials, plural, secondsLeft, tokens, usageLine, usd, when } from "./format";

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

  it("says how long ago a commit was", () => {
    expect(ago(now - 20_000, now)).toBe("just now");
    expect(ago(now - 5 * 60_000, now)).toBe("5 minutes ago");
    expect(ago(now - 60 * 60_000, now)).toBe("1 hour ago");
    expect(ago(now - 3 * 86_400_000, now)).toBe("3 days ago");
    expect(ago(new Date(2026, 0, 2).getTime(), now)).toBe("2 Jan 2026");
    expect(ago(0, now)).toBe("");
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

  it("shows token counts short", () => {
    expect(tokens(950)).toBe("950");
    expect(tokens(1000)).toBe("1k");
    expect(tokens(33125)).toBe("33k");
    expect(tokens(4250)).toBe("4.3k");
    expect(tokens(1_200_000)).toBe("1.2M");
  });

  it("shows costs in dollars", () => {
    expect(usd(0)).toBe("$0");
    expect(usd(0.0046)).toBe("$0.005");
    expect(usd(0.0512)).toBe("$0.05");
    expect(usd(3.2)).toBe("$3.20");
  });

  it("sums a run's usage up in one line", () => {
    expect(usageLine({ tokens: 33625, cacheRead: 11382, costUsd: 0.0459 })).toBe("34k tokens · 34% from cache · ≈ $0.05");
    expect(usageLine({ tokens: 0, cacheRead: 0, costUsd: 0 })).toBe("0 tokens · 0% from cache · ≈ $0");
  });
});
