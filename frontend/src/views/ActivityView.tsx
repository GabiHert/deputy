import { useCallback, useEffect, useState } from "react";
import { api, errText, on, type Agent, type Message, type Run, type Stats, type Usage } from "../api";
import { bytes, clock, duration, tokens, usageLine, usd, when } from "../format";
import { Icon, Pill, SrLabel } from "../components/ui";

const emptyStats: Stats = { sent: 0, waiting: 0, quiet: 0, learned: 0, failed: 0, photos: 0, voice: 0, imagesSent: 0 };

function matches(r: Run, filter: string): boolean {
  if (filter === "") return true;
  if (filter === "needs") return r.status === "waiting" || r.status === "failed";
  return r.agentId === filter;
}

/** ActivityView is the Log: every run, newest first, with the Today panel. */
export function ActivityView({ agents, embedded }: { agents: Agent[]; embedded?: boolean }) {
  const [filter, setFilter] = useState("");
  const [runs, setRuns] = useState<Run[]>([]);
  const [stats, setStats] = useState<Stats>(emptyStats);
  const [loaded, setLoaded] = useState(false);

  const loadStats = useCallback(() => {
    api.todayStats().then(setStats).catch(() => {});
  }, []);

  useEffect(() => {
    let alive = true;
    setLoaded(false);
    api
      .listActivity(filter)
      .then((rs) => {
        if (!alive) return;
        setRuns(rs);
        setLoaded(true);
      })
      .catch(() => alive && setLoaded(true));
    loadStats();
    const off = on("activity", (r: Run) => {
      setRuns((cur) => {
        const rest = cur.filter((x) => x.id !== r.id);
        if (!matches(r, filter)) return rest;
        return [r, ...rest].sort((a, b) => b.at - a.at);
      });
      loadStats();
    });
    return () => {
      alive = false;
      off();
    };
  }, [filter, loadStats]);

  const tabs = [{ id: "", label: "Everything" }, { id: "needs", label: "Needs you" }, ...agents.map((a) => ({ id: a.id, label: a.name || "Untitled agent" }))];

  return (
    <div className={embedded ? "page page--activity page--embedded" : "page page--activity"}>
      <div className="feed-col">
        <header className="page__head page__head--split">
          {!embedded && <h1>Activity</h1>}
          <div role="tablist" aria-label="Filter activity" className="tabs">
            {tabs.map((t) => (
              <button
                key={t.id || "all"}
                type="button"
                role="tab"
                aria-selected={filter === t.id}
                className={filter === t.id ? "tabs__tab tabs__tab--on" : "tabs__tab"}
                onClick={() => setFilter(t.id)}
              >
                {t.label}
              </button>
            ))}
          </div>
        </header>
        {loaded && runs.length === 0 && (
          <div className="card empty">
            <span className="muted">
              {filter === "needs"
                ? "Nothing needs you right now."
                : "Nothing yet. When someone writes in a chat that has an agent, the agent's work shows up here."}
            </span>
          </div>
        )}
        <ol className="feed">
          {runs.map((r) => (
            <RunCard key={r.id} run={r} />
          ))}
        </ol>
      </div>
      <aside className="today" aria-labelledby="today">
        <h2 id="today" className="eyebrow">
          Today
        </h2>
        <div className="grid2">
          <Tile n={stats.sent} label="replies sent" />
          <Tile n={stats.waiting} label="waiting for you" tone={stats.waiting > 0 ? "warn" : undefined} />
          <Tile n={stats.quiet} label="stayed quiet" />
          <Tile n={stats.learned} label="things learned" />
        </div>
        <div className="card media-today">
          <div className="strong">Media today</div>
          <div className="kv">
            <span>Photos read</span>
            <span className="tabular">{stats.photos}</span>
          </div>
          <div className="kv">
            <span>Voice notes transcribed</span>
            <span className="tabular">{stats.voice}</span>
          </div>
          <div className="kv">
            <span>Images sent</span>
            <span className="tabular">{stats.imagesSent}</span>
          </div>
          {stats.failed > 0 && (
            <div className="kv kv--bad">
              <span>Runs that failed</span>
              <span className="tabular">{stats.failed}</span>
            </div>
          )}
        </div>
      </aside>
    </div>
  );
}

function Tile({ n, label, tone }: { n: number; label: string; tone?: "warn" }) {
  return (
    <div className="card tile">
      <div className={tone === "warn" ? "tile__num tile__num--warn" : "tile__num"}>{n}</div>
      <div className="muted xsmall">{label}</div>
    </div>
  );
}

