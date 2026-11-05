import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import {
  api,
  api2,
  errText,
  useChatMessages,
  type AgentProposal,
  type AssistantChat,
  type AssistantMessage,
  type ChangeProposal,
  type Form,
} from "../api";
import { compactDiff, lineDiff } from "../lib/diff";
import { filterChats, groupChats } from "../lib/chatgroups";
import { Markdown } from "../lib/markdown";
import { when } from "../format";
import { Confirm, Icon, Pill, SrLabel } from "./ui";

export const MODE_LABEL: Record<string, string> = {
  approve: "Wait for my OK",
  auto: "Sent on its own",
  never: "Never reply, just learn",
};
export const TRIGGER_LABEL: Record<string, string> = {
  all: "When the instructions say so",
  mention: "Only when mentioned or replied to",
};

/* ---------- rename ---------- */

/** RenameInput edits a title in place: Enter or blur saves, Escape cancels. */
export function RenameInput({ id, value, onDone, className }: { id: string; value: string; onDone: (v: string | null) => void; className?: string }) {
  const [v, setV] = useState(value);
  const done = useRef(false);
  const finish = (out: string | null) => {
    if (done.current) return;
    done.current = true;
    onDone(out);
  };
  return (
    <>
      <SrLabel htmlFor={id}>Chat name</SrLabel>
      <input
        id={id}
        className={className ?? "input rename"}
        value={v}
        autoFocus
        onFocus={(e) => e.target.select()}
        onChange={(e) => setV(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            finish(v.trim() || null);
          } else if (e.key === "Escape") {
            e.preventDefault();
            finish(null);
          }
        }}
        onBlur={() => finish(v.trim() || null)}
      />
    </>
  );
}

/* ---------- conversation ---------- */

export function Conversation({
  chat,
  onOpenAgent,
  onAgentCreated,
  intro,
  suggestions,
  placeholder,
  wide,
  ensureChat,
}: {
  chat: AssistantChat | null;
  ensureChat?: () => Promise<string>;
  onOpenAgent?: (id: string) => void;
  onAgentCreated?: (id: string) => void;
  intro?: ReactNode;
  suggestions?: { label: string; onPick?: () => void }[];
  placeholder?: string;
  wide?: boolean;
}) {
  const chatId = chat?.id ?? "";
  const [msgs] = useChatMessages(chatId);
  const [text, setText] = useState("");
  const [err, setErr] = useState("");
  const listRef = useRef<HTMLDivElement>(null);
  const busy = !!chat?.busy;

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [msgs.length, busy]);

  const send = async (t: string) => {
    const body = t.trim();
    if (!body || (!chatId && !ensureChat)) return;
    setErr("");
    try {
      const id = chatId || (await ensureChat!());
      await api2.send(id, body);
      setText("");
    } catch (e) {
      setErr(errText(e));
    }
  };

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      if (!busy) send(text);
    }
  };

  const empty = msgs.length === 0;
  return (
    <div className={wide ? "convo convo--wide" : "convo"}>
      <div className="convo__list" ref={listRef} aria-live="polite">
        {intro}
        {msgs.map((m) => (
          <MessageItem key={m.id} m={m} onOpenAgent={onOpenAgent} onAgentCreated={onAgentCreated} />
        ))}
        {busy && (
          <div className="convo__busy">
            <span className="spinner" aria-hidden="true" />
            Claude is working…
            <button type="button" className="btn btn--sm" onClick={() => api2.stop(chatId)}>
              Stop
            </button>
          </div>
        )}
      </div>
      <div className="convo__foot">
        {empty && suggestions && suggestions.length > 0 && (
          <div className="chips">
            {suggestions.map((s) => (
              <button
                key={s.label}
                type="button"
                className="suggest"
                onClick={() => (s.onPick ? s.onPick() : send(s.label))}
                disabled={!chatId && !ensureChat}
              >
                {s.label}
              </button>
            ))}
          </div>
        )}
        {err && (
          <span className="error-text" role="alert">
            {err}
          </span>
        )}
        <div className="composer">
          <SrLabel htmlFor={`composer-${chatId}`}>Message Claude</SrLabel>
          <textarea
            id={`composer-${chatId}`}
            className="composer__input"
            rows={2}
            value={text}
            placeholder={placeholder ?? "Ask about your chats or agents"}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKey}
            disabled={!chatId && !ensureChat}
          />
          <button type="button" className="composer__send" aria-label="Send to Claude" disabled={busy || !text.trim() || (!chatId && !ensureChat)} onClick={() => send(text)}>
            <Icon name="send" size={16} strokeWidth={2} />
          </button>
        </div>
      </div>
    </div>
  );
}

