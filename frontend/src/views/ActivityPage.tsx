import { useState } from "react";
import { useAssistantChats, type Agent } from "../api";
import { ClaudePanel } from "../components/Assistant";
import { ActivityView } from "./ActivityView";
import { BoardView } from "./BoardView";

/** ActivityPage is the board (or the log) with the Claude chats beside it. */
export function ActivityPage({
  agents,
  onOpenAgent,
  onOpenChats,
  onNewAgent,
}: {
  agents: Agent[];
  onOpenAgent: (id: string) => void;
  onOpenChats: () => void;
  onNewAgent: () => void;
}) {
  const [tab, setTab] = useState<"board" | "log">("board");
  const [chats, reloadChats] = useAssistantChats();
  return (
    <div className="activity2">
      <div className="activity2__main">
        <header className="activity2__head">
          <h1 className="h1">Activity</h1>
          <div role="tablist" aria-label="Activity view" className="tabs">
            <button type="button" role="tab" aria-selected={tab === "board"} className={tab === "board" ? "tabs__tab tabs__tab--on" : "tabs__tab"} onClick={() => setTab("board")}>
              Board
            </button>
            <button type="button" role="tab" aria-selected={tab === "log"} className={tab === "log" ? "tabs__tab tabs__tab--on" : "tabs__tab"} onClick={() => setTab("log")}>
              Log
            </button>
          </div>
        </header>
        {tab === "board" ? <BoardView onOpenAgent={onOpenAgent} onOpenChats={onOpenChats} onOpenLog={() => setTab("log")} /> : <ActivityView agents={agents} embedded />}
      </div>
      <ClaudePanel chats={chats} reloadChats={reloadChats} onOpenAgent={onOpenAgent} onNewAgent={onNewAgent} />
    </div>
  );
}
