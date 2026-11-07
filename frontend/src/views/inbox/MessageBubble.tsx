import { memo, useEffect, useRef, useState } from "react";
import { mentionParts, useMembers } from "./mentions";
import { errText, inbox, type ChatMessage, type MessageStatus } from "../../api";
import { bytes, clock, duration } from "../../format";
import { Confirm, Icon } from "../../components/ui";
import { canEdit, kindLabel, tone, viewOnceLabel } from "./helpers";

export const QUICK_REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🙏"];

export type BubbleActions = {
  onReply: (m: ChatMessage) => void;
  onEdit: (m: ChatMessage) => void;
  onJump: (id: string) => void;
  onError: (text: string) => void;
  /** ask the chat's agent about this message; absent when the chat has no agent */
  onAsk?: (m: ChatMessage) => void;
  askLabel?: string;
};

export const MessageBubble = memo(function MessageBubble({
  m,
  isGroup,
  continued,
  highlight,
  actions,
}: {
  m: ChatMessage;
  isGroup: boolean;
  continued: boolean;
  highlight: boolean;
  actions: BubbleActions;
}) {
  const members = useMembers(isGroup && m.mentions?.length ? m.chatJid : "");
  const [reacting, setReacting] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [copied, setCopied] = useState(false);
  const mine = m.fromMe;
  const mineReaction = m.reactions?.find((r) => r.fromMe)?.emoji ?? "";

  const react = async (emoji: string) => {
    setReacting(false);
    try {
      await inbox.react(m.chatJid, m.id, emoji);
    } catch (e) {
      actions.onError(errText(e));
    }
  };
  const copy = async () => {
    const text = m.text || m.transcript;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch (e) {
      actions.onError(errText(e));
    }
  };
  const remove = async () => {
    setConfirmDelete(false);
    try {
      await inbox.deleteForEveryone(m.chatJid, m.id);
    } catch (e) {
      actions.onError(errText(e));
    }
  };

  const who = mine ? (m.fromAgent ? m.agentName || "Agent" : "You") : m.senderName || "Someone";
  const cls = [
    "ib-msg",
    mine ? "ib-msg--me" : "",
    continued ? "ib-msg--cont" : "",
    highlight ? "ib-msg--hl" : "",
    m.reactions?.length ? "ib-msg--reacted" : "",
  ].join(" ");
  const bubbleCls = ["ib-b", mine ? "ib-b--me" : "", m.fromAgent ? "ib-b--agent" : "", m.revoked ? "ib-b--gone" : ""].join(" ");
  // Older document messages kept the file name as their text; show it in the card, not twice.
  const docAsText = m.kind === "document" && !!m.text && (!m.media?.fileName || m.text === m.media.fileName);
  // A view-once caption shows with the media, as on the phone.
  const showText = !m.revoked && m.text && !docAsText && !m.viewOnce;

  return (
    <div className={cls} id={`msg-${m.id}`} data-id={m.id}>
      <div className="ib-msg__line">
        <div className={bubbleCls} aria-label={`${who}, ${clock(m.at)}`} role="group">
          {isGroup && !mine && !continued && <span className={`ib-b__by ib-s${tone(m.senderJid || m.senderName, 6)}`}>{m.senderName || "Someone"}</span>}
          {m.fromAgent && <span className="ib-b__by ib-b__by--agent">{m.agentName || "Agent"}</span>}
          {m.revoked ? (
            <span className="ib-b__gone">{mine ? "You deleted this message" : "This message was deleted"}</span>
          ) : (
            <>
              {m.quoted && (
                <button
                  type="button"
                  className="ib-quote"
                  disabled={!m.quoted.found}
                  onClick={() => m.quoted && actions.onJump(m.quoted.id)}
                  title={m.quoted.found ? "Show the message" : "That message is not in Deputy"}
                >
                  <span className={`ib-quote__by ib-s${tone(m.quoted.senderJid || m.quoted.senderName, 6)}`}>{m.quoted.senderName}</span>
                  <span className="ib-quote__text">{m.quoted.snippet || kindLabel(m.quoted.kind)}</span>
                </button>
              )}
              {m.viewOnce && (
                <span className="ib-viewonce__badge">
                  <Icon name="viewonce" size={13} />
                  View once
                </span>
              )}
              {m.media && (m.viewOnce ? <ViewOnceView m={m} /> : <MediaView m={m} onError={actions.onError} />)}
              {showText && (
                <span className="ib-b__text">
                  {mentionParts(m.text, m.mentions, members).map((part, i) =>
                    part.mention ? (
                      <span key={i} className="ib-b__mention">
                        {part.text}
                      </span>
                    ) : (
                      part.text
                    ),
                  )}
                </span>
              )}
              {m.transcript && (m.kind === "voice" || m.kind === "audio") && <span className="ib-b__transcript">“{m.transcript}”</span>}
            </>
          )}
          <span className="ib-b__time">
            {m.edited && !m.revoked && <span className="ib-b__edited">Edited</span>}
            <span className="tabular">{clock(m.at)}</span>
            {mine && !m.revoked && <Ticks status={m.status} />}
          </span>
        </div>

        {!m.revoked && (
          <div className="ib-acts" role="toolbar" aria-label="Message actions">
            <button type="button" className="ib-act" aria-label="Reply" title="Reply" onClick={() => actions.onReply(m)}>
              <Icon name="reply" size={15} />
            </button>
            {actions.onAsk && (
              <button
                type="button"
                className="ib-act"
                aria-label={actions.askLabel ?? "Ask the agent about this"}
                title={actions.askLabel ?? "Ask the agent about this"}
                onClick={() => actions.onAsk?.(m)}
              >
                <Icon name="chats" size={15} />
              </button>
            )}
            <button
              type="button"
              className="ib-act"
              aria-label="React"
              title="React"
              aria-expanded={reacting}
              onClick={() => setReacting((v) => !v)}
            >
              <Icon name="smile" size={15} />
            </button>
            {(m.text || m.transcript) && (
              <button type="button" className="ib-act" aria-label={copied ? "Copied" : "Copy"} title={copied ? "Copied" : "Copy"} onClick={copy}>
                <Icon name={copied ? "check" : "copy"} size={15} />
              </button>
            )}
            {canEdit(m) && (
              <button type="button" className="ib-act" aria-label="Edit" title="Edit" onClick={() => actions.onEdit(m)}>
                <Icon name="pencil" size={15} />
              </button>
            )}
            {mine && m.status !== "pending" && m.status !== "failed" && (
              <button type="button" className="ib-act ib-act--bad" aria-label="Delete for everyone" title="Delete for everyone" onClick={() => setConfirmDelete(true)}>
                <Icon name="trash" size={15} />
              </button>
            )}
          </div>
        )}
      </div>

      {reacting && (
        <div className="ib-reactbar" role="group" aria-label="Pick a reaction" onKeyDown={(e) => e.key === "Escape" && setReacting(false)}>
          {QUICK_REACTIONS.map((e, i) => (
            <button
              key={e}
              type="button"
              className={mineReaction === e ? "ib-reactbar__e ib-reactbar__e--on" : "ib-reactbar__e"}
              aria-label={`React with ${e}`}
              aria-pressed={mineReaction === e}
              autoFocus={i === 0}
              onClick={() => react(mineReaction === e ? "" : e)}
            >
              {e}
            </button>
          ))}
          {mineReaction && (
            <button type="button" className="btn btn--quiet btn--sm" onClick={() => react("")}>
              Remove
            </button>
          )}
        </div>
      )}

      {m.reactions?.length > 0 && (
        <div className="ib-reactions">
          {m.reactions.map((r) => (
            <button
              key={r.emoji}
              type="button"
              className={r.fromMe ? "ib-reaction ib-reaction--me" : "ib-reaction"}
              title={r.senders?.join(", ")}
              aria-label={`${r.emoji} ${r.count}: ${r.senders?.join(", ")}${r.fromMe ? ". Click to remove yours" : ""}`}
              onClick={() => react(r.fromMe ? "" : r.emoji)}
            >
              {r.emoji}
              {r.count > 1 && <span className="tabular">{r.count}</span>}
            </button>
          ))}
        </div>
      )}

      {confirmDelete && (
        <div className="ib-confirm">
          <Confirm danger text="Delete this message for everyone?" yes="Delete" onNo={() => setConfirmDelete(false)} onYes={remove} />
        </div>
      )}
    </div>
  );
});

