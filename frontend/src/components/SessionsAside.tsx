import { useEffect, useState } from "react";
import { useAssistantChats } from "../api";
import { ClaudePanel } from "./Assistant";
import { Icon } from "./ui";

/** SessionsAside is the Claude sessions panel on the right of a page, or its collapsed rail. */
export function SessionsAside({
  storageKey,
  defaultOpen = true,
  onOpenAgent,
  onNewAgent,
  focus,
}: {
  /** open the panel on this Claude chat (n changes on every request) */
  focus?: { id: string; n: number } | null;
  /** localStorage key that remembers whether it is open */
  storageKey: string;
  defaultOpen?: boolean;
  onOpenAgent: (id: string) => void;
  onNewAgent: () => void;
}) {
  const [chats, reloadChats] = useAssistantChats();
  const [open, setOpenState] = useState(() => {
    const v = localStorage.getItem(storageKey);
    return v === null ? defaultOpen : v !== "hidden";
  });
  const setOpen = (v: boolean) => {
    localStorage.setItem(storageKey, v ? "shown" : "hidden");
    setOpenState(v);
  };
  useEffect(() => {
    if (focus?.id) setOpen(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focus?.n]);
  if (open) {
    return (
      <ClaudePanel chats={chats} reloadChats={reloadChats} onOpenAgent={onOpenAgent} onNewAgent={onNewAgent} onHide={() => setOpen(false)} focus={focus} />
    );
  }
  return (
    <aside className="claude-rail" aria-label="Sessions, collapsed">
      <button type="button" className="claude-rail__btn" aria-label="Show sessions" title="Show sessions" onClick={() => setOpen(true)}>
        <span className="claude-rail__chev">
          <Icon name="chevron" size={16} strokeWidth={2} />
        </span>
      </button>
      <button type="button" className="claude-rail__label" onClick={() => setOpen(true)} tabIndex={-1} aria-hidden="true">
        <Icon name="chats" size={16} />
        <span>Sessions</span>
      </button>
    </aside>
  );
}
