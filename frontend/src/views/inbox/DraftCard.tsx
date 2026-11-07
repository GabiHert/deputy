import { useState } from "react";
import { api, errText, type Run } from "../../api";
import { SrLabel } from "../../components/ui";

/** DraftCard is an agent's reply waiting for the owner, shown at the bottom of its chat. */
export function DraftCard({ run, onDone }: { run: Run; onDone: (id: string) => void }) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(run.reply);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr("");
    try {
      await fn();
      onDone(run.id);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  const images = run.images?.length ?? 0;
  return (
    <div className="ib-draft" role="group" aria-label={`${run.agentName} wants to reply`}>
      <span className="ib-draft__by">{run.agentName || "Your agent"} wants to reply · waiting for you</span>
      {editing ? (
        <>
          <SrLabel htmlFor={`draft-${run.id}`}>Reply to send</SrLabel>
          <textarea id={`draft-${run.id}`} className="input input--sm ib-draft__edit" rows={3} value={text} onChange={(e) => setText(e.target.value)} autoFocus />
        </>
      ) : (
        <span className="ib-draft__text">{run.reply}</span>
      )}
      {(images > 0 || run.voice) && (
        <span className="muted xsmall">
          {[images ? `${images} photo${images === 1 ? "" : "s"}` : "", run.voice ? "sent as a voice note" : ""].filter(Boolean).join(" · ")}
        </span>
      )}
      {err && <span className="error-text">{err}</span>}
      <div className="ib-draft__acts">
        {editing ? (
          <button type="button" className="btn btn--sm" disabled={busy} onClick={() => (setEditing(false), setText(run.reply))}>
            Undo edit
          </button>
        ) : (
          <button type="button" className="btn btn--sm" disabled={busy} onClick={() => setEditing(true)}>
            Edit
          </button>
        )}
        <button type="button" className="btn btn--sm" disabled={busy} onClick={() => act(() => api.discardDraft(run.id))}>
          Discard
        </button>
        <button type="button" className="btn btn--primary btn--sm" disabled={busy || !text.trim()} onClick={() => act(() => api.sendDraft(run.id, text))}>
          {busy ? "Sending…" : "Send"}
        </button>
      </div>
    </div>
  );
}
