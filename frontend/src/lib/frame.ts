/** The CSP every custom block frame gets: no network, inline code only. */
export const FRAME_CSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:";

const THEME_VARS = [
  "--bg", "--bar", "--side", "--card", "--fg", "--fg2", "--fg3", "--fg3-text", "--border", "--row-line",
  "--accent", "--on-accent", "--ok", "--ok-bg", "--warn", "--warn-bg", "--bad", "--bad-bg", "--sel",
  "--muted-bg", "--wave", "--rec", "--rec-bg",
];

/** themeScheme says whether the app shows its light or dark theme: the data-theme setting wins,
 * then the system appearance. */
export function themeScheme(): "light" | "dark" {
  if (typeof document === "undefined") return "light";
  const set = document.documentElement.getAttribute("data-theme");
  if (set === "light" || set === "dark") return set;
  return typeof matchMedia === "function" && matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** watchTheme calls onChange when the app's theme can have changed: the system appearance flips,
 * or the data-theme setting changes. It returns the function that stops watching. */
export function watchTheme(onChange: () => void): () => void {
  if (typeof document === "undefined") return () => {};
  const mo = new MutationObserver(onChange);
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "class", "style"] });
  const mq = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: dark)") : null;
  mq?.addEventListener?.("change", onChange);
  return () => {
    mo.disconnect();
    mq?.removeEventListener?.("change", onChange);
  };
}

/** themeMessage is what the board posts to a frame so it follows the app's current theme. */
export function themeMessage(): { positron: "theme"; vars: Record<string, string>; scheme: "light" | "dark" } {
  return { positron: "theme", vars: themeVars(), scheme: themeScheme() };
}

/** themeVars reads the app's color tokens so a frame matches light and dark mode. They are read
 * when the frame is built and again whenever the theme changes (see watchTheme). */
export function themeVars(): Record<string, string> {
  const out: Record<string, string> = {};
  if (typeof document === "undefined") return out;
  const cs = getComputedStyle(document.documentElement);
  for (const v of THEME_VARS) {
    const val = cs.getPropertyValue(v).trim();
    if (val) out[v] = val;
  }
  return out;
}

/** jsonForScript serializes data so it is safe inside a <script> element. */
export function jsonForScript(data: unknown): string {
  return (JSON.stringify(data ?? null) ?? "null")
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

function cssValue(v: string): string {
  return v.replace(/[<>{};]/g, "");
}

const BRIDGE = `(function(){
var pending={},n=0,subs=[];
function act(action,runId){return new Promise(function(res,rej){var id=++n;pending[id]={res:res,rej:rej};parent.postMessage({positron:"action",id:id,action:action,runId:String(runId||"")},"*");});}
window.positron={
query:function(name,params){return new Promise(function(res,rej){var id=++n;pending[id]={res:res,rej:rej};parent.postMessage({positron:"query",id:id,name:String(name),params:params||{}},"*");});},
dismissRun:function(runId){return act("dismissRun",runId);},
restoreRun:function(runId){return act("restoreRun",runId);},
onUpdate:function(fn){if(typeof fn==="function")subs.push(fn);}
};
window.deputy=window.positron;
window.addEventListener("message",function(e){
if(e.source!==parent)return;var m=e.data||{};
if(m.positron==="result"&&pending[m.id]){var p=pending[m.id];delete pending[m.id];if(m.error)p.rej(new Error(m.error));else p.res(m.result);}
if(m.positron==="theme"){var r=document.documentElement,v=m.vars||{};for(var k in v){if(/^--[a-z0-9-]+$/.test(k))r.style.setProperty(k,String(v[k]).replace(/[<>{};]/g,""));}if(m.scheme==="dark"||m.scheme==="light")r.style.colorScheme=m.scheme;}
if(m.positron==="update"){window.data=m.data;subs.forEach(function(f){try{f(m.data);}catch(_){}});}
});
})();`;

/** buildSrcdoc makes the sandboxed document for a custom block. */
export function buildSrcdoc(opts: {
  html: string;
  js?: string;
  data: unknown;
  theme?: Record<string, string>;
  scheme?: "light" | "dark";
}): string {
  const theme = Object.entries(opts.theme ?? {})
    .filter(([k]) => /^--[a-z0-9-]+$/.test(k))
    .map(([k, v]) => `${k}:${cssValue(v)};`)
    .join("");
  const js = (opts.js ?? "").replace(/<\/script/gi, "<\\/script");
  return [
    "<!doctype html><html><head>",
    `<meta http-equiv="Content-Security-Policy" content="${FRAME_CSP}">`,
    '<meta charset="utf-8">',
    // color-scheme and the CanvasText fallback keep text readable even if a variable is missing.
    `<style>:root{color-scheme:${opts.scheme === "dark" ? "dark" : opts.scheme === "light" ? "light" : "light dark"};${theme}}html,body{margin:0;background:transparent;color:var(--fg,CanvasText);font:13px/1.45 -apple-system,"IBM Plex Sans",sans-serif}</style>`,
    `<script>window.data=${jsonForScript(opts.data)};${BRIDGE}</script>`,
    "</head><body>",
    opts.html ?? "",
    js ? `<script>${js}</script>` : "",
    "</body></html>",
  ].join("");
}