function RunCard({ run }: { run: Run }) {
  const incoming = run.incoming ?? [];
  const where = run.chatName || "the chat";

  if (run.status === "quiet" && !run.commit && !run.request) {
    const first = incoming.find((m) => m.text)?.text;
    return (
      <li className="feed__item">
        <time className="feed__time">{clock(run.at)}</time>
        <div className="card card--condensed">
          <strong className="strong">{run.agentName}</strong> stayed quiet in {where}
          {first ? ` · "${first.length > 80 ? first.slice(0, 80) + "…" : first}"` : ""}
          {incoming.length > 1 ? ` (${incoming.length} messages${run.note ? `, ${run.note}` : ""})` : run.note ? ` (${run.note})` : ""}
          {run.usage && (
            <span className="muted xsmall run__usage" title={usageTitle(run.usage)}>
              {" · "}
              {tokens(run.usage.tokens)} tokens, ≈ {usd(run.usage.costUsd)}
            </span>
          )}
        </div>
      </li>
    );
  }

  return (
    <li className="feed__item">
      <time className="feed__time">{clock(run.at)}</time>
      <div className={run.status === "failed" ? "card run run--failed" : "card run"}>
        {run.request && (
          <div className="run__in">
            <p className="small no-margin">
              <span className="strong">You asked from Claude Code:</span> {run.request}
            </p>
          </div>
        )}
        {incoming.length > 0 && (
          <div className="run__in">
            {incoming.map((m) => (
              <IncomingMessage key={m.id} m={m} chatName={run.chatName} isGroup={run.isGroup} />
            ))}
          </div>
        )}
        <ReplySection run={run} />
        {run.usage && (
          <span className="muted xsmall run__usage" title={usageTitle(run.usage)}>
            {usageLine(run.usage)}
          </span>
        )}
      </div>
    </li>
  );
}

/** usageTitle is the full breakdown of a run's usage, for a hover. */
export function usageTitle(u: Usage): string {
  const lines = [
    u.model && `Model: ${u.model}`,
    `Input: ${u.input.toLocaleString()} new, ${u.cacheRead.toLocaleString()} read from cache, ${u.cacheWrite.toLocaleString()} written to cache`,
    `Output: ${u.output.toLocaleString()}`,
    `${u.turns} ${u.turns === 1 ? "turn" : "turns"}, ${(u.durationMs / 1000).toFixed(1)} s`,
    `≈ ${usd(u.costUsd)} at API prices; your subscription is not billed per run.`,
  ];
  return lines.filter(Boolean).join("\n");
}

function IncomingMessage({ m, chatName, isGroup }: { m: Message; chatName: string; isGroup: boolean }) {
  const who = m.fromMe ? "You" : m.senderName || "Someone";
  const inChat = isGroup ? ` in ${chatName}` : "";
  const verb: Record<string, string> = {
    image: "sent a photo",
    voice: "sent a voice note",
    audio: "sent an audio file",
    video: "sent a video",
    document: "sent a document",
    sticker: "sent a sticker",
  };
  return (
    <div className="msg">
      <div className="msg__who">
        <strong className="strong">{who}</strong> {verb[m.kind] ?? "wrote"}
        {inChat}
        {m.mentionsMe && <Pill tone="acc">Mentions you</Pill>}
      </div>
      <div className="msg__body">
        {m.kind === "image" && m.mediaUrl && <img className="msg__photo" src={m.mediaUrl} alt={m.text ? `Photo: ${m.text}` : "Photo"} />}
        {(m.kind === "voice" || m.kind === "audio") && (
          <div className="voice">
            {m.mediaUrl ? (
              <audio controls preload="none" src={m.mediaUrl} className="voice__player">
                <track kind="captions" />
              </audio>
            ) : (
              <span className="muted small row row--tight">
                <Icon name="mic" size={14} />
                Voice note
              </span>
            )}
            {m.seconds > 0 && <span className="mono muted xsmall">{duration(m.seconds)}</span>}
          </div>
        )}
        <div className="msg__text-col">
          {m.text && <p className="prose">{m.text}</p>}
          {(m.kind === "voice" || m.kind === "audio") &&
            (m.transcript ? (
              <p className="prose">
                <span className="eyebrow eyebrow--inline">Transcript</span>
                {m.transcript}
              </p>
            ) : (
              <span className="muted small">Not transcribed. Turn on transcription for this agent and install the language in Settings.</span>
            ))}
        </div>
      </div>
    </div>
  );
}

