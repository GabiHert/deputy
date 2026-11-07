import { useEffect, useRef, useState } from "react";
import { inbox, inboxEvents, on, useAgents, useStatus, type Agent, type Status, type UnreadEvent } from "./api";
import { Icon, Logo } from "./components/ui";
import { ChatsView } from "./views/ChatsView";
import { AgentView } from "./views/AgentView";
import { ActivityPage } from "./views/ActivityPage";
import { NewAgentView } from "./views/NewAgentView";
import "./v2.css";
import { SettingsView } from "./views/SettingsView";

export type View =
  | { name: "chats"; open?: { jid: string; n: number } }
  | { name: "agent"; id: string } // id "" = new agent
  | { name: "activity" }
  | { name: "new-agent"; n: number }
  | { name: "settings" };

export function App() {
  const [status, setStatus] = useStatus();
  const [agents, reloadAgents] = useAgents();
  const [view, setView] = useState<View>({ name: "activity" });
  const newAgent = () => setView({ name: "new-agent", n: Date.now() });

  // Start on Chats (where the QR code is) until WhatsApp is linked; Activity afterwards.
  const routed = useRef(false);
  useEffect(() => {
    if (routed.current || !status) return;
    routed.current = true;
    const linked = status.whatsapp.state === "connected" || status.whatsapp.state === "disconnected";
    if (!linked) setView((v) => (v.name === "activity" ? { name: "chats" } : v));
  }, [status]);

  useEffect(() => {
    const offs = [
      on(inboxEvents.navigate, (to: string) => {
        routed.current = true;
        if (to === "inbox" || to === "chats") setView((v) => (v.name === "chats" ? v : { name: "chats" }));
        else if (to === "activity" || to === "settings") setView({ name: to } as View);
        else if (to === "new-agent") setView({ name: "new-agent", n: Date.now() });
        else if (to.startsWith("agent:")) setView({ name: "agent", id: to.slice(6) });
      }),
      // A notification was clicked: show that chat.
      on(inboxEvents.openChat, (jid: string) => {
        routed.current = true;
        if (jid) setView({ name: "chats", open: { jid, n: Date.now() } });
      }),
    ];
    return () => offs.forEach((f) => f());
  }, []);

  // Unread WhatsApp messages, for the sidebar badge.
  const [unread, setUnread] = useState(0);
  useEffect(() => {
    inbox
      .unreadTotal()
      .then((n) => setUnread(n || 0))
      .catch(() => {});
    return on(inboxEvents.unread, (ev: UnreadEvent) => setUnread(ev.total || 0));
  }, []);
  const openAgent = (id: string) => setView({ name: "agent", id });

  return (
    <div className="shell">
      <Sidebar view={view} setView={setView} agents={agents} status={status} unread={unread} onNewAgent={newAgent} />
      <main
        className={view.name === "activity" || view.name === "new-agent" || view.name === "chats" ? "content content--full" : "content"}
        id="main"
      >
        {view.name === "chats" && (
          <ChatsView status={status} agents={agents} open={view.open ?? null} onOpenAgent={openAgent} onNewAgent={newAgent} />
        )}
        {view.name === "agent" && (
          <AgentView
            key={view.id || "new"}
            id={view.id}
            agents={agents}
            onSaved={(id) => setView({ name: "agent", id })}
            onDeleted={() => setView({ name: "chats" })}
          />
        )}
        {view.name === "activity" && (
          <ActivityPage agents={agents} onOpenAgent={openAgent} onOpenChats={() => setView({ name: "chats" })} onNewAgent={newAgent} />
        )}
        {view.name === "new-agent" && <NewAgentView key={view.n} onOpenAgent={openAgent} onCreated={reloadAgents} />}
        {view.name === "settings" && <SettingsView status={status} setStatus={setStatus} />}
      </main>
    </div>
  );
}

function Sidebar({
  view,
  setView,
  agents,
  status,
  unread,
  onNewAgent,
}: {
  view: View;
  setView: (v: View) => void;
  agents: Agent[];
  status: Status | null;
  unread: number;
  onNewAgent: () => void;
}) {
  const wa = status?.whatsapp;
  const connected = wa?.state === "connected";
  const footer = connected
    ? wa?.phone || "Connected"
    : wa?.state === "disconnected"
      ? "Reconnecting to WhatsApp…"
      : wa?.state === "pairing"
        ? "Linking WhatsApp…"
        : "WhatsApp not connected";
  const navItem = (name: View["name"], icon: string, label: string, onClick: () => void, extra?: React.ReactNode) => {
    const current = view.name === name;
    return (
      <button
        type="button"
        className={current ? "nav nav--on" : "nav"}
        aria-current={current ? "page" : undefined}
        onClick={onClick}
      >
        <Icon name={icon} />
        <span className="nav__label">{label}</span>
        {extra}
      </button>
    );
  };
  return (
    <nav className="sidebar" aria-label="Main">
      <div className="sidebar__drag" />
      <div className="brand">
        <span className="brand__mark">
          <Logo size={26} />
        </span>
        <span className="brand__name">Deputy</span>
      </div>
      {navItem(
        "activity",
        "activity",
        "Activity",
        () => setView({ name: "activity" }),
        status && status.waiting > 0 ? (
          <span className="count" aria-label={`${status.waiting} waiting`}>
            {status.waiting}
          </span>
        ) : null,
      )}
      {navItem(
        "chats",
        "chats",
        "WhatsApp chats",
        () => setView({ name: "chats" }),
        unread > 0 ? (
          <span className="count count--unread" aria-label={`${unread} unread`}>
            {unread > 999 ? "999+" : unread}
          </span>
        ) : null,
      )}
      <div className="nav-heading">Agents</div>
      <div className="subnav subnav--flat">
        {agents.map((a) => {
          const current = view.name === "agent" && view.id === a.id;
          return (
            <button
              key={a.id}
              type="button"
              className={current ? "subnav__item subnav__item--on" : "subnav__item"}
              aria-current={current ? "page" : undefined}
              onClick={() => setView({ name: "agent", id: a.id })}
            >
              <span className="subnav__name">{a.name || "Untitled agent"}</span>
              <span className={a.enabled ? "dot dot--ok" : "dot dot--idle"} aria-label={a.enabled ? "On" : "Paused"} />
            </button>
          );
        })}
        <button
          type="button"
          className={view.name === "new-agent" ? "subnav__item subnav__item--on subnav__item--new" : "subnav__item subnav__item--new"}
          aria-current={view.name === "new-agent" ? "page" : undefined}
          onClick={onNewAgent}
        >
          <Icon name="plus" size={14} strokeWidth={2} />
          New agent
        </button>
      </div>
      {navItem("settings", "settings", "Settings", () => setView({ name: "settings" }))}
      <div className="sidebar__footer">
        <span className={connected ? "dot dot--ok" : "dot dot--warn"} />
        {footer}
      </div>
    </nav>
  );
}
