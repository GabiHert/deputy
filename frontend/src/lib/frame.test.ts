import { FRAME_CSP, buildSrcdoc, jsonForScript } from "./frame";

describe("buildSrcdoc", () => {
  it("puts the CSP first and blocks the network", () => {
    const doc = buildSrcdoc({ html: "<div id=x></div>", data: { a: 1 } });
    const csp = doc.indexOf('http-equiv="Content-Security-Policy"');
    expect(csp).toBeGreaterThan(0);
    expect(csp).toBeLessThan(doc.indexOf("<script>"));
    expect(doc).toContain(FRAME_CSP);
    expect(FRAME_CSP).toContain("default-src 'none'");
    expect(FRAME_CSP).not.toContain("connect-src");
  });

  it("escapes data so it cannot close the script", () => {
    const data = { t: "</script><script>alert(1)</script>", u: "\u2028" };
    const doc = buildSrcdoc({ html: "", data });
    const head = doc.slice(0, doc.indexOf("</head>"));
    expect(head.match(/<\/script>/g)?.length).toBe(1);
    const json = jsonForScript(data);
    expect(json).not.toMatch(/[<>&\u2028]/);
    expect(JSON.parse(json)).toEqual(data);
  });

  it("keeps the block's own script from closing early and filters theme values", () => {
    const doc = buildSrcdoc({ html: "", js: "var s='</script>';", data: null, theme: { "--bg": "#fff;}</style><script>", "bad key": "x" } });
    expect(doc).toContain("<\\/script>");
    expect(doc).not.toContain("#fff;}</style>");
    expect(doc.match(/<\/style>/g)?.length).toBe(1);
    expect(doc).not.toContain("bad key");
  });

  it("gives blocks window.positron and keeps window.deputy for blocks written before the rename", () => {
    const doc = buildSrcdoc({ html: "", data: null });
    expect(doc).toContain("window.positron={");
    expect(doc).toContain("window.deputy=window.positron;");
  });
});