function statusPill(status: string) {
  switch (status) {
    case "sent":
      return <Pill tone="ok">Sent</Pill>;
    case "waiting":
      return <Pill tone="warn">Waiting for you</Pill>;
    case "discarded":
      return <Pill>Discarded</Pill>;
    case "learned":
      return <Pill tone="acc">Only learned</Pill>;
    case "quiet":
      return <Pill>No reply</Pill>;
    case "running":
      return <Pill tone="acc">Thinking…</Pill>;
    case "failed":
      return <Pill tone="bad">Failed</Pill>;
  }
  return null;
}

function ReplySection({ run }: { run: Run }) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(run.reply);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!editing) setText(run.reply);
  }, [run.reply, editing]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr("");
    try {
      await fn();
      setEditing(false);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  const where = run.chatName || "the chat";
  const pending = run.status === "waiting";
  const images = run.images ?? [];
  const files = run.files ?? [];

  if (run.status === "running") {
    return (
      <div className="run__reply">
        <div className="run__head">
          <strong className="strong">{run.agentName}</strong> is reading {where} {statusPill("running")}
        </div>
      </div>
    );
  }

  if (run.status === "failed") {
    return (
      <div className="run__reply run__reply--failed">
        <div className="run__failed">
          <div>
            <strong className="bad-text">{run.agentName} couldn't answer</strong> in {where}
            <div className="fg2 small">{run.error || "Claude Code stopped with an error."} Nothing was sent.</div>
          </div>
          <button type="button" className="btn btn--sm" disabled={busy} onClick={() => act(() => api.retryRun(run.id))}>
            Try again
          </button>
        </div>
        {err && <span className="error-text">{err}</span>}
      </div>
    );
  }

  const verb =
    run.status === "sent"
      ? `replied${run.isGroup ? ` in ${where}` : ""}`
      : run.status === "waiting"
        ? `wrote a reply${run.isGroup ? ` for ${where}` : ""}`
        : run.status === "learned"
          ? `read ${where}`
          : run.status === "quiet"
            ? `stayed quiet in ${where}`
            : `wrote a reply${run.isGroup ? ` for ${where}` : ""}`;

  return (
    <div className={pending ? "run__reply run__reply--draft" : "run__reply"}>
      <div className="run__head">
        <strong className="strong">{run.agentName}</strong> {verb} {statusPill(run.status)}
        {run.voice && run.reply && <span className="muted xsmall">as a voice note</span>}
      </div>
      {run.reply &&
        (editing && pending ? (
          <>
            <SrLabel htmlFor={`draft-${run.id}`}>Reply text</SrLabel>
            <textarea id={`draft-${run.id}`} className="input input--prose" rows={4} value={text} onChange={(e) => setText(e.target.value)} autoFocus />
          </>
        ) : (
          <p className={run.status === "discarded" ? "prose prose--struck" : "prose"}>{run.reply}</p>
        ))}
      {run.note && run.status !== "quiet" && <span className="muted small">{run.note}</span>}
      {images.length > 0 && (
        <div className="attachments">
          {images.map((a) => (
            <div key={a.path} className="attachment">
              {a.url ? <img src={a.url} alt="" className="attachment__img" /> : <span className="attachment__img attachment__img--none"><Icon name="image" /></span>}
              <div className="attachment__text">
                <span className="mono small ellipsis">{a.path}</span>
                <span className="muted xsmall">
                  {run.status === "sent" ? "Image sent with the reply" : "Image to send with the reply"}
                  {a.bytes ? ` · ${bytes(a.bytes)}` : ""}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
      {pending && (
        <div className="row">
          <button type="button" className="btn btn--primary" disabled={busy || !text.trim()} onClick={() => act(() => api.sendDraft(run.id, text))}>
            {busy ? "Sending…" : "Send"}
          </button>
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => {
              if (editing) setText(text);
              setEditing(!editing);
            }}
          >
            {editing ? "Done editing" : "Edit"}
          </button>
          <button type="button" className="btn" disabled={busy} onClick={() => act(() => api.discardDraft(run.id))}>
            Discard
          </button>
        </div>
      )}
      {err && <span className="error-text">{err}</span>}
      {run.commit && (
        <div className="commit">
          <span className="ok-text">
            <Icon name="commit" size={15} strokeWidth={2} />
          </span>
          <span>
            Committed <span className="mono">{run.commit.slice(0, 7)}</span>
            {files.length > 0 && (
              <>
                {" · "}
                {files.map((f, i) => (
                  <span key={f}>
                    {i > 0 && ", "}
                    <span className="mono">{f}</span>
                  </span>
                ))}
              </>
            )}
          </span>
        </div>
      )}
      {run.sentAt > 0 && run.status === "sent" && run.sentAt - run.at > 60000 && <span className="muted xsmall">Sent {when(run.sentAt)}</span>}
    </div>
  );
}
