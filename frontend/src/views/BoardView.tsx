import { useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  api2,
  errText,
  on,
  useChatMessages,
  type Board,
  type BoardBlock,
  type BoardColumn,
  type BoardMessage,
  type Cell,
  type Group,
  type Run,
} from "../api";
import { FormFields, MODE_LABEL } from "../components/Assistant";
import { Icon, Pill, SrLabel } from "../components/ui";
import { buildSrcdoc, themeVars } from "../lib/frame";
import { Markdown } from "../lib/markdown";
import { clock, when } from "../format";

/** describeSource gives "runs.count(since: 7d, by: agent)". */
export function describeSource(b: BoardBlock): string {
  if (!b.source?.query) return "";
  const params = Object.entries(b.source.params ?? {})
    .filter(([, v]) => v !== "" && v !== null && v !== undefined)
    .map(([k, v]) => `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`)
    .join(", ");
  return `${b.source.query}(${params})`;
}

function useBoard(): [Board | null, number, string, () => void] {
  const [board, setBoard] = useState<Board | null>(null);
  const [version, setVersion] = useState(0);
  const [err, setErr] = useState("");
  const reload = () => {
    api2
      .getBoard()
      .then((b) => {
        setBoard(b);
        setVersion((v) => v + 1);
        setErr("");
      })
      .catch((e) => setErr(errText(e)));
  };
  useEffect(() => {
    reload();
    let t: ReturnType<typeof setTimeout> | undefined;
    const soon = () => {
      clearTimeout(t);
      t = setTimeout(reload, 600);
    };
    const offs = [on("board", reload), on("activity", soon), on("chats", soon)];
    return () => {
      clearTimeout(t);
      offs.forEach((off) => off());
    };
  }, []);
  return [board, version, err, reload];
}

export function BoardView({
  onOpenAgent,
  onOpenChats,
  onOpenLog,
}: {
  onOpenAgent: (id: string) => void;
  onOpenChats: () => void;
  onOpenLog: () => void;
}) {
  const [board, version, loadErr] = useBoard();
  const [text, setText] = useState("");
  const [err, setErr] = useState("");
  const [sending, setSending] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [boardMsgs] = useChatMessages(board?.chatId ?? "");

  // The board chat's latest answer, shown under the input when Claude says something instead of (or besides) changing the layout.
  const lastWord = useMemo(() => {
    for (let i = boardMsgs.length - 1; i >= 0; i--) {
      const m = boardMsgs[i];
      if (m.role === "user") return null;
      if (m.role === "assistant" || m.role === "error") return m;
    }
    return null;
  }, [boardMsgs]);

  const change = async () => {
    const t = text.trim();
    if (!t) return;
    setSending(true);
    setErr("");
    try {
      await api2.changeBoard(t);
      setText("");
    } catch (e) {
      setErr(errText(e));
    } finally {
      setSending(false);
    }
  };

  const blocks = board?.blocks ?? [];
  const label = board?.layoutAt
    ? `Live · numbers refresh on their own · layout by Claude, ${when(board.layoutAt)}`
    : "Default board · numbers refresh on their own";

  return (
    <div className="board">
      <div className="board__meta">
        <span className="muted small">{label}</span>
        <div className="row row--tight">
          <button
            type="button"
            className="btn btn--sm"
            onClick={() => inputRef.current?.focus()}
          >
            <Icon name="refresh" size={14} strokeWidth={2} />
            Change layout
          </button>
          {!!board?.layoutAt && (
            <button
              type="button"
              className="btn btn--quiet btn--sm"
              onClick={() => api2.resetBoard().catch((e) => setErr(errText(e)))}
            >
              Reset
            </button>
          )}
        </div>
      </div>
      {loadErr && (
        <div className="banner banner--bad" role="alert">
          {loadErr}
        </div>
      )}
      {board?.building && (
        <div className="board__building" role="status">
          <span className="spinner" aria-hidden="true" />
          Claude is changing the layout…
        </div>
      )}
      {board && blocks.length === 0 && !board.building && (
        <div className="card empty">
          <span className="muted">
            The board is empty. Tell Claude what you want to see below.
          </span>
        </div>
      )}
      <div className="board__grid">
        {blocks.map((b) => (
          <BlockView
            key={b.id}
            b={b}
            version={version}
            chatId={board?.chatId ?? ""}
            onOpenAgent={onOpenAgent}
            onOpenChats={onOpenChats}
            onOpenLog={onOpenLog}
          />
        ))}
      </div>
      <form
        className="board__ask"
        onSubmit={(e) => {
          e.preventDefault();
          change();
        }}
      >
        <SrLabel htmlFor="board-ask">
          Tell Claude how to change the layout
        </SrLabel>
        <input
          id="board-ask"
          ref={inputRef}
          className="board__input"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Tell Claude how to change the layout, like “add the open claims as a list”"
        />
        <button
          type="submit"
          className="btn btn--primary"
          disabled={sending || !text.trim() || board?.building}
        >
          Change
        </button>
      </form>
      {lastWord && !board?.building && (
        <div
          className={
            lastWord.role === "error" ? "board__word error-text" : "board__word"
          }
        >
          {lastWord.role === "error" ? (
            lastWord.text
          ) : (
            <Markdown text={lastWord.text} />
          )}
        </div>
      )}
      {err && (
        <span className="error-text" role="alert">
          {err}
        </span>
      )}
    </div>
  );
}