function MessageItem({ m, onOpenAgent, onAgentCreated }: { m: AssistantMessage; onOpenAgent?: (id: string) => void; onAgentCreated?: (id: string) => void }) {
  switch (m.role) {
    case "user":
      return <div className="bubble">{m.text}</div>;
    case "assistant":
      return <Markdown text={m.text} className="say" />;
    case "progress":
      return (
        <div className="progress-line">
          <Icon name="check" size={14} strokeWidth={2} />
          {m.text}
        </div>
      );
    case "note":
      return (
        <div className="note-line">
          <Icon name="check" size={14} strokeWidth={2} />
          {m.text}
        </div>
      );
    case "error":
      return (
        <div className="error-line" role="alert">
          {m.text}
        </div>
      );
    case "card":
      if (!m.card) return null;
      if (m.card.kind === "ask" && m.card.ask) return <AskCard m={m} form={m.card.ask} />;
      if (m.card.kind === "agent" && m.card.agent) return <AgentCard m={m} p={m.card.agent} onOpenAgent={onOpenAgent} onCreated={onAgentCreated} />;
      if (m.card.kind === "change" && m.card.change) return <ChangeCard m={m} c={m.card.change} onOpenAgent={onOpenAgent} />;
      return null;
  }
  return m.text ? <div className="say">{m.text}</div> : null;
}

/* ---------- ask / form ---------- */

/** FormFields renders a form's questions; shared by ask cards and board form blocks. */
export function FormFields({
  form,
  idPrefix,
  answers,
  setAnswer,
  disabled,
}: {
  form: Form;
  idPrefix: string;
  answers: Record<string, string>;
  setAnswer: (id: string, v: string) => void;
  disabled?: boolean;
}) {
  return (
    <>
      {(form.questions ?? []).map((q) => {
        const qid = `${idPrefix}-${q.id}`;
        const val = answers[q.id] ?? "";
        const opts = q.options ?? [];
        if (q.kind === "input") {
          return (
            <div key={q.id} className="field">
              <label htmlFor={qid} className="field__label">
                {q.label}
              </label>
              <input id={qid} className="input" value={val} placeholder={q.placeholder} disabled={disabled} onChange={(e) => setAnswer(q.id, e.target.value)} />
            </div>
          );
        }
        if (q.kind === "multi") {
          const picked = val ? val.split("\n") : [];
          return (
            <fieldset key={q.id} className="ask__q">
              <legend className="ask__legend">{q.label}</legend>
              {opts.map((o, i) => (
                <label key={o} className="check" htmlFor={`${qid}-${i}`}>
                  <input
                    id={`${qid}-${i}`}
                    type="checkbox"
                    disabled={disabled}
                    checked={picked.includes(o)}
                    onChange={(e) => setAnswer(q.id, (e.target.checked ? [...picked, o] : picked.filter((x) => x !== o)).join("\n"))}
                  />
                  <span className="check__title">{o}</span>
                </label>
              ))}
            </fieldset>
          );
        }
        const choices = q.kind === "confirm" ? (opts.length ? opts : ["Yes", "No"]) : opts;
        return (
          <fieldset key={q.id} className="ask__q">
            <legend className="ask__legend">{q.label}</legend>
            <div className={q.kind === "confirm" ? "row" : "ask__opts"}>
              {choices.map((o) => (
                <button
                  key={o}
                  type="button"
                  aria-pressed={val === o}
                  disabled={disabled}
                  className={val === o ? "opt opt--on" : "opt"}
                  onClick={() => setAnswer(q.id, o)}
                >
                  {o}
                </button>
              ))}
            </div>
          </fieldset>
        );
      })}
    </>
  );
}