function Ticks({ status }: { status: MessageStatus | string }) {
  if (status === "pending") return <span className="ib-tick" aria-label="Sending" title="Sending">🕓</span>;
  if (status === "failed")
    return (
      <span className="ib-tick ib-tick--bad" aria-label="Not sent" title="Not sent">
        !
      </span>
    );
  const two = status === "delivered" || status === "read" || status === "played";
  const read = status === "read" || status === "played";
  const label = read ? "Read" : two ? "Delivered" : "Sent";
  return (
    <svg className={read ? "ib-tick ib-tick--read" : "ib-tick"} width="16" height="10" viewBox="0 0 26 14" fill="none" stroke="currentColor" strokeWidth="2" role="img" aria-label={label}>
      <title>{label}</title>
      {two ? <path d="M1 7l4 4 8-9M10 11l1 1 8-10" /> : <path d="M5 7l4 4 8-9" />}
    </svg>
  );
}

/** useVisible fires once when the element scrolls into view. */
function useVisible<T extends Element>(): [React.RefObject<T>, boolean] {
  const ref = useRef<T>(null);
  const [seen, setSeen] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el || seen) return;
    if (typeof IntersectionObserver === "undefined") {
      setSeen(true);
      return;
    }
    const io = new IntersectionObserver((es) => {
      if (es.some((e) => e.isIntersecting)) {
        setSeen(true);
        io.disconnect();
      }
    });
    io.observe(el);
    return () => io.disconnect();
  }, [seen]);
  return [ref, seen];
}