/* ---------- one block ---------- */

function BlockView({
  b,
  version,
  chatId,
  onOpenAgent,
  onOpenChats,
  onOpenLog,
}: {
  b: BoardBlock;
  version: number;
  chatId: string;
  onOpenAgent: (id: string) => void;
  onOpenChats: () => void;
  onOpenLog: () => void;
}) {
  const [showSource, setShowSource] = useState(false);
  const [err, setErr] = useState("");
  const src = describeSource(b);
  const items = Array.isArray(b.data?.items)
    ? (b.data!.items as unknown[])
    : [];
  const wide = b.span === 2 || b.type === "stats";
  const tone = b.type === "replies" && items.length > 0 ? " block--warn" : "";

  let body: React.ReactNode = null;
  switch (b.type) {
    case "stats":
      body = <StatsBlock items={items as Group[]} />;
      break;
    case "bars":
      body = <BarsBlock items={items as Group[]} />;
      break;
    case "replies":
      body = <RepliesBlock runs={items as Run[]} />;
      break;
    case "runs":
      body = <RunsBlock runs={items as Run[]} onOpenLog={onOpenLog} />;
      break;
    case "messages":
      body = (
        <MessagesBlock
          msgs={items as BoardMessage[]}
          onOpenChats={onOpenChats}
        />
      );
      break;
    case "table":
      body = <TableBlock items={items} columns={b.columns ?? []} />;
      break;
    case "heatmap":
      body = <Heatmap cells={items as Cell[]} />;
      break;
    case "prose":
      body = <Markdown text={b.text} className="prose-block" />;
      break;
    case "claude":
      body = b.text ? (
        <Markdown text={b.text} className="prose-block" />
      ) : (
        <span className="muted small">
          {b.refreshing ? "Claude is writing this…" : "Not written yet."}
        </span>
      );
      break;
    case "agent":
      body = b.agent ? (
        <div className="row row--center">
          <span className="agent-mark" aria-hidden="true">
            {(b.agent.name || "?").slice(0, 1).toUpperCase()}
          </span>
          <div className="grow">
            <strong className="strong">{b.agent.name}</strong>
            <div className="muted xsmall">
              {(b.agent.chats ?? []).length} chat
              {(b.agent.chats ?? []).length === 1 ? "" : "s"} ·{" "}
              {MODE_LABEL[b.agent.mode] ?? b.agent.mode} ·{" "}
              {b.agent.enabled ? "on" : "paused"}
            </div>
          </div>
          <button
            type="button"
            className="btn btn--sm"
            onClick={() => onOpenAgent(b.agent!.id)}
          >
            Open
          </button>
        </div>
      ) : (
        <span className="muted small">This agent no longer exists.</span>
      );
      break;
    case "form":
      body = b.form ? <FormBlock b={b} chatId={chatId} /> : null;
      break;
    case "custom":
      body = <CustomFrame b={b} version={version} showSource={showSource} />;
      break;
    default:
      body = (
        <span className="muted small">Unknown block type “{b.type}”.</span>
      );
  }

  return (
    <section
      className={`card block${wide ? " block--wide" : ""}${tone}`}
      aria-labelledby={`blk-${b.id}`}
    >
      <div className="block__head">
        <h2 id={`blk-${b.id}`} className="block__title">
          {b.title}
        </h2>
        {b.type === "claude" && (
          <div className="row row--tight">
            <span className="pill pill--claude">
              Claude · {scheduleLabel(b.schedule)}
            </span>
            <button
              type="button"
              className="icon-btn"
              aria-label={`Refresh ${b.title}`}
              disabled={b.refreshing}
              onClick={() => {
                setErr("");
                api2.refreshBoardBlock(b.id).catch((e) => setErr(errText(e)));
              }}
            >
              <Icon name="refresh" size={14} strokeWidth={2} />
            </button>
          </div>
        )}
        {b.type === "custom" && (
          <button
            type="button"
            className="chip-btn"
            aria-pressed={showSource}
            onClick={() => setShowSource(!showSource)}
          >
            Custom HTML · {showSource ? "hide source" : "show source"}
          </button>
        )}
      </div>
      {src && (
        <div className="block__src">
          <span className="dot dot--ok" aria-hidden="true" />
          {src}
        </div>
      )}
      {b.type === "claude" && (b.refreshing || b.textAt > 0) && (
        <div className="block__src">
          {b.refreshing
            ? "Claude is writing this now…"
            : `Written ${when(b.textAt)}`}
        </div>
      )}
      {b.error ? (
        <span className="error-text small" role="alert">
          {b.error}
        </span>
      ) : (
        body
      )}
      {err && <span className="error-text small">{err}</span>}
    </section>
  );
}

