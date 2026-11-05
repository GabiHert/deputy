import { useEffect, useMemo, useState } from "react";
import { api, errText, on, type Agent, type Chat, type Status } from "../api";
import { initials, plural, secondsLeft, when } from "../format";
import { Confirm, Icon, Pill, SrLabel } from "../components/ui";

export function ChatsView({
  status,
  setStatus,
  agents,
}: {
  status: Status | null;
  setStatus: (s: Status) => void;
  agents: Agent[];
}) {
  if (!status) return <div className="page"><p className="muted">Starting Deputy…</p></div>;
  if (status.whatsapp.state !== "connected") return <ConnectScreen status={status} />;
  return <ChatTable status={status} setStatus={setStatus} agents={agents} />;
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
          Deputy links to your own number as a linked device, the same way WhatsApp Web does. Your agents then answer only the
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
            Deputy stays linked when you quit it. To unlink, use <strong>Log out</strong> in Chats or remove it from Linked devices
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

/* ---------- Chats table ---------- */

type Filter = "all" | "groups" | "direct" | "agent";

function ChatTable({ status, agents }: { status: Status; setStatus: (s: Status) => void; agents: Agent[] }) {
  const [chats, setChats] = useState<Chat[]>([]);
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [confirmLogout, setConfirmLogout] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    const load = () => api.listChats().then(setChats).catch(() => {});
    load();
    const offs = [on("chats", load), on("agents", load)];
    return () => offs.forEach((f) => f());
  }, []);

  const counts = useMemo(
    () => ({
      all: chats.length,
      groups: chats.filter((c) => c.isGroup).length,
      direct: chats.filter((c) => !c.isGroup).length,
      agent: chats.filter((c) => c.agentId).length,
    }),
    [chats],
  );
  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return chats.filter((c) => {
      if (filter === "groups" && !c.isGroup) return false;
      if (filter === "direct" && c.isGroup) return false;
      if (filter === "agent" && !c.agentId) return false;
      return !needle || c.name.toLowerCase().includes(needle) || c.lastText.toLowerCase().includes(needle);
    });
  }, [chats, q, filter]);

  const assign = async (jid: string, agentId: string) => {
    setErr("");
    setChats((cs) => cs.map((c) => (c.jid === jid ? { ...c, agentId, agentName: agents.find((a) => a.id === agentId)?.name ?? "" } : c)));
    try {
      await api.assignChat(jid, agentId);
    } catch (e) {
      setErr(errText(e));
      api.listChats().then(setChats).catch(() => {});
    }
  };

  const wa = status.whatsapp;
  const tabs: { id: Filter; label: string }[] = [
    { id: "all", label: `All ${counts.all}` },
    { id: "groups", label: `Groups ${counts.groups}` },
    { id: "direct", label: `Direct ${counts.direct}` },
    { id: "agent", label: `With an agent ${counts.agent}` },
  ];

  return (
    <div className="page">
      <header className="page__head page__head--split">
        <div>
          <h1>Chats</h1>
          <p className="lede">Pick which agent handles each chat. Chats without an agent are left alone.</p>
        </div>
        <div className="account">
          <span className="dot dot--ok" />
          <div className="account__text">
            <span className="strong small">
              {wa.pushName ? `${wa.pushName} · ` : ""}
              {wa.phone}
            </span>
            {wa.since > 0 && <span className="muted xsmall">Connected since {when(wa.since)}</span>}
          </div>
          <button type="button" className="btn btn--sm" onClick={() => setConfirmLogout(true)}>
            Log out
          </button>
        </div>
      </header>
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
      {err && <div className="banner banner--bad">{err}</div>}

      <div className="toolbar">
        <div className="search">
          <Icon name="search" size={16} strokeWidth={2} />
          <SrLabel htmlFor="chat-search">Search chats</SrLabel>
          <input id="chat-search" type="search" placeholder="Search chats" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div role="tablist" aria-label="Filter chats" className="tabs">
          {tabs.map((t) => (
            <button
              key={t.id}
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
      </div>

      <div className="table card">
        <div className="table__inner">
          <div className="table__row table__row--head">
            <span>Chat</span>
            <span>Last message</span>
            <span>Agent</span>
            <span>Answers</span>
          </div>
          {shown.length === 0 && (
            <div className="table__empty muted">
              {chats.length === 0 ? "No chats yet. They appear here as soon as someone writes in them." : "No chats match."}
            </div>
          )}
          {shown.map((c) => (
            <div key={c.jid} className="table__row">
              <div className="chatcell">
                <span className={`avatar avatar--${avatarTone(c.jid)}`}>{initials(c.name)}</span>
                <div className="chatcell__text">
                  <span className="strong ellipsis">{c.name}</span>
                  <span className="muted xsmall">{c.isGroup ? `Group${c.members ? ` · ${plural(c.members, "member")}` : ""}` : "Direct"}</span>
                </div>
              </div>
              <div className="chatcell__text">
                <span className="fg2 ellipsis">{c.lastText || "—"}</span>
                <span className="muted xsmall tabular">{when(c.lastAt)}</span>
              </div>
              <div>
                <SrLabel htmlFor={`agent-${c.jid}`}>{`Agent for ${c.name}`}</SrLabel>
                <select
                  id={`agent-${c.jid}`}
                  className={c.agentId ? "select select--on" : "select"}
                  value={c.agentId}
                  onChange={(e) => assign(c.jid, e.target.value)}
                >
                  <option value="">No agent</option>
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name || "Untitled agent"}
                    </option>
                  ))}
                </select>
              </div>
              <div>{modePill(c, agents)}</div>
            </div>
          ))}
        </div>
      </div>
      <p className="muted small">Showing chats with messages since you linked Deputy. Older chats appear as soon as someone writes in them.</p>
    </div>
  );
}

function modePill(c: Chat, agents: Agent[]) {
  if (!c.agentId) return <Pill>Ignored</Pill>;
  const mode = c.mode || agents.find((a) => a.id === c.agentId)?.mode;
  if (mode === "auto") return <Pill tone="ok">On its own</Pill>;
  if (mode === "never") return <Pill tone="acc">Only learns</Pill>;
  return <Pill tone="warn">After I approve</Pill>;
}

function avatarTone(jid: string): number {
  let h = 0;
  for (let i = 0; i < jid.length; i++) h = (h * 31 + jid.charCodeAt(i)) | 0;
  return Math.abs(h) % 5;
}