function MediaView({ m, onError }: { m: ChatMessage; onError: (t: string) => void }) {
  const media = m.media!;
  const [url, setUrl] = useState(media.url);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState("");
  useEffect(() => setUrl(media.url), [media.url]);

  const fetchUrl = async (): Promise<string> => {
    if (url) return url;
    if (!media.downloadable) {
      setFailed(OLD_FILE);
      return "";
    }
    setLoading(true);
    try {
      const u = await inbox.getMedia(m.chatJid, m.id);
      setUrl(u);
      return u;
    } catch (e) {
      setFailed(errText(e));
      return "";
    } finally {
      setLoading(false);
    }
  };
  const open = async () => {
    if (!media.downloadable && !url) {
      setFailed(OLD_FILE);
      return;
    }
    try {
      await inbox.openMedia(m.chatJid, m.id);
    } catch (e) {
      onError(errText(e));
    }
  };

  if (m.kind === "image" || m.kind === "sticker") return <ImageView m={m} url={url} fetchUrl={fetchUrl} open={open} failed={failed} />;

  if (m.kind === "video") {
    return (
      <button type="button" className="ib-media ib-media--video" onClick={open} aria-label={`Play video${media.seconds ? `, ${duration(media.seconds)}` : ""}`}>
        {media.thumbUrl ? <img src={media.thumbUrl} alt="" /> : <span className="ib-media__ph" />}
        <span className="ib-media__play">
          <Icon name="play" size={18} />
        </span>
        {media.seconds > 0 && <span className="ib-media__dur tabular">{duration(media.seconds)}</span>}
      </button>
    );
  }

  if (m.kind === "voice" || m.kind === "audio") {
    return (
      <div className="ib-voice">
        {url ? (
          <audio className="ib-voice__audio" controls preload="metadata" src={url} aria-label={m.kind === "voice" ? "Voice note" : "Audio"} />
        ) : (
          <>
            <button
              type="button"
              className="ib-voice__play"
              disabled={loading || (!media.downloadable && !url)}
              aria-label={loading ? "Loading" : "Play"}
              onClick={async () => {
                const u = await fetchUrl();
                if (u) {
                  // Let React render the <audio>, then start it.
                  setTimeout(() => document.querySelector<HTMLAudioElement>(`#msg-${CSS.escape(m.id)} audio`)?.play().catch(() => {}), 50);
                }
              }}
            >
              {loading ? <span className="spinner spinner--sm" aria-hidden="true" /> : <Icon name="play" size={14} />}
            </button>
            <span className="ib-voice__wave" aria-hidden="true">
              {WAVE.map((h, i) => (
                <i key={i} style={{ height: h }} />
              ))}
            </span>
            <span className="ib-voice__dur tabular">{duration(media.seconds)}</span>
          </>
        )}
        {failed && <span className="error-text xsmall">{failed}</span>}
      </div>
    );
  }

  // document and anything else
  return (
    <button
      type="button"
      className={media.downloadable || url ? "ib-doc" : "ib-doc ib-doc--gone"}
      onClick={open}
      title={media.downloadable || url ? "Open" : OLD_FILE}
    >
      {media.thumbUrl ? <img className="ib-doc__thumb" src={media.thumbUrl} alt="" /> : <Icon name="file" size={26} />}
      <span className="ib-doc__text">
        <span className="ib-doc__name">{media.fileName || (m.kind === "document" && m.text) || "Document"}</span>
        <span className="ib-doc__meta">
          {media.downloadable || url
            ? [docType(media.fileName || m.text, media.mime), media.size ? bytes(media.size) : ""].filter(Boolean).join(" · ")
            : failed || "Only on your phone"}
        </span>
      </span>
    </button>
  );
}

