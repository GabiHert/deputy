import { describe, expect, it } from "vitest";
import { encodeMentions, matchMembers, mentionAt, mentionParts } from "./mentions";

const members = [
  { jid: "111@lid", name: "Ana Paula", phone: "+5551111", users: ["111", "5551111"], me: false, admin: false },
  { jid: "222@lid", name: "Ana", phone: "", users: ["222"], me: false, admin: false },
  { jid: "333@lid", name: "Bruno Lima", phone: "+5551333", users: ["333"], me: false, admin: false },
  { jid: "999@lid", name: "Gabriel", phone: "", users: ["999"], me: true, admin: false },
];

describe("mentions", () => {
  it("finds the @word at the caret", () => {
    expect(mentionAt("oi @an", 6)).toEqual({ start: 3, query: "an" });
    expect(mentionAt("@", 1)).toEqual({ start: 0, query: "" });
    expect(mentionAt("mail@x", 6)).toBeNull();
    expect(mentionAt("@ana oi", 7)).toBeNull();
  });
  it("matches members, never the owner", () => {
    expect(matchMembers(members, "li").map((m) => m.name)).toEqual(["Bruno Lima"]);
    expect(matchMembers(members, "ana").map((m) => m.name)).toEqual(["Ana", "Ana Paula"]);
    expect(matchMembers(members, "").some((m) => m.me)).toBe(false);
  });
  it("encodes names as numbers, longest first", () => {
    const r = encodeMentions("oi @Ana Paula e @Ana", [
      { name: "Ana", jid: "222@lid" },
      { name: "Ana Paula", jid: "111@lid" },
    ]);
    expect(r.text).toBe("oi @111 e @222");
    expect(r.mentions.sort()).toEqual(["111@lid", "222@lid"]);
    expect(encodeMentions("sem ninguém", [{ name: "Ana", jid: "222@lid" }]).mentions).toEqual([]);
  });
  it("shows incoming mentions by name", () => {
    const parts = mentionParts("valeu @5551111!", ["5551111@s.whatsapp.net"], members);
    expect(parts).toEqual([{ text: "valeu " }, { text: "@Ana Paula", mention: true }, { text: "!" }]);
    expect(mentionParts("@123456 oi", ["123456@lid"], [])).toEqual([{ text: "@123456", mention: true }, { text: " oi" }]);
  });
});
