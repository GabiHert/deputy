/** The CSP every custom block frame gets: no network, inline code only. */
export const FRAME_CSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:";

const THEME_VARS = ["--bg", "--card", "--fg", "--fg2", "--fg3", "--border", "--accent", "--on-accent", "--ok", "--warn", "--bad", "--sel", "--wave"];

/** themeVars reads the app's color tokens so a frame matches light and dark mode. */
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
window.deputy={
query:function(name,params){return new Promise(function(res,rej){var id=++n;pending[id]={res:res,rej:rej};parent.postMessage({deputy:"query",id:id,name:String(name),params:params||{}},"*");});},
onUpdate:function(fn){if(typeof fn==="function")subs.push(fn);}
};
window.addEventListener("message",function(e){
if(e.source!==parent)return;var m=e.data||{};
if(m.deputy==="result"&&pending[m.id]){var p=pending[m.id];delete pending[m.id];if(m.error)p.rej(new Error(m.error));else p.res(m.result);}
if(m.deputy==="update"){window.data=m.data;subs.forEach(function(f){try{f(m.data);}catch(_){}});}
});
})();`;

/** buildSrcdoc makes the sandboxed document for a custom block. */
export function buildSrcdoc(opts: { html: string; js?: string; data: unknown; theme?: Record<string, string> }): string {
  const theme = Object.entries(opts.theme ?? {})
    .filter(([k]) => /^--[a-z0-9-]+$/.test(k))
    .map(([k, v]) => `${k}:${cssValue(v)};`)
    .join("");
  const js = (opts.js ?? "").replace(/<\/script/gi, "<\\/script");
  return [
    "<!doctype html><html><head>",
    `<meta http-equiv="Content-Security-Policy" content="${FRAME_CSP}">`,
    '<meta charset="utf-8">',
    `<style>:root{${theme}}html,body{margin:0;background:transparent;color:var(--fg);font:13px/1.45 -apple-system,"IBM Plex Sans",sans-serif}</style>`,
    `<script>window.data=${jsonForScript(opts.data)};${BRIDGE}</script>`,
    "</head><body>",
    opts.html ?? "",
    js ? `<script>${js}</script>` : "",
    "</body></html>",
  ].join("");
}
