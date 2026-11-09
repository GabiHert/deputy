import { useEffect, useMemo, useRef, useState } from "react";
import { api, api2, errText, on, useAssistantChats, type Agent, type Chat, type MCPServer } from "../api";
import { Conversation } from "../components/Assistant";
import { Markdown } from "../lib/markdown";
import { when } from "../format";
import { Check, Confirm, Icon, Segmented, SrLabel, Switch } from "../components/ui";

export function AgentView({
  id,
  agents,
  onSaved,
  onDeleted,
}: {
  id: string;
  agents: Agent[];
  onSaved: (id: string) => void;
  onDeleted: () => void;
}) {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [saved, setSaved] = useState<string>("");
  const [chats, setChats] = useState<Chat[]>([]);
  const [err, setErr] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [editText, setEditText] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const dirtyRef = useRef(false);

  useEffect(() => {
    let alive = true;
    (id ? api.getAgent(id) : api.newAgent())
      .then((a) => {
        if (!alive) return;
        const norm = { ...a, chats: a.chats ?? [], except: a.except ?? [], mcp: a.mcp ?? [], folders: a.folders ?? [] };
        setAgent(norm);
        setSaved(JSON.stringify(norm));
      })
      .catch((e) => alive && setErr(errText(e)));
    return () => {
      alive = false;
    };
  }, [id]);

  // Claude may change the agent (a change card applied): reload when nothing is being edited here.
  useEffect(() => {
    if (!id) return;
    return on("agents", () => {
      if (dirtyRef.current) return;
      api
        .getAgent(id)
        .then((a) => {
          if (dirtyRef.current) return;
          const norm = { ...a, chats: a.chats ?? [], except: a.except ?? [], mcp: a.mcp ?? [], folders: a.folders ?? [] };
          setAgent(norm);
          setSaved(JSON.stringify(norm));
        })
        .catch(() => {});
    });
  }, [id]);

  useEffect(() => {
    const load = () => api.listChats().then(setChats).catch(() => {});
    load();
    return on("chats", load);
  }, []);

  const dirty = agent !== null && JSON.stringify(agent) !== saved;
  dirtyRef.current = dirty;
  const set = <K extends keyof Agent>(k: K, v: Agent[K]) => {
    setNotice("");
    setAgent((a) => (a ? { ...a, [k]: v } : a));
  };

  const chatName = useMemo(() => {
    const m = new Map(chats.map((c) => [c.jid, c]));
    return (jid: string) => m.get(jid)?.name || jid.split("@")[0];
  }, [chats]);
  const addable = chats.filter((c) => agent && !agent.chats.includes(c.jid));

  if (!agent) {
    return <div className="page">{err ? <div className="banner banner--bad">{err}</div> : <p className="muted">Loading…</p>}</div>;
  }

  const save = async () => {
    setBusy(true);
    setErr("");
    try {
      const out = await api.saveAgent(agent);
      const norm = { ...out, chats: out.chats ?? [], except: out.except ?? [], mcp: out.mcp ?? [], folders: out.folders ?? [] };
      setAgent(norm);
      setSaved(JSON.stringify(norm));
      setNotice("Saved.");
      if (!id) onSaved(out.id);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  const chooseRepo = async () => {
    try {
      const p = await api.chooseFolder();
      if (p) set("repoPath", p);
    } catch (e) {
      setErr(errText(e));
    }
  };

  const otherOwner = (jid: string) => agents.find((a) => a.id !== agent.id && (a.chats ?? []).includes(jid));
  const isNew = !agent.id;
  const allOwner = agents.find((a) => a.id !== agent.id && a.allDirect);
  const skippable = chats.filter((c) => !c.isGroup && !agent.except.includes(c.jid) && !agent.chats.includes(c.jid));
  const modeHelp: Record<string, string> = {
    auto: "Replies go out as soon as the agent writes them.",
    approve: "Replies wait in Activity until you send them.",
    never: "The agent reads and learns, and never writes in the chat.",
  };

  return (
    <div className="page">
      <header className="page__head page__head--split">
        <div className="row row--center">
          <h1>{agent.name || (isNew ? "New agent" : "Untitled agent")}</h1>
          <Switch on={agent.enabled} onChange={(v) => set("enabled", v)} label="Agent on" />
        </div>
        <div className="row">
          {dirty && <span className="muted small">Unsaved changes</span>}
          {!dirty && notice && <span className="ok-text small">{notice}</span>}
          <button type="button" className="btn btn--primary" onClick={save} disabled={busy || (!dirty && !isNew)}>
            {busy ? "Saving…" : isNew ? "Create agent" : "Save"}
          </button>
        </div>
      </header>
      {err && (
        <div className="banner banner--bad" role="alert">
          {err}
        </div>
      )}

      <div className="cols">
        <div className="col">
          <section className="card section behaves" aria-labelledby="h-who">
            <div className="behaves__head">
              <div>
                <h2 id="h-who" className="section__title">
                  How it behaves
                </h2>
                <span className="muted small">
                  <span className="mono">INSTRUCTIONS.md</span> · Claude writes and refines it from your chats and corrections.
                </span>
              </div>
              <div className="row row--tight">
                {!isNew && (
                  <button type="button" className={chatOpen ? "btn btn--primary btn--sm btn--pressed" : "btn btn--primary btn--sm"} aria-expanded={chatOpen} onClick={() => setChatOpen(!chatOpen)}>
                    <Icon name="chats" size={15} />
                    Change with Claude
                  </button>
                )}
                <button type="button" className="btn btn--sm" aria-pressed={editText} onClick={() => setEditText(!editText)}>
                  {editText ? "Done editing" : "Edit text"}
                </button>
              </div>
            </div>
            {editText ? (
              <div className="field">
                <label htmlFor="a-ins" className="field__label">
                  Instructions
                </label>
                <textarea
                  id="a-ins"
                  className="input input--prose"
                  rows={12}
                  value={agent.instructions}
                  onChange={(e) => set("instructions", e.target.value)}
                  placeholder="Who the agent is, what it answers, how it talks, and what it should write down."
                />
                <span className="muted small">Save to write it to INSTRUCTIONS.md and commit it.</span>
              </div>
            ) : agent.instructions.trim() ? (
              <Markdown text={agent.instructions} className="instructions" />
            ) : (
              <span className="muted small">No instructions yet. Use Change with Claude and say what this agent should do.</span>
            )}
            {chatOpen && !isNew && <AgentChat agentId={agent.id} agentName={agent.name} />}
          </section>

          <section className="card section" aria-labelledby="h-chats">
            <h2 id="h-chats" className="section__title">
              Name and chats
            </h2>
            <div className="field">
              <label htmlFor="a-name" className="field__label">
                Name
              </label>
              <input id="a-name" className="input" value={agent.name} onChange={(e) => set("name", e.target.value)} placeholder="Checagem" />
            </div>
            <div className="chips">
              {agent.chats.length === 0 && (
                <span className="muted small">
                  {agent.allDirect ? "No group chats picked. It handles your direct chats." : "No chats yet. The agent does nothing until you add one."}
                </span>
              )}
              {agent.chats.map((jid) => (
                <span key={jid} className="chip">
                  {chatName(jid)}
                  <button
                    type="button"
                    className="chip__x"
                    aria-label={`Remove ${chatName(jid)}`}
                    onClick={() => set("chats", agent.chats.filter((c) => c !== jid))}
                  >
                    <Icon name="close" size={14} strokeWidth={2} />
                  </button>
                </span>
              ))}
              <SrLabel htmlFor="add-chat">Add chat</SrLabel>
              <select
                id="add-chat"
                className="chip-add"
                value=""
                onChange={(e) => e.target.value && set("chats", [...agent.chats, e.target.value])}
              >
                <option value="">+ Add chat</option>
                {addable.map((c) => {
                  const owner = otherOwner(c.jid);
                  return (
                    <option key={c.jid} value={c.jid}>
                      {c.name}
                      {owner ? ` (moves from ${owner.name})` : ""}
                    </option>
                  );
                })}
              </select>
            </div>
            <Check
              id="a-alldirect"
              checked={agent.allDirect}
              onChange={(v) => set("allDirect", v)}
              title="Also handle every direct chat"
              help={
                allOwner
                  ? `Takes this over from ${allOwner.name || "another agent"}. Direct chats you gave another agent stay with that agent.`
                  : "Every one-to-one chat, including new ones. Direct chats you gave another agent stay with that agent. Groups are never included."
              }
            />
            {agent.allDirect && (
              <div className="chips">
                <span className="muted small">Leaves alone:</span>
                {agent.except.length === 0 && <span className="muted small">nobody</span>}
                {agent.except.map((jid) => (
                  <span key={jid} className="chip">
                    {chatName(jid)}
                    <button
                      type="button"
                      className="chip__x"
                      aria-label={`Stop leaving ${chatName(jid)} alone`}
                      onClick={() => set("except", agent.except.filter((c) => c !== jid))}
                    >
                      <Icon name="close" size={14} strokeWidth={2} />
                    </button>
                  </span>
                ))}
                <SrLabel htmlFor="add-except">Leave a direct chat alone</SrLabel>
                <select
                  id="add-except"
                  className="chip-add"
                  value=""
                  onChange={(e) => e.target.value && set("except", [...agent.except, e.target.value])}
                >
                  <option value="">+ Leave a chat alone</option>
                  {skippable.map((c) => (
                    <option key={c.jid} value={c.jid}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </section>

          <section className="card section" aria-labelledby="h-when">
            <h2 id="h-when" className="section__title">
              When it answers
            </h2>
            <div className="field">
              <Segmented
                id="l-mode"
                label="Replies"
                value={agent.mode}
                onChange={(v) => set("mode", v)}
                options={[
                  { value: "auto", label: "On its own" },
                  { value: "approve", label: "After I approve" },
                  { value: "never", label: "Never, only learns" },
                ]}
              />
              <span className="muted small">{modeHelp[agent.mode] ?? ""}</span>
            </div>
            <div className="field">
              <Segmented
                id="l-trig"
                label="Reacts to"
                value={agent.trigger}
                onChange={(v) => set("trigger", v)}
                options={[
                  { value: "all", label: "Every message" },
                  { value: "mention", label: "Mentions and replies to me" },
                ]}
              />
              {agent.trigger === "mention" && <span className="muted small">In direct chats every message counts.</span>}
            </div>
            <Check
              id="a-answerme"
              checked={agent.answerMe}
              onChange={(v) => set("answerMe", v)}
              title="Answer my own messages"
              help="Every message you type in its chats starts a run, so you can talk to it. Best in a chat that's just you, such as a group with only yourself. Its own replies never count."
            />
            <div className="grid3">
              <NumberField id="a-wait" label="Wait for more messages" suffix="seconds" value={agent.waitSeconds} min={0} max={600} onChange={(v) => set("waitSeconds", v)} />
              <NumberField id="a-max" label="At most" suffix="replies an hour" value={agent.maxPerHour} min={1} max={500} onChange={(v) => set("maxPerHour", v)} />
              <div className="field">
                <label htmlFor="a-prefix" className="field__label">
                  Prefix on replies
                </label>
                <input id="a-prefix" className="input" value={agent.prefix} onChange={(e) => set("prefix", e.target.value)} />
              </div>
            </div>
          </section>
        </div>

        <div className="col">
          <section className="card section" aria-labelledby="h-repo">
            <h2 id="h-repo" className="section__title">
              Knowledge repo
            </h2>
            <div className="row row--nowrap">
              <SrLabel htmlFor="a-repo">Repo folder</SrLabel>
              <input
                id="a-repo"
                className="input input--mono grow"
                value={agent.repoPath}
                onChange={(e) => set("repoPath", e.target.value)}
                placeholder="~/Documents/agent-kb"
              />
              <button type="button" className="btn" onClick={chooseRepo}>
                Choose…
              </button>
            </div>
            {agent.stats?.exists ? (
              <div className="stats3">
                <div>
                  <div className="stat__num">{agent.stats.files}</div>
                  <div className="muted xsmall">knowledge files</div>
                </div>
                <div>
                  <div className="stat__num">{agent.stats.commits}</div>
                  <div className="muted xsmall">commits</div>
                </div>
                <div>
                  <div className="stat__num">{agent.stats.lastLearned ? when(agent.stats.lastLearned) : "—"}</div>
                  <div className="muted xsmall">last learned</div>
                </div>
              </div>
            ) : (
              <span className="muted small">
                Pick an empty folder or an existing repo. If it is empty, Positron sets it up with CLAUDE.md, INSTRUCTIONS.md and a
                knowledge folder.
              </span>
            )}
            <Check
              id="a-push"
              checked={agent.push}
              onChange={(v) => set("push", v)}
              title="Push after each commit"
              help={agent.stats?.remote ? <span className="mono">{agent.stats.remote}</span> : "Needs a remote on the repo."}
            />
          </section>

          <section className="card section" aria-labelledby="h-media">
            <h2 id="h-media" className="section__title">
              Photos and voice notes
            </h2>
            <Check id="m-img" checked={agent.readImages} onChange={(v) => set("readImages", v)} title="Look at photos people send" help="Claude Code sees the image and its caption. Stickers are skipped." />
            <Check
              id="m-voice"
              checked={agent.transcribeVoice}
              onChange={(v) => set("transcribeVoice", v)}
              title="Transcribe voice notes on this Mac"
              help="Audio never leaves your Mac. The agent reads the transcript. Install the language's model in Settings."
            />
            <div className="field field--indent">
              <label htmlFor="a-lang" className="field__label">
                Language
              </label>
              <select id="a-lang" className="select select--block" value={agent.language} onChange={(e) => set("language", e.target.value)}>
                <option value="pt-BR">Portuguese (Brazil)</option>
                <option value="en">English</option>
              </select>
            </div>
            <Check id="m-send" checked={agent.sendImages} onChange={(v) => set("sendImages", v)} title="Send images in replies" help="The agent can attach images from its repo, such as charts or screenshots it saved." />
            <Check
              id="m-voiceout"
              checked={agent.voiceReplies}
              onChange={(v) => set("voiceReplies", v)}
              title="Answer voice notes with a voice note"
              help="Uses the macOS voice for the agent's language."
            />
          </section>

          <section className="card section" aria-labelledby="h-tools">
            <h2 id="h-tools" className="section__title">
              What it can do
            </h2>
            <Check id="t-repo" checked disabled title="Read and write files in its repo" help="Always on. This is how it remembers." />
            <Check id="t-web" checked={agent.web} onChange={(v) => set("web", v)} title="Search and read the web" />
            <Check
              id="t-bash"
              checked={agent.bash}
              onChange={(v) => set("bash", v)}
              title="Run shell commands"
              helpTone="warn"
              help="Anyone in its chats could get it to run commands on your Mac. Leave off unless you trust every member."
            />
            <FolderAccess
              level={agent.folderAccess}
              folders={agent.folders}
              bash={agent.bash}
              onLevel={(v) => set("folderAccess", v)}
              onFolders={(v) => set("folders", v)}
              onError={setErr}
            />
            <MCPAccess value={agent.mcp} onChange={(v) => set("mcp", v)} />
            <div className="field">
              <label htmlFor="t-model" className="field__label">
                Model
              </label>
              <select id="t-model" className="select select--block" value={agent.model} onChange={(e) => set("model", e.target.value)}>
                <option value="">Default (your Claude Code setting)</option>
                <option value="sonnet">Sonnet · fast and cheaper</option>
                <option value="opus">Opus · most capable</option>
                <option value="haiku">Haiku · fastest</option>
              </select>
            </div>
          </section>

          {!isNew && (
            <section className="card section section--danger" aria-labelledby="h-del">
              <h2 id="h-del" className="section__title">
                Delete agent
              </h2>
              <span className="muted small">Its chats are left alone afterwards. The knowledge repo stays on disk.</span>
              {confirmDelete ? (
                <Confirm
                  danger
                  text={`Delete ${agent.name || "this agent"}?`}
                  yes="Delete agent"
                  onNo={() => setConfirmDelete(false)}
                  onYes={async () => {
                    try {
                      await api.deleteAgent(agent.id);
                      onDeleted();
                    } catch (e) {
                      setErr(errText(e));
                      setConfirmDelete(false);
                    }
                  }}
                />
              ) : (
                <div>
                  <button type="button" className="btn btn--danger-quiet" onClick={() => setConfirmDelete(true)}>
                    Delete agent
                  </button>
                </div>
              )}
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

function NumberField({
  id,
  label,
  suffix,
  value,
  min,
  max,
  onChange,
}: {
  id: string;
  label: string;
  suffix: string;
  value: number;
  min: number;
  max: number;
  onChange: (v: number) => void;
}) {
  return (
    <div className="field">
      <label htmlFor={id} className="field__label">
        {label}
      </label>
      <div className="input input--suffix">
        <input
          id={id}
          type="number"
          min={min}
          max={max}
          value={Number.isFinite(value) ? value : 0}
          onChange={(e) => onChange(Math.max(min, Math.min(max, Number(e.target.value) || 0)))}
        />
        <span className="muted small">{suffix}</span>
      </div>
    </div>
  );
}

/** AgentChat is the inline Claude chat about one agent; it reuses that agent's latest chat. */
function AgentChat({ agentId, agentName }: { agentId: string; agentName: string }) {
  const [chats, reloadChats] = useAssistantChats();
  const [createdId, setCreatedId] = useState("");
  const latest = chats
    .filter((c) => c.purpose === "agent" && c.agentId === agentId)
    .sort((a, b) => b.updatedAt - a.updatedAt)[0];
  const chat = chats.find((c) => c.id === createdId) ?? latest ?? null;
  return (
    <div className="agent-chat">
      <div className="eyebrow">Chat with Claude about {agentName || "this agent"}</div>
      <Conversation
        key={chat?.id ?? "new"}
        chat={chat}
        ensureChat={async () => {
          const c = await api2.newChat("agent", agentId);
          reloadChats();
          setCreatedId(c.id);
          return c.id;
        }}
        placeholder="Tell Claude what to change, like “be warmer with Vó”"
        suggestions={[{ label: "What does it know so far?" }, { label: "Read its last replies and suggest improvements" }]}
      />
    </div>
  );
}

const statusText: Record<string, string> = { connected: "Connected", failed: "Not connecting", "needs auth": "Needs sign-in", pending: "Waiting for approval" };

/** The person's Claude Code MCP servers, each one a tick box for this agent. */
function MCPAccess({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const load = (refresh: boolean) => {
    setBusy(true);
    setErr("");
    api
      .listMCPServers(refresh)
      .then(setServers)
      .catch((e) => setErr(errText(e)))
      .finally(() => setBusy(false));
  };
  useEffect(() => load(false), []);
  const known = new Set((servers ?? []).map((s) => s.name));
  const gone = value.filter((n) => servers && !known.has(n));
  const toggle = (name: string, on: boolean) => onChange(on ? [...value, name] : value.filter((n) => n !== name));
  return (
    <fieldset className="mcp-access">
      <legend className="field__label">MCP servers it can use</legend>
      <p className="muted small">
        The servers your Claude Code has, like your calendar or Notion. The agent acts as you with them, and people in its chats can ask it
        to use them. Turn on only what it needs.
      </p>
      {busy && !servers && <span className="muted small">Checking your MCP servers… this takes a few seconds.</span>}
      {err && <span className="error small">{err}</span>}
      {servers && servers.length === 0 && <span className="muted small">Your Claude Code has no MCP servers. Add them with claude mcp add.</span>}
      {servers?.map((s, i) => (
        <Check
          key={s.name}
          id={`mcp-${i}`}
          checked={value.includes(s.name)}
          onChange={(v) => toggle(s.name, v)}
          title={s.name}
          help={`${statusText[s.status] ?? s.status} · ${s.target}`}
          helpTone={s.status === "connected" ? undefined : "warn"}
        />
      ))}
      {gone.map((n) => (
        <Check key={n} id={`mcp-gone-${n}`} checked onChange={() => toggle(n, false)} title={n} help="No longer in your Claude Code. Untick to remove." helpTone="warn" />
      ))}
      {servers && (
        <button type="button" className="btn btn--quiet btn--sm" onClick={() => load(true)} disabled={busy}>
          {busy ? "Checking…" : "Check again"}
        </button>
      )}
    </fieldset>
  );
}

const FOLDER_LEVELS: { id: string; label: string; help: string }[] = [
  { id: "", label: "Off", help: "Only its own knowledge repo." },
  { id: "read", label: "Read files", help: "It can read and search the folders below, but not change them." },
  { id: "edit", label: "Read and change files", help: "It can also edit and create files in those folders." },
];

/** Folders on the Mac the agent may use beyond its repo, in its WhatsApp runs and its side chats. */
function FolderAccess({
  level,
  folders,
  bash,
  onLevel,
  onFolders,
  onError,
}: {
  level: string;
  folders: string[];
  bash: boolean;
  onLevel: (v: string) => void;
  onFolders: (v: string[]) => void;
  onError: (t: string) => void;
}) {
  const add = async () => {
    try {
      const p = await api.chooseFolder();
      if (p && !folders.includes(p)) onFolders([...folders, p]);
    } catch (e) {
      onError(errText(e));
    }
  };
  const short = (p: string) => p.replace(/^\/Users\/[^/]+/, "~");
  return (
    <fieldset className="folder-access">
      <legend className="field__label">Computer access</legend>
      <div className="folder-access__levels" role="radiogroup" aria-label="Computer access">
        {FOLDER_LEVELS.map((l) => (
          <label key={l.id || "off"} className={level === l.id ? "folder-access__lvl folder-access__lvl--on" : "folder-access__lvl"}>
            <input type="radio" name="folder-access" checked={level === l.id} onChange={() => onLevel(l.id)} />
            <span>
              <span className="folder-access__name">{l.label}</span>
              <span className="muted xsmall">{l.help}</span>
            </span>
          </label>
        ))}
      </div>
      {level && (
        <div className="chips">
          {folders.length === 0 && <span className="muted small">Pick the folders it may use, like a project's code.</span>}
          {folders.map((f) => (
            <span key={f} className="chip" title={f}>
              <Icon name="folder" size={13} />
              {short(f)}
              <button type="button" className="chip__x" aria-label={`Remove ${f}`} onClick={() => onFolders(folders.filter((x) => x !== f))}>
                <Icon name="close" size={14} strokeWidth={2} />
              </button>
            </span>
          ))}
          <button type="button" className="chip-add" onClick={add}>
            + Add folder
          </button>
        </div>
      )}
      {level && (
        <span className="check__help check__help--warn">
          This applies when it answers WhatsApp and when you ask it something in Sessions. People in its chats can ask it about
          these files{level === "edit" ? " or to change them" : ""}
          {bash ? ", and with shell commands on it can reach the rest of your Mac too" : ""}.
        </span>
      )}
    </fieldset>
  );
}
