import { useEffect, useRef, useState } from "react";
import type { Agent, Chat } from "../../api";
import { Icon, Pill } from "../../components/ui";

/** modeOf names how the chat's agent answers, as the old chats table did. */
export function modeLabel(mode: string): { text: string; tone: "ok" | "acc" | "warn" } {
  if (mode === "auto") return { text: "On its own", tone: "ok" };
  if (mode === "never") return { text: "Only learns", tone: "acc" };
  return { text: "After I approve", tone: "warn" };
}

/** AgentPicker is the conversation header's "who answers this chat" button and menu. */
export function AgentPicker({
  info,
  isGroup,
  agents,
  onAssign,
}: {
  /** the chat's ListChats row; undefined until loaded */
  info: Chat | undefined;
  isGroup: boolean;
  agents: Agent[];
  onAssign: (agentId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const btn = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const allAgent = agents.find((a) => a.allDirect);
  const agentId = info?.agentId ?? "";
  const agent = agents.find((a) => a.id === agentId);
  const name = agent?.name || info?.agentName || "";
  const mode = modeLabel(info?.mode || agent?.mode || "");

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!menu.current?.contains(e.target as Node) && !btn.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    menu.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus() ?? menu.current?.querySelector<HTMLElement>("button")?.focus();
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const onMenuKey = (e: React.KeyboardEvent) => {
    const items = Array.from(menu.current?.querySelectorAll<HTMLElement>('[role="menuitemradio"]') ?? []);
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      items[(i + 1) % items.length]?.focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      items[(i - 1 + items.length) % items.length]?.focus();
    } else if (e.key === "Escape" || e.key === "Tab") {
      setOpen(false);
      btn.current?.focus();
    }
  };

  const pick = (id: string) => {
    setOpen(false);
    btn.current?.focus();
    if (id !== agentId) onAssign(id);
  };

  let label: React.ReactNode;
  if (agentId) {
    label = (
      <>
        <span className="ib-agentpick__name">{name || "Agent"}</span>
        <Pill tone={mode.tone}>{mode.text}</Pill>
      </>
    );
  } else if (info?.skipped) {
    label = (
      <>
        <span className="ib-agentpick__name">No agent</span>
        <Pill>Left alone</Pill>
      </>
    );
  } else {
    label = <span className="ib-agentpick__name ib-agentpick__name--none">No agent</span>;
  }

  return (
    <div className="ib-agentpick">
      <button
        ref={btn}
        type="button"
        className={agentId ? "ib-agentpick__btn ib-agentpick__btn--on" : "ib-agentpick__btn"}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Agent for this chat: ${agentId ? `${name}, ${mode.text}` : info?.skipped ? `none, left alone by ${allAgent?.name || "all direct chats"}` : "none"}`}
        title={info?.viaAll ? `${name} answers all direct chats` : undefined}
        disabled={!info}
        onClick={() => setOpen((v) => !v)}
      >
        {label}
        <Icon name="chevron" size={13} strokeWidth={2} />
      </button>
      {open && (
        <div ref={menu} className="ib-menu" role="menu" aria-label="Agent for this chat" onKeyDown={onMenuKey}>
          <button type="button" role="menuitemradio" aria-checked={!agentId} className="ib-menu__item" onClick={() => pick("")}>
            <span className="ib-menu__check">{!agentId && <Icon name="check" size={14} strokeWidth={2.4} />}</span>
            <span>
              No agent
              {!isGroup && allAgent && <span className="ib-menu__help">{allAgent.name || "The all-chats agent"} leaves this chat alone</span>}
            </span>
          </button>
          {agents.map((a) => (
            <button
              key={a.id}
              type="button"
              role="menuitemradio"
              aria-checked={a.id === agentId}
              className="ib-menu__item"
              onClick={() => pick(a.id)}
            >
              <span className="ib-menu__check">{a.id === agentId && <Icon name="check" size={14} strokeWidth={2.4} />}</span>
              <span>
                {a.name || "Untitled agent"}
                {a.allDirect && !isGroup ? " (all direct chats)" : ""}
                <span className="ib-menu__help">
                  {modeLabel(a.mode).text}
                  {a.enabled ? "" : " · paused"}
                </span>
              </span>
            </button>
          ))}
          {agents.length === 0 && <p className="ib-menu__empty muted small">No agents yet. Create one under Agents.</p>}
        </div>
      )}
    </div>
  );
}