function scheduleLabel(s: string): string {
  if (!s || s === "manual") return "on Refresh";
  if (s === "hourly") return "hourly";
  const m = /^daily\s+(\d{1,2}:\d{2})$/.exec(s);
  return m ? `daily at ${m[1]}` : s;
}

function StatsBlock({ items }: { items: Group[] }) {
  if (items.length === 0)
    return <span className="muted small">Nothing yet.</span>;
  return (
    <div className="stat-tiles">
      {items.map((g) => (
        <div key={g.key} className="stat-tile">
          <div
            className={
              g.key === "waiting" && g.value > 0
                ? "stat-tile__num stat-tile__num--warn"
                : "stat-tile__num"
            }
          >
            {g.value.toLocaleString("en")}
          </div>
          <div className="muted xsmall">{g.label}</div>
        </div>
      ))}
    </div>
  );
}

function BarsBlock({ items }: { items: Group[] }) {
  if (items.length === 0)
    return <span className="muted small">Nothing yet.</span>;
  const max = Math.max(1, ...items.map((g) => g.value));
  return (
    <div className="bars">
      {items.map((g) => (
        <div key={g.key} className="bars__row">
          <span className="bars__label ellipsis">{g.label}</span>
          <span className="bars__track">
            <span
              className="bars__bar"
              style={{ width: `${(g.value / max) * 100}%` }}
            />
          </span>
          <span className="bars__val tabular">
            {g.value.toLocaleString("en")}
          </span>
        </div>
      ))}
    </div>
  );
}

function RepliesBlock({ runs }: { runs: Run[] }) {
  if (runs.length === 0)
    return <span className="muted small">Nothing is waiting for you.</span>;
  return (
    <div className="replies">
      {runs.map((r) => (
        <ReplyItem key={r.id} r={r} />
      ))}
    </div>
  );
}

function ReplyItem({ r }: { r: Run }) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(r.reply);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const first = (r.incoming ?? []).find((m) => m.text || m.transcript);
  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr("");
    try {
      await fn();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="reply-item">
      <div className="small fg2">
        <strong className="strong">{first?.senderName || r.chatName}</strong>
        {r.isGroup ? ` in ${r.chatName}` : ""}
        {first
          ? ` · “${(first.text || first.transcript).slice(0, 90)}${(first.text || first.transcript).length > 90 ? "…" : ""}”`
          : ""}
      </div>
      <div className="reply-item__draft">
        <div className="muted xsmall">{r.agentName} would reply</div>
        {editing ? (
          <>
            <SrLabel htmlFor={`bd-${r.id}`}>Reply text</SrLabel>
            <textarea
              id={`bd-${r.id}`}
              className="input input--prose"
              rows={3}
              value={text}
              onChange={(e) => setText(e.target.value)}
              autoFocus
            />
          </>
        ) : (
          <p className="prose">{r.reply}</p>
        )}
      </div>
      <div className="row">
        <button
          type="button"
          className="btn btn--primary btn--sm"
          disabled={busy || !text.trim()}
          onClick={() => act(() => api.sendDraft(r.id, text))}
        >
          Send
        </button>
        <button
          type="button"
          className="btn btn--sm"
          disabled={busy}
          onClick={() => setEditing(!editing)}
        >
          {editing ? "Done editing" : "Edit"}
        </button>
        <button
          type="button"
          className="btn btn--sm"
          disabled={busy}
          onClick={() => act(() => api.discardDraft(r.id))}
        >
          Discard
        </button>
      </div>
      {err && <span className="error-text small">{err}</span>}
    </div>
  );
}

const STATUS: Record<
  string,
  [string, "ok" | "warn" | "bad" | "acc" | "plain"]
