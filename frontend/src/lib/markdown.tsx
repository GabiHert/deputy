import { Fragment, type ReactNode } from "react";

export type Inline =
  | { t: "text"; v: string }
  | { t: "b"; c: Inline[] }
  | { t: "i"; c: Inline[] }
  | { t: "code"; v: string }
  | { t: "link"; text: string; url: string };

export type MdBlock =
  | { t: "p"; c: Inline[] }
  | { t: "h"; level: number; c: Inline[] }
  | { t: "ul" | "ol"; items: Inline[][] }
  | { t: "pre"; v: string }
  | { t: "quote"; c: Inline[] };

/** parseInline handles **bold**, *italic* / _italic_, `code` and [text](url). */
export function parseInline(s: string): Inline[] {
  const out: Inline[] = [];
  const re = /(\*\*([^*]+)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|\*([^*\s][^*]*)\*|(^|[^\p{L}\p{N}])_([^_]+)_(?![\p{L}\p{N}]))/u;
  let rest = s;
  while (rest.length > 0) {
    const m = re.exec(rest);
    if (!m) {
      out.push({ t: "text", v: rest });
      break;
    }
    let start = m.index;
    if (m[8] !== undefined) start += m[7].length; // keep the char before _italic_
    if (start > 0) out.push({ t: "text", v: rest.slice(0, start) });
    if (m[2] !== undefined) out.push({ t: "b", c: parseInline(m[2]) });
    else if (m[3] !== undefined) out.push({ t: "code", v: m[3] });
    else if (m[4] !== undefined) out.push({ t: "link", text: m[4], url: m[5] });
    else if (m[6] !== undefined) out.push({ t: "i", c: parseInline(m[6]) });
    else if (m[8] !== undefined) out.push({ t: "i", c: parseInline(m[8]) });
    rest = rest.slice(m.index + m[0].length);
  }
  return out;
}

/** parseMarkdown splits text into paragraphs, headings, lists, quotes and code blocks. */
export function parseMarkdown(src: string): MdBlock[] {
  const lines = (src ?? "").replace(/\r\n/g, "\n").split("\n");
  const blocks: MdBlock[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ t: "p", c: parseInline(para.join(" ")) });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim().startsWith("```")) {
      flush();
      const code: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trim().startsWith("```")) code.push(lines[i++]);
      blocks.push({ t: "pre", v: code.join("\n") });
      continue;
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) {
      flush();
      blocks.push({ t: "h", level: h[1].length, c: parseInline(h[2]) });
      continue;
    }
    const li = /^\s*([-*•]|\d+[.)])\s+(.*)$/.exec(line);
    if (li) {
      flush();
      const kind = /\d/.test(li[1]) ? "ol" : "ul";
      const last = blocks[blocks.length - 1];
      if (last && last.t === kind) last.items.push(parseInline(li[2]));
      else blocks.push({ t: kind, items: [parseInline(li[2])] });
      continue;
    }
    const q = /^>\s?(.*)$/.exec(line);
    if (q) {
      flush();
      blocks.push({ t: "quote", c: parseInline(q[1]) });
      continue;
    }
    if (line.trim() === "") flush();
    else para.push(line.trim());
  }
  flush();
  return blocks;
}

function renderInline(c: Inline[]): ReactNode {
  return c.map((n, i) => {
    switch (n.t) {
      case "text":
        return <Fragment key={i}>{n.v}</Fragment>;
      case "b":
        return <strong key={i}>{renderInline(n.c)}</strong>;
      case "i":
        return <em key={i}>{renderInline(n.c)}</em>;
      case "code":
        return (
          <code key={i} className="md__code">
            {n.v}
          </code>
        );
      case "link":
        return (
          <span key={i} className="md__link" title={n.url}>
            {n.text}
          </span>
        );
    }
    return null;
  });
}

/** Markdown renders text as React elements; nothing is ever inserted as HTML. */
export function Markdown({ text, className }: { text: string; className?: string }) {
  const blocks = parseMarkdown(text);
  return (
    <div className={className ? `md ${className}` : "md"}>
      {blocks.map((b, i) => {
        switch (b.t) {
          case "p":
            return <p key={i}>{renderInline(b.c)}</p>;
          case "h":
            return (
              <p key={i} className={`md__h md__h${Math.min(b.level, 3)}`}>
                {renderInline(b.c)}
              </p>
            );
          case "ul":
            return (
              <ul key={i}>
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(it)}</li>
                ))}
              </ul>
            );
          case "ol":
            return (
              <ol key={i}>
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(it)}</li>
                ))}
              </ol>
            );
          case "pre":
            return (
              <pre key={i} className="md__pre">
                {b.v}
              </pre>
            );
          case "quote":
            return <blockquote key={i}>{renderInline(b.c)}</blockquote>;
        }
        return null;
      })}
    </div>
  );
}
