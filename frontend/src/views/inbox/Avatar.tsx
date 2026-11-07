import { useEffect, useRef, useState } from "react";
import { inbox } from "../../api";
import { tone } from "./helpers";

/* Chat pictures: fetched once per session, only when the avatar scrolls into view, a few at a time. */

const cache = new Map<string, string>(); // jid -> url ("" = none)
const waiting = new Map<string, ((url: string) => void)[]>();
const queue: string[] = [];
let active = 0;
const MAX = 4;

function pump() {
  while (active < MAX && queue.length > 0) {
    const jid = queue.shift()!;
    active++;
    inbox
      .avatar(jid)
      .catch(() => "")
      .then((url) => {
        cache.set(jid, url || "");
        const cbs = waiting.get(jid) ?? [];
        waiting.delete(jid);
        cbs.forEach((cb) => cb(url || ""));
      })
      .finally(() => {
        active--;
        pump();
      });
  }
}

function request(jid: string, cb: (url: string) => void): () => void {
  if (cache.has(jid)) {
    cb(cache.get(jid)!);
    return () => {};
  }
  const cbs = waiting.get(jid);
  if (cbs) cbs.push(cb);
  else {
    waiting.set(jid, [cb]);
    queue.push(jid);
    pump();
  }
  return () => {
    const list = waiting.get(jid);
    if (!list) return;
    const i = list.indexOf(cb);
    if (i >= 0) list.splice(i, 1);
    if (list.length === 0) {
      // Nobody is looking any more: drop it from the queue if it has not started.
      const q = queue.indexOf(jid);
      if (q >= 0) {
        queue.splice(q, 1);
        waiting.delete(jid);
      }
    }
  };
}

let observer: IntersectionObserver | null = null;
const seen = new WeakMap<Element, () => void>();
function observe(el: Element, onVisible: () => void): () => void {
  if (typeof IntersectionObserver === "undefined") {
    onVisible();
    return () => {};
  }
  if (!observer) {
    observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          const cb = seen.get(e.target);
          if (cb) {
            seen.delete(e.target);
            observer?.unobserve(e.target);
            cb();
          }
        }
      },
      { rootMargin: "120px" },
    );
  }
  seen.set(el, onVisible);
  observer.observe(el);
  return () => {
    seen.delete(el);
    observer?.unobserve(el);
  };
}

export function Avatar({
  jid,
  url,
  initials,
  size = 40,
}: {
  jid: string;
  /** the picture the list already knows ("" when not fetched yet) */
  url?: string;
  initials: string;
  size?: number;
}) {
  const ref = useRef<HTMLSpanElement>(null);
  const [src, setSrc] = useState(() => url || cache.get(jid) || "");
  const [broken, setBroken] = useState(false);

  useEffect(() => {
    setBroken(false);
    if (url) {
      cache.set(jid, url);
      setSrc(url);
      return;
    }
    const known = cache.get(jid);
    if (known !== undefined) {
      setSrc(known);
      return;
    }
    setSrc("");
    const el = ref.current;
    if (!el || !jid) return;
    let cancel = () => {};
    const stop = observe(el, () => {
      cancel = request(jid, setSrc);
    });
    return () => {
      stop();
      cancel();
    };
  }, [jid, url]);

  const style = { width: size, height: size, fontSize: Math.round(size * 0.34) };
  return (
    <span ref={ref} className={`avatar avatar--${tone(jid)} ib-avatar`} style={style} aria-hidden="true">
      {src && !broken ? <img src={src} alt="" loading="lazy" onError={() => setBroken(true)} /> : initials || "?"}
    </span>
  );
}
