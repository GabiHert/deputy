import { useEffect, useState } from "react";
import { api2, useAssistantChats, useChatMessages } from "../api";
import { Conversation } from "../components/Assistant";

/** NewAgentView creates an agent by chatting with Claude, which reads the chats and writes the agent. */
const DRAFT = "deputy.newAgentDraft";

/** The draft is the new-agent chat in progress, kept so leaving the page doesn't lose it. */
const draft = {
  get: () => localStorage.getItem(DRAFT) ?? "",
  set: (id: string) => (id ? localStorage.setItem(DRAFT, id) : localStorage.removeItem(DRAFT)),
};

export function NewAgentView({ onOpenAgent, onCreated }: { onOpenAgent: (id: string) => void; onCreated: () => void }) {
  const [chatId, setChatId] = useState(draft.get);

  useEffect(() => {
    const id = draft.get();
    if (!id) return;
    api2
      .listChats()
      .then((cs) => {
        if (!cs.some((c) => c.id === id)) {
          draft.set("");
          setChatId("");
        }
      })
      .catch(() => {});
  }, []);
  const [chats, reloadChats] = useAssistantChats();
  const [msgs] = useChatMessages(chatId);
  const chat = chats.find((c) => c.id === chatId) ?? null;

  const cards = msgs.filter((m) => m.card?.kind === "agent");
  const step = cards.some((m) => m.card?.status === "done") ? 2 : cards.some((m) => m.card?.status === "open") ? 1 : 0;
  const pill = (n: number, label: string) => (
    <span className={step === n ? "step step--on" : step > n ? "step step--done" : "step"} aria-current={step === n ? "step" : undefined}>
      {n + 1} {label}
    </span>
  );

  return (
    <div className="newagent">
      <header className="newagent__head">
        <div>
          <h1 className="h1">New agent</h1>
          <div className="muted small">A chat with Claude. It reads your chats and writes the agent for you.</div>
        </div>
        <div className="row row--tight" aria-label="Progress">
          {chatId && msgs.length > 0 && (
            <button
              type="button" className="btn btn--sm"
              onClick={() => {
                draft.set("");
                setChatId("");
              }}
            >
              Start over
            </button>
          )}
          {pill(0, "Describe")}
          {pill(1, "Review")}
          {pill(2, "Running")}
        </div>
      </header>
      <Conversation
        key={chatId || "new"}
        chat={chat}
        wide
        onOpenAgent={onOpenAgent}
        onAgentCreated={() => {
          draft.set("");
          onCreated();
        }}
        ensureChat={async () => {
          const c = await api2.newChat("new-agent", "");
          reloadChats();
          draft.set(c.id);
          setChatId(c.id);
          return c.id;
        }}
        placeholder={step === 1 ? "Change something, like “don't talk about the farm”" : "Say what the agent should do, and in which chat"}
        intro={
          <p className="say">
            What should the new agent do, and where? Say it the way you'd tell a friend. I'll read the chat to learn the people, the topics and how
            you write, then show you the agent before anything is created.
          </p>
        }
      />
    </div>
  );
}
