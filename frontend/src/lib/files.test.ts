import { describe, expect, it } from "vitest";
import { blobToBase64 } from "./files";

describe("blobToBase64", () => {
  it("encodes bytes without a data URL prefix", async () => {
    const b = new Blob([new Uint8Array([0, 1, 2, 250, 255])]);
    expect(await blobToBase64(b)).toBe("AAEC+v8=");
  });
  it("handles large blobs in chunks", async () => {
    const big = new Uint8Array(100_000).fill(65);
    const out = await blobToBase64(new Blob([big]));
    expect(atob(out).length).toBe(100_000);
  });
});