function AskCard({ m, form }: { m: AssistantMessage; form: Form }) {
  const card = m.card!;
  const open = card.status === "open";
  const [answers, setAnswers] = useState<Record<string, string>>(card.answers ?? {});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
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
    <form className="ask" aria-label={form.title || "Claude is asking"} onSubmit={(e) => e.preventDefault()}>
      <div className="ask__head">{form.title || "Claude is asking"}</div>
      <div className="ask__body">
        <FormFields form={form} idPrefix={m.id} answers={answers} setAnswer={(id, v) => setAnswers((a) => ({ ...a, [id]: v }))} disabled={!open || busy} />
        {open ? (
          <div className="row">
            <button type="button" className="btn btn--primary" disabled={busy} onClick={() => act(() => api2.answerCard(m.id, answers))}>
              Answer
            </button>
            <button type="button" className="btn" disabled={busy} onClick={() => act(() => api2.answerCard(m.id, {}))}>
              Skip, you choose
            </button>
          </div>
        ) : (
          <span className="muted small">{card.status === "dismissed" ? "Skipped." : card.result || "Answered."}</span>
        )}
        {err && <span className="error-text">{err}</span>}
      </div>
    </form>
  );
}

/* ---------- agent proposal ---------- */

function AgentCard({
  m,
  p: proposal,
  onOpenAgent,
  onCreated,
}: {
  m: AssistantMessage;
  p: AgentProposal;
  onOpenAgent?: (id: string) => void;
  onCreated?: (id: string) => void;
}) {
  const card = m.card!;
  const open = card.status === "open";
  const [p, setP] = useState<AgentProposal>(proposal);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!editing) setP(proposal);
  }, [proposal, editing]);
  const set = <K extends keyof AgentProposal>(k: K, v: AgentProposal[K]) => setP((x) => ({ ...x, [k]: v }));
  const chats = p.chats ?? [];
  const files = p.knowledge ?? [];
  const media = [p.readImages && "Reads photos", p.transcribeVoice && "transcribes voice notes", p.voiceReplies ? "answers with voice notes" : "answers in text", p.sendImages && "sends images"]
    .filter(Boolean)
    .join(" · ");

  const create = async () => {
    setBusy(true);
    setErr("");
    try {
      const a = await api2.createAgentFromCard(m.id, p);
      setEditing(false);
      onCreated?.(a.id);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  const choose = async () => {
    try {
      const dir = await api.chooseFolder();
      if (dir) set("repoPath", dir);
    } catch (e) {
      setErr(errText(e));
    }
  };

  const created = card.status === "done";
  return (
    <section className="proposal" aria-label={`Proposed agent ${p.name}`}>
      <div className="proposal__head">
        <div className="row row--center">
          <span className="agent-mark" aria-hidden="true">
            {(p.name || "?").slice(0, 1).toUpperCase()}
          </span>
          <div>
            {editing ? (
              <>
                <SrLabel htmlFor={`${m.id}-name`}>Agent name</SrLabel>
                <input id={`${m.id}-name`} className="input" value={p.name} onChange={(e) => set("name", e.target.value)} />
              </>
            ) : (
              <h2 className="proposal__name">{p.name || "Untitled agent"}</h2>
            )}
            <div className="muted xsmall">New agent · proposed by Claude</div>
          </div>
        </div>
        {created ? <Pill tone="ok">Created</Pill> : card.status === "dismissed" ? <Pill>Dismissed</Pill> : <Pill tone="warn">Not created yet</Pill>}
      </div>

      <dl className="proposal__rows">
        <dt>Chat{chats.length === 1 ? "" : "s"}</dt>
        <dd>
          {chats.length === 0
            ? "None yet"
            : chats.map((c) => `${c.name}${c.isGroup ? ` (group${c.members ? `, ${c.members} people` : ""})` : ""}`).join(", ")}
        </dd>
        <dt>Answers</dt>
        <dd>
          {editing ? (
            <select aria-label="When it answers" className="select" value={p.trigger} onChange={(e) => set("trigger", e.target.value)}>
              <option value="all">{TRIGGER_LABEL.all}</option>
              <option value="mention">{TRIGGER_LABEL.mention}</option>
            </select>
          ) : (
            TRIGGER_LABEL[p.trigger] ?? p.trigger
          )}
        </dd>
        <dt>Replies</dt>
        <dd>
          {editing ? (
            <select aria-label="Replies" className="select" value={p.mode} onChange={(e) => set("mode", e.target.value)}>
              <option value="approve">{MODE_LABEL.approve}</option>
              <option value="auto">{MODE_LABEL.auto}</option>
              <option value="never">{MODE_LABEL.never}</option>
            </select>
          ) : (
            <>
              {MODE_LABEL[p.mode] ?? p.mode}
              {p.maxPerHour ? ` · up to ${p.maxPerHour} an hour` : ""}
            </>
          )}
        </dd>
        <dt>Media</dt>
        <dd>
          {editing ? (
            <div className="proposal__checks">
              <label className="check" htmlFor={`${m.id}-ri`}>
                <input id={`${m.id}-ri`} type="checkbox" checked={p.readImages} onChange={(e) => set("readImages", e.target.checked)} />
                <span className="check__title">Read photos</span>
              </label>
              <label className="check" htmlFor={`${m.id}-tv`}>
                <input id={`${m.id}-tv`} type="checkbox" checked={p.transcribeVoice} onChange={(e) => set("transcribeVoice", e.target.checked)} />
                <span className="check__title">Transcribe voice notes</span>
              </label>
              <label className="check" htmlFor={`${m.id}-vr`}>
                <input id={`${m.id}-vr`} type="checkbox" checked={p.voiceReplies} onChange={(e) => set("voiceReplies", e.target.checked)} />
                <span className="check__title">Answer voice notes with voice</span>
              </label>
              <label className="check" htmlFor={`${m.id}-si`}>
                <input id={`${m.id}-si`} type="checkbox" checked={p.sendImages} onChange={(e) => set("sendImages", e.target.checked)} />
                <span className="check__title">Send images</span>
              </label>
            </div>
          ) : (
            media
          )}
        </dd>
        <dt>Repo</dt>
        <dd className="mono small">
          {editing ? (
            <div className="row row--nowrap">
              <SrLabel htmlFor={`${m.id}-repo`}>Repo folder</SrLabel>
              <input id={`${m.id}-repo`} className="input input--mono grow" value={p.repoPath} onChange={(e) => set("repoPath", e.target.value)} />
              <button type="button" className="btn btn--sm" onClick={choose}>
                Choose…
              </button>
            </div>
          ) : (
            p.repoPath || "Chosen by Deputy"
          )}
        </dd>
      </dl>

      <div className="proposal__section">
        <h3 className="proposal__h">
          How it behaves <span className="mono muted xsmall">INSTRUCTIONS.md</span>
        </h3>
        {editing ? (
          <>
            <SrLabel htmlFor={`${m.id}-ins`}>Instructions</SrLabel>
            <textarea id={`${m.id}-ins`} className="input input--prose" rows={10} value={p.instructions} onChange={(e) => set("instructions", e.target.value)} />
          </>
        ) : (
          <Markdown text={p.instructions} className="instructions" />
        )}
      </div>

      {files.length > 0 && (
        <div className="proposal__section">
          <h3 className="proposal__h">It starts out knowing</h3>
          <ul className="filelist">
            {files.map((f) => (
              <li key={f.path}>
                <span className="mono">{f.path}</span>
                <span className="fg2">{f.summary}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {open && (
        <div className="proposal__foot">
          <button type="button" className="btn btn--primary" disabled={busy || !p.name.trim()} onClick={create}>
            {busy ? "Creating…" : "Create agent"}
          </button>
          <button type="button" className="btn" disabled={busy} onClick={() => setEditing(!editing)}>
            {editing ? "Done editing" : "Edit before creating"}
          </button>
          <button type="button" className="btn btn--quiet" disabled={busy} onClick={() => api2.dismissCard(m.id).catch((e) => setErr(errText(e)))}>
            Dismiss
          </button>
          {!editing && <span className="muted small">Or reply below to change it.</span>}
        </div>
      )}
      {created && (
        <div className="proposal__foot">
          <span className="ok-text small">{card.result || "Created."}</span>
          {p.createdAgentId && onOpenAgent && (
            <button type="button" className="btn btn--sm" onClick={() => onOpenAgent(p.createdAgentId)}>
              Open {p.name}
            </button>
          )}
        </div>
      )}
      {err && (
        <div className="proposal__foot">
          <span className="error-text" role="alert">
            {err}
          </span>
        </div>
      )}
    </section>
  );
}

/* ---------- change proposal ---------- */

function ChangeCard({ m, c, onOpenAgent }: { m: AssistantMessage; c: ChangeProposal; onOpenAgent?: (id: string) => void }) {
  const card = m.card!;
  const open = card.status === "open";
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const diff = useMemo(() => (c.instructions ? compactDiff(lineDiff(c.oldInstructions ?? "", c.instructions)) : []), [c.instructions, c.oldInstructions]);
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
    <section className="change" aria-label={`Proposed change to ${c.agentName}`}>
      <div className="change__head">Proposed change · {c.agentName}</div>
      <div className="change__body">
        {c.summary && <p className="change__summary">{c.summary}</p>}
        {diff.length > 0 && (
          <div className="diff" aria-label="Changes to INSTRUCTIONS.md">
            {diff.map((l, i) =>
              l === null ? (
                <div key={i} className="diff__gap">
                  …
                </div>
              ) : (
                <div key={i} className={l.op === "+" ? "diff__line diff__line--add" : l.op === "-" ? "diff__line diff__line--del" : "diff__line"}>
                  <span className="diff__op" aria-label={l.op === "+" ? "added" : l.op === "-" ? "removed" : undefined}>
                    {l.op}
                  </span>
                  {l.text || " "}
                </div>
              ),
            )}
          </div>
        )}
        {(c.settings ?? []).length > 0 && (
          <ul className="filelist">
            {(c.settings ?? []).map((s) => (
              <li key={s.field}>
                <span>{s.label}</span>
                <span className="fg2">
                  {s.from} → <strong>{s.to}</strong>
                </span>
              </li>
            ))}
          </ul>
        )}
        {(c.knowledge ?? []).length > 0 && (
          <ul className="filelist">
            {(c.knowledge ?? []).map((f) => (
              <li key={f.path}>
                <span className="mono">{f.path}</span>
                <span className="fg2">{f.summary}</span>
              </li>
            ))}
          </ul>
        )}
        {open ? (
          <div className="row">
            <button type="button" className="btn btn--primary btn--sm" disabled={busy} onClick={() => act(() => api2.applyChangeCard(m.id))}>
              Apply
            </button>
            <button type="button" className="btn btn--sm" disabled={busy} onClick={() => act(() => api2.dismissCard(m.id))}>
              Not now
            </button>
          </div>
        ) : card.status === "done" ? (
          <div className="row">
            <span className="ok-text small">
              {card.result || "Applied."}
              {c.commit ? (
                <>
                  {" "}
                  · <span className="mono">{c.commit.slice(0, 7)}</span>
                </>
              ) : null}
            </span>
            {onOpenAgent && c.agentId && (
              <button type="button" className="btn btn--quiet btn--sm" onClick={() => onOpenAgent(c.agentId)}>
                Open {c.agentName}
              </button>
            )}
          </div>
        ) : (
          <span className="muted small">Skipped. Nothing changed.</span>
        )}
        {err && <span className="error-text">{err}</span>}
      </div>
    </section>
  );
}

/* ---------- saved chats list ---------- */

export function ChatList({
  chats,
  activeId,
  onPick,
  onNew,
  renamingId,
  setRenamingId,
  onRename,
  onDelete,
}: {
  chats: AssistantChat[];
  activeId: string;
  onPick: (id: string) => void;
  onNew: () => void;
  renamingId: string;
  setRenamingId: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onDelete: (id: string) => void;
}) {
  const [q, setQ] = useState("");
  const [confirmId, setConfirmId] = useState("");
  const groups = groupChats(filterChats(chats, q));
  return (
    <nav className="chatlist" aria-label="Saved chats">
      <div className="chatlist__top">
        <button type="button" className="btn btn--primary chatlist__new" onClick={onNew}>
          <Icon name="plus" size={14} strokeWidth={2} />
          New chat
        </button>
        <SrLabel htmlFor="chat-search">Search chats</SrLabel>
        <input id="chat-search" type="search" className="input input--sm" placeholder="Search chats" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div className="chatlist__items">
        {groups.length === 0 && <span className="muted xsmall chatlist__empty">{q ? "No chats match." : "No chats yet."}</span>}
        {groups.map((g) => (
          <div key={g.label} className="chatlist__group">
            <div className="chatlist__label">{g.label}</div>
            {g.chats.map((c) => {
              const on = c.id === activeId;
              if (renamingId === c.id) {
                return (
                  <div key={c.id} className="chatlist__rename">
                    <RenameInput
                      id={`rename-${c.id}`}
                      value={c.title}
                      className="input input--sm"
                      onDone={(v) => {
                        setRenamingId("");
                        if (v && v !== c.title) onRename(c.id, v);
                      }}
                    />
                  </div>
                );
              }
              return (
                <div key={c.id}>
                  <div className={on ? "chatlist__row chatlist__row--on" : "chatlist__row"}>
                    <button
                      type="button"
                      className="chatlist__pick"
                      aria-current={on ? "true" : undefined}
                      title="Double-click to rename"
                      onClick={() => onPick(c.id)}
                      onDoubleClick={() => setRenamingId(c.id)}
                    >
                      <span className="chatlist__title">
                        {c.busy && <span className="dot dot--acc" aria-label="Working" />}
                        {c.title || "New chat"}
                      </span>
                      <span className="chatlist__sub">{c.preview || when(c.updatedAt)}</span>
                    </button>
                    {on && (
                      <>
                        <button type="button" className="icon-btn" aria-label={`Rename ${c.title}`} onClick={() => setRenamingId(c.id)}>
                          <Icon name="pencil" size={13} />
                        </button>
                        <button type="button" className="icon-btn" aria-label={`Delete ${c.title}`} onClick={() => setConfirmId(c.id)}>
                          <Icon name="trash" size={13} />
                        </button>
                      </>
                    )}
                  </div>
                  {confirmId === c.id && (
                    <Confirm
                      danger
                      text="Delete this chat?"
                      yes="Delete"
                      onNo={() => setConfirmId("")}
                      onYes={() => {
                        setConfirmId("");
                        onDelete(c.id);
                      }}
                    />
                  )}
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </nav>
  );
}

/* ---------- the panel in Activity ---------- */

export function ClaudePanel({
  chats,
  reloadChats,
  onOpenAgent,
  onNewAgent,
}: {
  chats: AssistantChat[];
  reloadChats: () => void;
  onOpenAgent: (id: string) => void;
  onNewAgent: () => void;
}) {
  const visible = chats.filter((c) => c.purpose !== "board");
  const [activeId, setActiveId] = useState("");
  const [renamingId, setRenamingId] = useState("");
  const [headRename, setHeadRename] = useState(false);
  const [listOpen, setListOpen] = useState(true);
  const [err, setErr] = useState("");
  const pendingId = useRef(""); // a chat just created, not in the list yet

  const select = (id: string) => {
    pendingId.current = id;
    setActiveId(id);
  };

  useEffect(() => {
    if (visible.some((c) => c.id === pendingId.current)) pendingId.current = "";
    if (activeId && activeId === pendingId.current) return;
    if (!visible.some((c) => c.id === activeId)) {
      const newest = [...visible].sort((a, b) => b.updatedAt - a.updatedAt)[0];
      setActiveId(newest?.id ?? "");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible.map((c) => c.id).join(",")]);

  const active = visible.find((c) => c.id === activeId) ?? null;

  const newChat = async () => {
    setErr("");
    try {
      const c = await api2.newChat("general", "");
      select(c.id);
      reloadChats();
    } catch (e) {
      setErr(errText(e));
    }
  };
  const rename = (id: string, title: string) =>
    api2
      .rename(id, title)
      .then(reloadChats)
      .catch((e) => setErr(errText(e)));
  const remove = (id: string) =>
    api2
      .remove(id)
      .then(() => {
        if (id === activeId) setActiveId("");
        reloadChats();
      })
      .catch((e) => setErr(errText(e)));

  return (
    <aside className="claude-panel" aria-label="Claude chats">
      <div className="claude-panel__main">
        <div className="claude-panel__head">
          <div className="claude-panel__title">
            <span className="eyebrow">Claude</span>
            {headRename && active ? (
              <RenameInput
                id="head-rename"
                value={active.title}
                className="input input--sm"
                onDone={(v) => {
                  setHeadRename(false);
                  if (v && v !== active.title) rename(active.id, v);
                }}
              />
            ) : (
              <button type="button" className="claude-panel__name" title="Double-click to rename" onDoubleClick={() => active && setHeadRename(true)}>
                {active?.title || "New chat"}
              </button>
            )}
          </div>
          <div className="row row--tight">
            <button type="button" className="icon-btn icon-btn--box" aria-label="Rename chat" disabled={!active} onClick={() => setHeadRename(true)}>
              <Icon name="pencil" size={15} />
            </button>
            <button
              type="button"
              className={listOpen ? "icon-btn icon-btn--box icon-btn--on" : "icon-btn icon-btn--box"}
              aria-label={listOpen ? "Hide chat list" : "Show chat list"}
              aria-pressed={listOpen}
              onClick={() => setListOpen(!listOpen)}
            >
              <Icon name="sidebar" size={16} />
            </button>
          </div>
        </div>
        {err && (
          <div className="banner banner--bad banner--tight" role="alert">
            {err}
          </div>
        )}
        <Conversation
          key={active?.id ?? "none"}
          chat={active}
          onOpenAgent={onOpenAgent}
          ensureChat={async () => {
            const c = await api2.newChat("general", "");
            select(c.id);
            reloadChats();
            return c.id;
          }}
          suggestions={[
            { label: "Create an agent", onPick: onNewAgent },
            { label: "What did my agents learn this week?" },
            { label: "Summarize a group" },
          ]}
        />
      </div>
      {listOpen && (
        <ChatList
          chats={visible}
          activeId={activeId}
          onPick={(id) => {
            setActiveId(id);
            setHeadRename(false);
          }}
          onNew={newChat}
          renamingId={renamingId}
          setRenamingId={setRenamingId}
          onRename={rename}
          onDelete={remove}
        />
      )}
    </aside>
  );
}
