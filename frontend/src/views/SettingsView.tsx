import { useEffect, useRef, useState } from "react";
import { api, errText, on, type MCPStatus, type Settings, type SpeechModel, type Status, type UpdateStatus } from "../api";
import { ClipboardSetText } from "../../wailsjs/runtime/runtime";
import { ago, bytes } from "../format";
import { Check, Confirm, Icon, Pill } from "../components/ui";

export function SettingsView({ status, setStatus }: { status: Status | null; setStatus: (s: Status) => void }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [models, setModels] = useState<SpeechModel[]>([]);
  const [err, setErr] = useState("");
  const [savedNote, setSavedNote] = useState("");
  const [checking, setChecking] = useState(false);
  const [confirmLogout, setConfirmLogout] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    api.getSettings().then(setSettings).catch((e) => setErr(errText(e)));
    api.listSpeechModels().then(setModels).catch(() => {});
    const off = on("speech", (ms: SpeechModel[]) => setModels(ms ?? []));
    return () => {
      off();
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);

  const update = (next: Settings) => {
    setSettings(next);
    setSavedNote("");
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(async () => {
      try {
        await api.saveSettings(next);
        setErr("");
        setSavedNote("Saved.");
      } catch (e) {
        setErr(errText(e));
      }
    }, 500);
  };
  const setDefault = <K extends keyof Settings["defaults"]>(k: K, v: Settings["defaults"][K]) =>
    settings && update({ ...settings, defaults: { ...settings.defaults, [k]: v } });

  const claude = status?.claude;
  const wa = status?.whatsapp;

  return (
    <div className="page">
      <header className="page__head page__head--split">
        <h1>Settings</h1>
        {savedNote && <span className="ok-text small">{savedNote}</span>}
      </header>
      {err && (
        <div className="banner banner--bad" role="alert">
          {err}
        </div>
      )}

      <div className="cols cols--settings">
        <section className="card section" aria-labelledby="s-claude">
          <div className="section__headrow">
            <h2 id="s-claude" className="section__title">
              Claude Code
            </h2>
            {claude && (claude.found ? <Pill tone="ok">Ready</Pill> : <Pill tone="bad">Not found</Pill>)}
          </div>
          <dl className="dl">
            <dt>Command</dt>
            <dd className="mono small">{claude?.path || "—"}</dd>
            <dt>Version</dt>
            <dd>{claude?.version || "—"}</dd>
            <dt>Account</dt>
            <dd>Your subscription login. API keys are not used.</dd>
          </dl>
          {claude?.error && <span className="error-text">{claude.error}</span>}
          <div>
            <button
              type="button"
              className="btn btn--sm"
              disabled={checking}
              onClick={async () => {
                setChecking(true);
                try {
                  setStatus(await api.recheckClaude());
                } catch (e) {
                  setErr(errText(e));
                } finally {
                  setChecking(false);
                }
              }}
            >
              {checking ? "Checking…" : "Check again"}
            </button>
          </div>
        </section>

        <section className="card section" aria-labelledby="s-voice">
          <h2 id="s-voice" className="section__title">
            Voice note transcription
          </h2>
          <p className="fg2 small no-margin">Speech models run on this Mac, so voice notes never leave it. Install the languages your chats use.</p>
          <div className="models">
            {models.length === 0 && <div className="models__row muted small">Loading the model list…</div>}
            {models.map((m) => (
              <ModelRow key={m.language} m={m} onError={setErr} />
            ))}
          </div>
        </section>

        {settings && (
          <section className="card section" aria-labelledby="s-defaults">
            <h2 id="s-defaults" className="section__title">
              New agents start with
            </h2>
            <div className="grid2">
              <div className="field">
                <label htmlFor="d-mode" className="field__label">
                  Replies
                </label>
                <select id="d-mode" className="select select--block" value={settings.defaults.mode} onChange={(e) => setDefault("mode", e.target.value)}>
                  <option value="auto">On its own</option>
                  <option value="approve">After I approve</option>
                  <option value="never">Never, only learns</option>
                </select>
              </div>
              <div className="field">
                <label htmlFor="d-max" className="field__label">
                  At most
                </label>
                <div className="input input--suffix">
                  <input
                    id="d-max"
                    type="number"
                    min={1}
                    max={500}
                    value={settings.defaults.maxPerHour}
                    onChange={(e) => setDefault("maxPerHour", Math.max(1, Number(e.target.value) || 1))}
                  />
                  <span className="muted small">replies an hour</span>
                </div>
              </div>
              <div className="field">
                <label htmlFor="d-wait" className="field__label">
                  Wait for more messages
                </label>
                <div className="input input--suffix">
                  <input
                    id="d-wait"
                    type="number"
                    min={0}
                    max={600}
                    value={settings.defaults.waitSeconds}
                    onChange={(e) => setDefault("waitSeconds", Math.max(0, Number(e.target.value) || 0))}
                  />
                  <span className="muted small">seconds</span>
                </div>
              </div>
              <div className="field">
                <label htmlFor="d-prefix" className="field__label">
                  Prefix on replies
                </label>
                <input id="d-prefix" className="input" value={settings.defaults.prefix} onChange={(e) => setDefault("prefix", e.target.value)} />
              </div>
            </div>
          </section>
        )}

        {settings && (
          <section className="card section" aria-labelledby="s-wa">
            <h2 id="s-wa" className="section__title">
              WhatsApp and media
            </h2>
            <dl className="dl">
              <dt>Linked as</dt>
              <dd>{wa?.state === "connected" ? `Deputy (Mac) on ${wa.phone}` : "Not linked"}</dd>
              <dt>Media folder</dt>
              <dd className="mono small wrap-anywhere">{settings.mediaDir}</dd>
              <dt>Size</dt>
              <dd>
                {bytes(settings.mediaBytes)} · {settings.mediaFiles.toLocaleString("en")} photos and voice notes
              </dd>
            </dl>
            <Check
              id="w-clean"
              checked={settings.mediaRetentionDays > 0}
              onChange={(v) => update({ ...settings, mediaRetentionDays: v ? 30 : 0 })}
              title={settings.mediaRetentionDays > 0 ? `Delete media older than ${settings.mediaRetentionDays} days` : "Delete old media"}
            />
            {settings.mediaRetentionDays > 0 && (
              <div className="field field--indent">
                <label htmlFor="w-days" className="field__label">
                  Keep media for
                </label>
                <div className="input input--suffix input--short">
                  <input
                    id="w-days"
                    type="number"
                    min={1}
                    max={3650}
                    value={settings.mediaRetentionDays}
                    onChange={(e) => update({ ...settings, mediaRetentionDays: Math.max(1, Number(e.target.value) || 1) })}
                  />
                  <span className="muted small">days</span>
                </div>
              </div>
            )}
            <div className="row">
              <button type="button" className="btn btn--sm" onClick={() => api.showMediaFolder().catch((e) => setErr(errText(e)))}>
                <Icon name="folder" size={15} />
                Show in Finder
              </button>
              {wa?.state === "connected" && (
                <button type="button" className="btn btn--sm btn--danger-quiet" onClick={() => setConfirmLogout(true)}>
                  Log out of WhatsApp
                </button>
              )}
            </div>
            {confirmLogout && (
              <Confirm
                danger
                text="Log out of WhatsApp? Agents stop until you link again."
                yes="Log out"
                onNo={() => setConfirmLogout(false)}
                onYes={async () => {
                  setConfirmLogout(false);
                  try {
                    await api.logout();
                  } catch (e) {
                    setErr(errText(e));
                  }
                }}
              />
            )}
          </section>
        )}

        {settings && (
          <section className="card section" aria-labelledby="s-app">
            <h2 id="s-app" className="section__title">
              App
            </h2>
            <Check
              id="a-login"
              checked={settings.openAtLogin}
              onChange={(v) => update({ ...settings, openAtLogin: v })}
              title="Open at login"
              help="Agents only work while Deputy is running. Closing the window keeps it running in the menu bar."
            />
            <Check id="a-notify" checked={settings.notify} onChange={(v) => update({ ...settings, notify: v })} title="Notify me when a reply needs approval" />
            <div className="section__foot">
              <span className="muted small">Deputy {status?.version || ""}</span>
            </div>
          </section>
        )}

        <ClaudeCodeCard onEnabled={(v) => setSettings((cur) => (cur ? { ...cur, mcpEnabled: v } : cur))} />

        <UpdatesCard />
      </div>
    </div>
  );
}