> = {
  sent: ["Sent", "ok"],
  waiting: ["Waiting", "warn"],
  failed: ["Failed", "bad"],
  quiet: ["Quiet", "plain"],
  learned: ["Learned", "acc"],
  discarded: ["Discarded", "plain"],
  running: ["Thinking…", "acc"],
};

function RunsBlock({
  runs,
  onOpenLog,
}: {
  runs: Run[];
  onOpenLog: () => void;
}) {
  if (runs.length === 0) return <span className="muted small">No runs.</span>;
  return (
    <ul className="rowlist">
      {runs.map((r) => {
        const [label, tone] = STATUS[r.status] ?? [r.status, "plain"];
        return (
          <li key={r.id}>
            <button type="button" className="rowlist__btn" onClick={onOpenLog}>
              <span className="mono muted xsmall">{clock(r.at)}</span>
              <span className="grow ellipsis">
                {r.agentName} · {r.chatName}
              </span>
              <Pill tone={tone}>{label}</Pill>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function MessagesBlock({
  msgs,
  onOpenChats,
}: {
  msgs: BoardMessage[];
  onOpenChats: () => void;
}) {
  if (msgs.length === 0)
    return <span className="muted small">No messages.</span>;
  return (
    <ul className="rowlist">
      {msgs.map((m) => (
        <li key={`${m.chatJid}/${m.id}`}>
          <button
            type="button"
            className="rowlist__btn rowlist__btn--msg"
            onClick={onOpenChats}
          >
            <span className="small">
              <strong className="strong">
                {m.fromMe ? "You" : m.senderName || "Someone"}
              </strong>
              <span className="muted">
                {" "}
                · {when(m.at)}
                {m.chatName ? ` · ${m.chatName}` : ""}
              </span>
            </span>
            <span className="small fg2">
              {m.text || m.transcript || `[${m.kind}]`}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

/** cellValue reads a column from an item; keys may be dotted ("agent.name"). */
export function cellValue(item: unknown, key: string): string {
  let v: unknown = item;
  for (const part of key.split(".")) {
    if (v && typeof v === "object") v = (v as Record<string, unknown>)[part];
    else return "";
  }
  if (v === null || v === undefined) return "";
  if (typeof v === "number" && key.toLowerCase().endsWith("at") && v > 1e11)
    return when(v);
  if (Array.isArray(v)) return v.join(", ");
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

function TableBlock({
  items,
  columns,
}: {
  items: unknown[];
  columns: BoardColumn[];
}) {
  const cols: BoardColumn[] =
    columns.length > 0
      ? columns
      : Object.entries((items[0] as Record<string, unknown>) ?? {})
          .filter(([, v]) => v === null || typeof v !== "object")
          .slice(0, 5)
          .map(([k, v]) => ({
            key: k,
            label: k,
            numeric: typeof v === "number",
          }));
  if (items.length === 0) return <span className="muted small">No rows.</span>;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {cols.map((c) => (
              <th
                key={c.key}
                className={c.numeric ? "num" : undefined}
                scope="col"
              >
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.slice(0, 200).map((it, i) => (
            <tr key={i}>
              {cols.map((c) => (
                <td key={c.key} className={c.numeric ? "num" : undefined}>
                  {cellValue(it, c.key)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const SHADES = [
  "var(--heat0)",
  "var(--heat1)",
  "var(--heat2)",
  "var(--heat3)",
  "var(--heat4)",
];

function Heatmap({ cells }: { cells: Cell[] }) {
  const grid: number[][] = DAYS.map(() => new Array(24).fill(0));
  cells.forEach((c) => {
    if (c.day >= 0 && c.day < 7 && c.hour >= 0 && c.hour < 24)
      grid[c.day][c.hour] += c.count;
  });
  const max = Math.max(1, ...grid.flat());
  const total = grid.flat().reduce((a, b) => a + b, 0);
  let peak = { d: 0, h: 0, v: -1 };
  grid.forEach((row, d) =>
    row.forEach((v, h) => v > peak.v && (peak = { d, h, v })),
  );
  const label = total
    ? `Messages per hour; busiest ${DAYS[peak.d]} around ${peak.h}:00`
    : "No messages yet";
  return (
    <div className="heat" role="img" aria-label={label}>
      {grid.map((row, d) => (
        <div key={d} className="heat__row">
          <span className="heat__day">{DAYS[d]}</span>
          {row.map((v, h) => (
            <span
              key={h}
              className="heat__cell"
              style={{
                background:
                  SHADES[
                    v === 0 ? 0 : Math.min(4, 1 + Math.floor((v / max) * 3.999))
                  ],
              }}
              title={`${DAYS[d]} ${h}:00 · ${v}`}
            />
          ))}
        </div>
      ))}
      <div className="heat__row heat__axis" aria-hidden="true">
        <span className="heat__day" />
        {Array.from({ length: 24 }, (_, h) => (
          <span key={h} className="heat__tick">
            {h % 6 === 0 ? `${h}h` : ""}
          </span>
        ))}
      </div>
    </div>
  );
}

function FormBlock({ b, chatId }: { b: BoardBlock; chatId: string }) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [state, setState] = useState<"idle" | "sending" | "sent">("idle");
  const [err, setErr] = useState("");
  const form = b.form!;
  const submit = async () => {
    const lines = (form.questions ?? []).map(
      (q) =>
        `- ${q.label}: ${(answers[q.id] ?? "").replace(/\n/g, ", ") || "(no answer)"}`,
    );
    setState("sending");
    setErr("");
    try {
      await api2.send(
        chatId,
        `Answers to “${form.title || b.title}”:\n${lines.join("\n")}`,
      );
      setState("sent");
    } catch (e) {
      setErr(errText(e));
      setState("idle");
    }
  };
  return (
    <div className="ask__body ask__body--flat">
      <FormFields
        form={form}
        idPrefix={`bf-${b.id}`}
        answers={answers}
        setAnswer={(id, v) => setAnswers((a) => ({ ...a, [id]: v }))}
        disabled={state !== "idle"}
      />
      {state === "sent" ? (
        <span className="ok-text small">
          Sent to Claude. The board changes when it's done.
        </span>
      ) : (
        <div>
          <button
            type="button"
            className="btn btn--primary btn--sm"
            disabled={state === "sending" || !chatId}
            onClick={submit}
          >
            Send to Claude
          </button>
        </div>
      )}
      {err && <span className="error-text small">{err}</span>}
    </div>
  );
}

/* ---------- custom HTML ---------- */

function CustomFrame({
  b,
  version,
  showSource,
}: {
  b: BoardBlock;
  version: number;
  showSource: boolean;
}) {
  const ref = useRef<HTMLIFrameElement>(null);
  const height = Math.max(80, Math.min(1200, b.height || 240));
  // Build the document once per code change; data updates are posted in, so the frame keeps its state.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const srcDoc = useMemo(
    () =>
      buildSrcdoc({ html: b.html, js: b.js, data: b.data, theme: themeVars() }),
    [b.html, b.js],
  );

  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      const win = ref.current?.contentWindow;
      if (!win || e.source !== win) return;
      const m = e.data as {
        positron?: string;
        id?: number;
        name?: string;
        params?: Record<string, unknown>;
        action?: string;
        runId?: string;
      };
      if (
        m &&
        m.positron === "action" &&
        typeof m.id === "number" &&
        typeof m.runId === "string" &&
        m.runId
      ) {
        const dismiss =
          m.action === "dismissRun"
            ? true
            : m.action === "restoreRun"
              ? false
              : null;
        if (dismiss === null) return;
        api2
          .dismissRun(m.runId, dismiss)
          .then(() =>
            win.postMessage({ positron: "result", id: m.id, result: true }, "*"),
          )
          .catch((err) =>
            win.postMessage(
              { positron: "result", id: m.id, error: errText(err) },
              "*",
            ),
          );
        return;
      }
      if (
        !m ||
        m.positron !== "query" ||
        typeof m.id !== "number" ||
        typeof m.name !== "string"
      )
        return;
      api2
        .runBoardQuery(
          m.name,
          (m.params && typeof m.params === "object" ? m.params : {}) as Record<
            string,
            unknown
          >,
        )
        .then((result) =>
          win.postMessage({ positron: "result", id: m.id, result }, "*"),
        )
        .catch((err) =>
          win.postMessage(
            { positron: "result", id: m.id, error: errText(err) },
            "*",
          ),
        );
    };
    window.addEventListener("message", onMsg);
    return () => window.removeEventListener("message", onMsg);
  }, []);

  useEffect(() => {
    ref.current?.contentWindow?.postMessage(
      { positron: "update", data: b.data },
      "*",
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version]);

  return (
    <>
      <iframe
        ref={ref}
        className="custom-frame"
        title={b.title}
        sandbox="allow-scripts"
        srcDoc={srcDoc}
        style={{ height }}
      />
      {showSource && (
        <pre className="md__pre custom-source">
          {b.html}
          {b.js ? `\n\n<script>\n${b.js}\n</script>` : ""}
        </pre>
      )}
    </>
  );
}
