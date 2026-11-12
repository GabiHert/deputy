import { FRAME_CSP, buildSrcdoc, jsonForScript, themeMessage, themeScheme, watchTheme } from "./frame";

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

describe("frame theme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.style.cssText = "";
  });

  it("falls back to system text colours and declares the colour scheme", () => {
    const dark = buildSrcdoc({ html: "", data: null, theme: { "--fg": "#ece8df" }, scheme: "dark" });
    expect(dark).toContain("color-scheme:dark;");
    expect(dark).toContain("--fg:#ece8df;");
    expect(dark).toContain("color:var(--fg,CanvasText)");
    expect(buildSrcdoc({ html: "", data: null })).toContain("color-scheme:light dark;");
  });

  it("lets a frame take new theme colours after it was built", () => {
    const doc = buildSrcdoc({ html: "", data: null });
    expect(doc).toContain('m.positron==="theme"');
    expect(doc).toContain("r.style.setProperty(k,");
  });

  it("reads the data-theme setting before the system appearance", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    expect(themeScheme()).toBe("dark");
    document.documentElement.setAttribute("data-theme", "light");
    expect(themeScheme()).toBe("light");
  });

  it("posts --fg and --fg2 with the scheme", () => {
    document.documentElement.style.setProperty("--fg", "#ece8df");
    document.documentElement.style.setProperty("--fg2", "#bab6ab");
    document.documentElement.setAttribute("data-theme", "dark");
    const m = themeMessage();
    expect(m.positron).toBe("theme");
    expect(m.scheme).toBe("dark");
    expect(m.vars["--fg"]).toBe("#ece8df");
    expect(m.vars["--fg2"]).toBe("#bab6ab");
  });

  it("calls back when the theme setting changes, until stopped", async () => {
    let n = 0;
    const stop = watchTheme(() => n++);
    document.documentElement.setAttribute("data-theme", "dark");
    await Promise.resolve();
    expect(n).toBe(1);
    stop();
    document.documentElement.setAttribute("data-theme", "light");
    await Promise.resolve();
    expect(n).toBe(1);
  });
});

describe("frame bridge", () => {
  it("applies a posted theme to the frame's root", async () => {
    const doc = buildSrcdoc({ html: "", data: null });
    const head = doc.slice(doc.indexOf("<script>") + "<script>".length, doc.indexOf("</script>"));
    // In the test window, parent === window, so the bridge accepts messages posted to itself.
    new Function(head)();
    // jsdom's postMessage leaves event.source empty, so send the event the way a parent would.
    const data = { positron: "theme", vars: { "--fg": "#ece8df", "--fg2": "#bab6ab", bad: "x" }, scheme: "dark" };
    window.dispatchEvent(new MessageEvent("message", { data, source: window }));
    await Promise.resolve();
    const root = document.documentElement.style;
    expect(root.getPropertyValue("--fg")).toBe("#ece8df");
    expect(root.getPropertyValue("--fg2")).toBe("#bab6ab");
    expect(root.getPropertyValue("bad")).toBe("");
    root.cssText = "";
  });
});
