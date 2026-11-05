import { useEffect, useRef, useState } from "react";
import { api, errText, on, type Settings, type SpeechModel, type Status } from "../api";
import { bytes } from "../format";
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
      </div>
    </div>
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