/** ClaudeCodeCard turns on the MCP server that lets Claude Code in other projects use Deputy. */
function ClaudeCodeCard({ onEnabled }: { onEnabled: (v: boolean) => void }) {
  const [st, setSt] = useState<MCPStatus | null>(null);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [note, setNote] = useState("");
  const [out, setOut] = useState("");

  useEffect(() => {
    api.getMCPStatus().then(setSt).catch((e) => setErr(errText(e)));
  }, []);

  const act = async (what: string, f: () => Promise<void>) => {
    setBusy(what);
    setErr("");
    setNote("");
    try {
      await f();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy("");
    }
  };

  const host = st?.url ? st.url.replace(/^https?:\/\//, "").replace(/\/mcp$/, "") : "";

  return (
    <section className="card section" aria-labelledby="s-mcp">
      <div className="section__headrow">
        <h2 id="s-mcp" className="section__title">
          Claude Code in other projects
        </h2>
        {st?.running && <Pill tone="ok">Running</Pill>}
      </div>
      <Check
        id="m-on"
        checked={!!st?.enabled}
        disabled={!st || !!busy}
        onChange={(v) =>
          act("toggle", async () => {
            setSt(await api.setMCPEnabled(v));
            onEnabled(v);
          })
        }
        title="Allow Claude Code in other projects"
        help="Claude Code in any folder can then use Deputy as you: read chats, send WhatsApp messages, ask agents, approve replies and change agents. Only this Mac can connect, with a secret token."
      />
      {st?.enabled && (
        <p className="small no-margin" role="status">
          {st.running ? (
            <span className="ok-text">Running on {host}</span>
          ) : st.error ? (
            <span className="error-text">{st.error}</span>
          ) : (
            <span className="muted">Starting…</span>
          )}
        </p>
      )}
      {st?.enabled && (
        <div className="row">
          <button
            type="button"
            className="btn btn--sm"
            disabled={!!busy}
            onClick={() =>
              act("add", async () => {
                setOut("");
                setOut(await api.addMCPToClaude());
                setNote("Added. New Claude Code sessions have Deputy's tools; check with /mcp.");
              })
            }
          >
            {busy === "add" ? "Adding…" : "Add to Claude Code"}
          </button>
          <button
            type="button"
            className="btn btn--quiet btn--sm"
            disabled={!!busy || !st.command}
            onClick={() =>
              act("copy", async () => {
                const ok = await ClipboardSetText(st.command).catch(() => false);
                if (!ok) await navigator.clipboard.writeText(st.command);
                setNote("Copied. Run it in a terminal. It holds your token, so keep it to yourself.");
              })
            }
          >
            Copy command
          </button>
          <button
            type="button"
            className="btn btn--quiet btn--sm"
            disabled={!!busy}
            onClick={() =>
              act("rotate", async () => {
                setSt(await api.rotateMCPToken());
                setOut("");
                setNote("New token made. The old one stopped working: click Add to Claude Code again.");
              })
            }
          >
            New token
          </button>
        </div>
      )}
      {note && <span className="ok-text small">{note}</span>}
      {err && <span className="error-text">{err}</span>}
      {out && (
        <pre className="update-log__out" tabIndex={0} aria-label="claude mcp add output">
          {out}
        </pre>
      )}
    </section>
  );
}

const MAX_UPDATE_LINES = 5000;
const RUNNING = ["checking", "downloading", "verifying", "installing"];

function UpdatesCard() {
  const [u, setU] = useState<UpdateStatus | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [token, setToken] = useState("");
  const [tokenOpen, setTokenOpen] = useState(false);
  const lastSeq = useRef(0);
  const logRef = useRef<HTMLPreElement>(null);

  const load = (s: UpdateStatus) => {
    setU(s);
    if (s.seq >= lastSeq.current) {
      setLines(s.lines ?? []);
      lastSeq.current = s.seq;
    }
  };
  const refresh = () => api.getUpdateStatus().then(load).catch((e) => setErr(errText(e)));

  useEffect(() => {
    refresh();
    const offOut = on("update:output", (e: { seq: number; line: string }) => {
      if (!e || e.seq <= lastSeq.current) return;
      lastSeq.current = e.seq;
      setLines((l) => (l.length >= MAX_UPDATE_LINES ? [...l.slice(l.length - MAX_UPDATE_LINES + 1), e.line] : [...l, e.line]));
    });
    const offState = on("update:state", (e: { state: string; message: string }) => {
      if (!e) return;
      if (e.state === "checking") setLines([]);
      setU((cur) => (cur ? { ...cur, state: e.state, message: e.message ?? "" } : cur));
      // A finished run may have a newer check result.
      if (e.state === "idle" || e.state === "failed") refresh();
    });
    return () => {
      offOut();
      offState();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  const act = async (what: string, f: () => Promise<unknown>) => {
    setBusy(what);
    setErr("");
    try {
      await f();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy("");
    }
  };

  const state = u?.state ?? "idle";
  const running = RUNNING.includes(state);
  const latest = u?.latest;
  const stateText =
    state === "checking"
      ? "Checking GitHub…"
      : state === "downloading"
        ? "Downloading…"
        : state === "verifying"
          ? "Checking the download and its signature…"
          : state === "installing"
            ? u?.message || "Deputy quits now and reopens when the new version is in place."
            : "";

  return (
    <section className="card section" aria-labelledby="s-updates">
      <h2 id="s-updates" className="section__title">
        Updates
      </h2>
      <dl className="dl">
        <dt>Version</dt>
        <dd className="mono small">{u?.version || "—"}</dd>
      </dl>
      {latest && !running && state !== "failed" &&
        (u?.upToDate ? (
          <span className="ok-text small row row--tight strong">
            <Icon name="check" size={14} strokeWidth={2.4} />
            Deputy is up to date
          </span>
        ) : (
          <p className="small no-margin">
            New version: <span className="mono">{latest.tag}</span>
            {latest.name && latest.name !== latest.tag ? ` · ${latest.name}` : ""} · {ago(latest.date)}
          </p>
        ))}
      {u && !u.installedApp && (
        <p className="muted small no-margin">Deputy is not running from an app bundle, so it cannot update itself.</p>
      )}
      {running && (
        <span className="fg2 small" role="status">
          {stateText}
        </span>
      )}
      {state === "idle" && u?.message && !latest && <span className="muted small">{u.message}</span>}
      {state === "failed" && u?.message && <span className="error-text">{u.message}</span>}
      {err && <span className="error-text">{err}</span>}
      <div className="row">
        <button type="button" className="btn btn--sm" disabled={!!busy || running} onClick={() => act("check", async () => load(await api.checkForUpdate()))}>
          {busy === "check" ? "Checking…" : "Check for updates"}
        </button>
        {latest && !u?.upToDate && (
          <button
            type="button"
            className="btn btn--sm btn--primary"
            disabled={!!busy || running || !u?.installedApp}
            onClick={() =>
              act("update", async () => {
                await api.startUpdate();
                await refresh();
              })
            }
          >
            {running ? "Updating…" : "Update Deputy"}
          </button>
        )}
      </div>
      <div className="update-token">
        <p className="muted small no-margin">
          Updates come from the releases of <span className="mono">{u?.repo || "GabiHert/deputy"}</span>.{" "}
          {u?.hasToken
            ? "A GitHub token is saved in your keychain."
            : "For a private repo, save a fine-grained GitHub token with read-only Contents access to that repo."}{" "}
          <button type="button" className="btn btn--sm btn--quiet" onClick={() => setTokenOpen((o) => !o)} aria-expanded={tokenOpen}>
            {u?.hasToken ? "Change token" : "Add token"}
          </button>
        </p>
        {tokenOpen && (
          <form
            className="row"
            onSubmit={(e) => {
              e.preventDefault();
              act("token", async () => {
                load(await api.setUpdateToken(token));
                setToken("");
                setTokenOpen(false);
              });
            }}
          >
            <input
              className="input input--sm mono"
              type="password"
              autoComplete="off"
              spellCheck={false}
              placeholder="github_pat_…"
              aria-label="GitHub token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <button type="submit" className="btn btn--sm" disabled={!!busy || !token.trim()}>
              Save in keychain
            </button>
            {u?.hasToken && (
              <button
                type="button"
                className="btn btn--sm btn--danger-quiet"
                disabled={!!busy}
                onClick={() =>
                  act("token", async () => {
                    load(await api.setUpdateToken(""));
                    setTokenOpen(false);
                  })
                }
              >
                Remove
              </button>
            )}
          </form>
        )}
      </div>
      {lines.length > 0 && (
        <details className="update-log" open={running || state === "failed"}>
          <summary className="muted small">Output ({lines.length.toLocaleString("en")} lines)</summary>
          <pre ref={logRef} className="update-log__out" tabIndex={0} aria-label="Update output">
            {lines.join("\n")}
          </pre>
        </details>
      )}
    </section>
  );
}

function ModelRow({ m, onError }: { m: SpeechModel; onError: (s: string) => void }) {
  const pct = m.total > 0 ? Math.min(100, Math.round((m.done / m.total) * 100)) : 0;
  const phase =
    m.phase === "verify" ? "Checking the download" : m.phase === "extract" ? "Unpacking" : `Downloading · ${bytes(m.done)} of ${bytes(m.total || m.sizeBytes)}`;
  return (
    <div className="models__row">
      <div className="models__line">
        <div className="models__text">
          <span className="strong">{m.label}</span>
          <span className="muted xsmall">
            {m.installed ? "Installed" : m.downloading ? phase : `Not installed · ${bytes(m.sizeBytes)}`}
          </span>
        </div>
        {m.installed ? (
          <span className="ok-text small row row--tight strong">
            <Icon name="check" size={14} strokeWidth={2.4} />
            Ready
          </span>
        ) : m.downloading ? (
          <button type="button" className="btn btn--sm" onClick={() => api.cancelSpeechDownload(m.language)}>
            Cancel
          </button>
        ) : (
          <button type="button" className="btn btn--sm" onClick={() => api.downloadSpeechModel(m.language).catch((e) => onError(errText(e)))}>
            Download
          </button>
        )}
      </div>
      {m.downloading && (
        <div
          className="progress"
          role="progressbar"
          aria-label={`${m.label} model download`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={pct}
        >
          <span className="progress__fill" style={{ width: `${pct}%` }} />
        </div>
      )}
      {m.error && <span className="error-text">{m.error}</span>}
    </div>
  );
}