/**
 * ViewOnceView keeps a view-once photo, video or voice note hidden until the owner asks, then
 * shows it once in the bubble. Without the media (WhatsApp keeps it on the phone) it says so.
 */
function ViewOnceView({ m }: { m: ChatMessage }) {
  const media = m.media!;
  const [state, setState] = useState<"hidden" | "loading" | "shown" | "done">("hidden");
  const [url, setUrl] = useState("");
  const [failed, setFailed] = useState("");
  const label = viewOnceLabel(m.kind);

  if (!media.downloadable) {
    return (
      <div className="ib-viewonce ib-viewonce--gone" role="note">
        <Icon name="viewonce" size={22} />
        <span className="ib-viewonce__text">
          <span className="ib-viewonce__name">View once {label}</span>
          <span className="ib-viewonce__meta">Open it on your phone</span>
        </span>
      </div>
    );
  }
  const reveal = async () => {
    setState("loading");
    setFailed("");
    try {
      setUrl(media.url || (await inbox.getMedia(m.chatJid, m.id)));
      setState("shown");
    } catch (e) {
      setFailed(errText(e));
      setState("hidden");
    }
  };
  if (state === "shown" && url) {
    return (
      <div className="ib-viewonce__open">
        {m.kind === "video" ? (
          <video src={url} controls autoPlay className="ib-viewonce__media" />
        ) : m.kind === "voice" || m.kind === "audio" ? (
          <audio src={url} controls autoPlay aria-label="View once voice message" />
        ) : (
          <img src={url} alt="View once photo" className="ib-viewonce__media" />
        )}
        {m.text && <span className="ib-b__text">{m.text}</span>}
        <button type="button" className="btn btn--quiet btn--sm" onClick={() => setState("done")}>
          Close
        </button>
      </div>
    );
  }
  if (state === "done") {
    return (
      <div className="ib-viewonce ib-viewonce--gone" role="note">
        <Icon name="viewonce" size={22} />
        <span className="ib-viewonce__text">
          <span className="ib-viewonce__name">Opened</span>
          <span className="ib-viewonce__meta">View once {label}</span>
        </span>
      </div>
    );
  }
  return (
    <button type="button" className="ib-viewonce" onClick={reveal} disabled={state === "loading"} aria-label={`Tap to view the view once ${label}`}>
      {state === "loading" ? <span className="spinner spinner--sm" aria-hidden="true" /> : <Icon name="viewonce" size={22} />}
      <span className="ib-viewonce__text">
        <span className="ib-viewonce__name">Tap to view</span>
        <span className="ib-viewonce__meta">{failed || `View once ${label}${media.seconds ? ` · ${duration(media.seconds)}` : ""}`}</span>
      </span>
    </button>
  );
}

/** "CSV", "PDF"… from the file name, else a short form of the mime type. */
function docType(name: string, mime: string): string {
  const ext = /\.([a-z0-9]{1,5})$/i.exec(name || "")?.[1];
  if (ext) return ext.toUpperCase();
  const sub = mime.split("/")[1] ?? "";
  return sub.length <= 6 ? sub.toUpperCase() : "";
}

const OLD_FILE = "Arrived before Deputy kept files. Open it on your phone.";

const WAVE = Array.from({ length: 30 }, (_, k) => 4 + Math.round(14 * Math.abs(Math.sin(k * 1.7) * Math.cos(k * 0.6))));

function ImageView({
  m,
  url,
  fetchUrl,
  open,
  failed,
}: {
  m: ChatMessage;
  url: string;
  fetchUrl: () => Promise<string>;
  open: () => void;
  failed: string;
}) {
  const [ref, seen] = useVisible<HTMLButtonElement>();
  useEffect(() => {
    if (seen && !url && m.media?.downloadable) void fetchUrl();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seen]);
  const src = url || m.media?.thumbUrl || "";
  const sticker = m.kind === "sticker";
  return (
    <button
      ref={ref}
      type="button"
      className={sticker ? "ib-media ib-media--sticker" : "ib-media ib-media--image"}
      onClick={open}
      aria-label={sticker ? "Sticker" : m.text ? `Photo: ${m.text}` : "Photo"}
      title="Open"
    >
      {src ? <img src={src} alt="" className={url ? "" : "ib-media__blur"} /> : <span className="ib-media__ph">{failed ? "Photo not available" : ""}</span>}
    </button>
  );
}
