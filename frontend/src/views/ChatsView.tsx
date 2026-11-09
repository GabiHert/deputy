import { useEffect, useState } from "react";
import { api, errText, type Agent, type Status } from "../api";
import { secondsLeft } from "../format";
import { Icon } from "../components/ui";
import { Inbox } from "./inbox/Inbox";

/** ChatsView is the WhatsApp chats page: linking WhatsApp first, then the inbox. */
export function ChatsView({
  status,
  agents,
  open,
  onOpenAgent,
  onNewAgent,
}: {
  status: Status | null;
  agents: Agent[];
  /** a chat to open (notification click or menu bar) */
  open: { jid: string; n: number } | null;
  onOpenAgent: (id: string) => void;
  onNewAgent: () => void;
}) {
  if (!status)
    return (
      <div className="chats-scroll">
        <div className="page">
          <p className="muted">Starting Positron…</p>
        </div>
      </div>
    );
  if (status.whatsapp.state !== "connected")
    return (
      <div className="chats-scroll">
        <ConnectScreen status={status} />
      </div>
    );
  return <Inbox agents={agents} open={open} onOpenAgent={onOpenAgent} onNewAgent={onNewAgent} />;
}

/* ---------- Connect ---------- */

function ConnectScreen({ status }: { status: Status }) {
  const wa = status.whatsapp;
  const [now, setNow] = useState(Date.now());
  const [phoneMode, setPhoneMode] = useState(false);
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  const left = secondsLeft(wa.qrExpiresAt, now);

  let body;
  if (wa.state === "qr" && wa.qr && !phoneMode) {
    body = (
      <div className="qr__code">
        <img className="qr__img" src={wa.qr} alt="QR code to link WhatsApp" width={256} height={256} />
        {wa.qrExpiresAt > 0 && (
          <span className="muted small row row--tight">
            <Icon name="refresh" size={14} strokeWidth={2} />
            {left > 0 ? `New code in ${left} s` : "Getting a new code…"}
          </span>
        )}
      </div>
    );
  } else if (phoneMode) {
    body = <PhoneLink />;
  } else {
    body = (
      <div className="qr__code qr__code--wait">
        <div className="spinner" aria-hidden="true" />
        <span className="muted small">
          {wa.state === "pairing"
            ? "Linked. Syncing your chats…"
            : wa.state === "disconnected"
              ? "Reconnecting to WhatsApp…"
              : wa.state === "error"
                ? "Could not reach WhatsApp."
                : "Getting a code from WhatsApp…"}
        </span>
      </div>
    );
  }

  const step2Done = wa.state === "pairing";
  return (
    <div className="page">
      <header className="page__head page__head--narrow">
        <h1>{wa.state === "disconnected" ? "Reconnecting to WhatsApp" : "Connect your WhatsApp"}</h1>
        <p className="lede">
          Positron links to your own number as a linked device, the same way WhatsApp Web does. Your agents then answer only the
          chats you give them.
        </p>
      </header>

      {wa.error && <div className="banner banner--bad">{wa.error}</div>}

      <section className="card card--raised qr" aria-label="Link WhatsApp">
        {body}
        <div className="qr__steps">
          <ol className="steps">
            <li>Open WhatsApp on your phone.</li>
            <li>
              Tap <strong>Settings</strong>, then <strong>Linked devices</strong>.
            </li>
            {phoneMode ? (
              <li>
                Tap <strong>Link a device</strong>, then <strong>Link with phone number instead</strong>, and type the code shown here.
              </li>
            ) : (
              <li>
                Tap <strong>Link a device</strong> and point the camera at this code.
              </li>
            )}
          </ol>
          <p className="muted small">
            Positron stays linked when you quit it. To unlink, use <strong>Log out</strong> in Settings or remove it from Linked devices
            on your phone.
          </p>
          <div className="row">
            <button type="button" className="btn" onClick={() => setPhoneMode(!phoneMode)}>
              {phoneMode ? "Use the QR code instead" : "Link with phone number instead"}
            </button>
          </div>
        </div>
      </section>

      <section className="checklist" aria-labelledby="before">
        <h2 id="before" className="eyebrow">
          Before you start
        </h2>
        <div className="grid3">
          <ChecklistItem
            state={status.claude.found ? "done" : "todo"}
            n={1}
            title={status.claude.found ? "Claude Code found" : "Install Claude Code"}
            text={
              status.claude.found
                ? `Version ${status.claude.version || "unknown"}. Agents use your subscription.`
                : status.claude.error || "Install it from claude.com/claude-code and log in, then check again in Settings."
            }
          />
          <ChecklistItem
            state={step2Done ? "done" : "now"}
            n={2}
            title="Link WhatsApp"
            text={step2Done ? "Linked. Syncing your chats." : "Waiting for you to scan the code."}
          />
          <ChecklistItem state="todo" n={3} title="Create your first agent" text="Give it instructions, a knowledge repo and some chats." />
        </div>
      </section>
    </div>
  );
}

function ChecklistItem({ state, n, title, text }: { state: "done" | "now" | "todo"; n: number; title: string; text: string }) {
  return (
    <div className="card check-item">
      <span className={`check-item__mark check-item__mark--${state}`}>
        {state === "done" ? <Icon name="check" size={13} strokeWidth={2.6} /> : n}
      </span>
      <div>
        <div className="strong">{title}</div>
        <div className="muted small">{text}</div>
      </div>
    </div>
  );
}

function PhoneLink() {
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      setCode(await api.linkWithPhone(phone.replace(/[^\d]/g, "")));
    } catch (e2) {
      setErr(errText(e2));
    } finally {
      setBusy(false);
    }
  };
  if (code) {
    return (
      <div className="qr__code">
        <span className="muted small">Type this code on your phone</span>
        <span className="pair-code" aria-live="polite">
          {code.length === 8 ? `${code.slice(0, 4)}-${code.slice(4)}` : code}
        </span>
        <button type="button" className="btn btn--quiet" onClick={() => setCode("")}>
          Use another number
        </button>
      </div>
    );
  }
  return (
    <form className="qr__code qr__code--form" onSubmit={submit}>
      <label htmlFor="pair-phone" className="field__label">
        Your phone number, with country code
      </label>
      <input
        id="pair-phone"
        className="input"
        type="tel"
        placeholder="+55 51 99999 0000"
        value={phone}
        onChange={(e) => setPhone(e.target.value)}
        autoFocus
      />
      {err && <span className="error-text">{err}</span>}
      <button type="submit" className="btn btn--primary" disabled={busy || phone.replace(/[^\d]/g, "").length < 8}>
        {busy ? "Asking WhatsApp…" : "Get a code"}
      </button>
    </form>
  );
}
