import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { encodeMentions, matchMembers, mentionAt, useMembers } from "./mentions";
import { completeEmoji, emojiAt, matchEmoji, type EmojiHit } from "./emoji";
import { Avatar } from "./Avatar";

const initialsOf = (n: string) =>
  n
    .replace(/[^\p{L}\p{N} ]/gu, "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join("") || "?";
import { errText, inbox, type ChatMessage } from "../../api";
import { blobToBase64 } from "../../lib/files";
import { duration } from "../../format";
import { Icon, SrLabel } from "../../components/ui";
import { baseName, fileExt, kindLabel, messageSnippet, pickRecorderMime } from "./helpers";

export type TrayItem =
  | { id: string; source: "path"; path: string; name: string }
  | { id: string; source: "blob"; file: File; name: string; preview: string };

let trayN = 0;
export function trayFromFiles(files: File[]): TrayItem[] {
  return files.map((f) => {
    const name = f.name || `pasted-${Date.now()}${fileExt(f.type) || ".bin"}`;
    return { id: `t${++trayN}`, source: "blob", file: f, name, preview: f.type.startsWith("image/") ? URL.createObjectURL(f) : "" };
  });
}
export function trayFromPaths(paths: string[]): TrayItem[] {
  return paths.map((p) => ({ id: `t${++trayN}`, source: "path", path: p, name: baseName(p) }));
}
export function releaseTray(items: TrayItem[]) {
  for (const t of items) if (t.source === "blob" && t.preview) URL.revokeObjectURL(t.preview);
}

const EMOJI =
  "😀 😃 😄 😁 😆 😅 😂 🤣 😊 😇 🙂 😉 😍 🥰 😘 😋 😜 🤪 🤗 🤔 🤨 😐 😏 🙄 😬 😌 😴 😷 🤒 🥳 😎 🤓 😕 😟 😮 😲 😳 🥺 😢 😭 😱 😤 😡 🤯 👍 👎 👏 🙌 🙏 🤝 💪 👌 ✌️ 🤞 👋 🫶 ❤️ 🧡 💛 💚 💙 💜 🖤 💔 💯 🔥 ✨ 🎉 🎂 🎁 🌹 ☀️ 🌧️ ⚽ 🍕 🍺 🍷 ☕ 🚗 ✈️ 🏠 📷 🎵 ✅ ❌ ❓ ❗ 👀 🤷 🙈"
    .split(" ")
    .filter(Boolean);

/** unsent text per chat, kept while the window is open */
const unsent = new Map<string, string>();

export function Composer({
  chat,
  chatName,
  replyTo,
  onCancelReply,
  editing,
  onCancelEdit,
  tray,
  setTray,
  onSent,
  onError,
  focusKey,
}: {
  chat: string;
  chatName: string;
  replyTo: ChatMessage | null;
  onCancelReply: () => void;
  editing: ChatMessage | null;
  onCancelEdit: () => void;
  tray: TrayItem[];
  setTray: (fn: (t: TrayItem[]) => TrayItem[]) => void;
  onSent: (m: ChatMessage) => void;
  onError: (t: string) => void;
  /** changes when the composer should take focus (chat opened, reply picked) */
  focusKey: string;
}) {
  const [text, setText] = useState(() => unsent.get(chat) ?? "");
  const [caption, setCaption] = useState("");
  const [sending, setSending] = useState(false);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [recording, setRecording] = useState(false);
  const ta = useRef<HTMLTextAreaElement>(null);
  const members = useMembers(chat);
  const picked = useRef<{ name: string; jid: string }[]>([]);
  const [pop, setPop] = useState<{ start: number; query: string } | null>(null);
  const [popIndex, setPopIndex] = useState(0);
  const options = pop && members.length ? matchMembers(members, pop.query) : [];
  const [epop, setEpop] = useState<{ start: number; query: string } | null>(null);
  const eoptions = !options.length && epop ? matchEmoji(epop.query) : [];
  const count = options.length || eoptions.length;
  const findMention = (v: string, caret: number | null) => {
    const hit = members.length && caret != null ? mentionAt(v, caret) : null;
    setPop(hit);
    setEpop(caret != null ? emojiAt(v, caret) : null);
    setPopIndex(0);
  };
  const putCaret = (at: number) =>
    requestAnimationFrame(() => {
      ta.current?.focus();
      ta.current?.setSelectionRange(at, at);
    });
  const chooseEmoji = (h: EmojiHit) => {
    if (!epop) return;
    const caret = ta.current?.selectionStart ?? text.length;
    setText(text.slice(0, epop.start) + h.emoji + text.slice(caret));
    setEpop(null);
    putCaret(epop.start + h.emoji.length);
  };
  const pick = (i: number) => (options.length ? chooseMember(options[i]) : chooseEmoji(eoptions[i]));
  const chooseMember = (m: { name: string; jid: string }) => {
    if (!pop) return;
    const caret = ta.current?.selectionStart ?? text.length;
    const next = text.slice(0, pop.start) + "@" + m.name + " " + text.slice(caret);
    const at = pop.start + m.name.length + 2;
    picked.current = [...picked.current.filter((p) => p.jid !== m.jid), { name: m.name, jid: m.jid }];
    setText(next);
    setPop(null);
    requestAnimationFrame(() => {
      ta.current?.focus();
      ta.current?.setSelectionRange(at, at);
    });
  };
  const typingOn = useRef(false);
  const typingTimer = useRef<ReturnType<typeof setTimeout>>();
  // Keep unsent text per chat (the conversation remounts for each chat).
  const textRef = useRef(text);
  textRef.current = text;
  useEffect(
    () => () => {
      if (textRef.current.trim()) unsent.set(chat, textRef.current);
      else unsent.delete(chat);
    },
    [chat],
  );

  useEffect(() => {
    if (editing) setText(editing.text);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editing?.id]);

  useEffect(() => {
    ta.current?.focus();
  }, [focusKey]);

  // Auto-grow.
  useLayoutEffect(() => {
    const el = ta.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [text]);

  const stopTyping = () => {
    clearTimeout(typingTimer.current);
    if (typingOn.current) {
      typingOn.current = false;
      inbox.setTyping(chat, false);
    }
  };
  useEffect(() => stopTyping, [chat]); // eslint-disable-line react-hooks/exhaustive-deps

  const onChange = (v: string) => {
    setText(v);
    if (editing) return;
    if (v.trim() && !typingOn.current) {
      typingOn.current = true;
      inbox.setTyping(chat, true);
    }
    clearTimeout(typingTimer.current);
    typingTimer.current = setTimeout(stopTyping, v.trim() ? 4000 : 0);
  };

  const sendText = async () => {
    const body = text.trim();
    if (!body || sending) return;
    setSending(true);
    stopTyping();
    try {
      if (editing) {
        const m = await inbox.edit(chat, editing.id, body);
        onSent(m);
        onCancelEdit();
      } else {
        const enc = encodeMentions(body, picked.current);
        const m = await inbox.sendText({ chat, text: enc.text, quotedId: replyTo?.id ?? "", mentions: enc.mentions });
        onSent(m);
        onCancelReply();
        picked.current = [];
      }
      setText("");
      unsent.delete(chat);
    } catch (e) {
      onError(errText(e));
    } finally {
      setSending(false);
      ta.current?.focus();
    }
  };

  const sendTray = async () => {
    if (!tray.length || sending) return;
    setSending(true);
    const items = tray;
    try {
      for (let i = 0; i < items.length; i++) {
        const t = items[i];
        const common = { chat, caption: i === 0 ? caption.trim() : "", quotedId: i === 0 ? (replyTo?.id ?? "") : "" };
        const m =
          t.source === "path"
            ? await inbox.sendFile({ ...common, path: t.path })
            : await inbox.sendFile({ ...common, dataBase64: await blobToBase64(t.file), fileName: t.name, mime: t.file.type });
        onSent(m);
        setTray((cur) => cur.filter((x) => x.id !== t.id));
        releaseTray([t]);
      }
      setCaption("");
      onCancelReply();
    } catch (e) {
      onError(errText(e));
    } finally {
      setSending(false);
    }
  };

  const attach = async () => {
    try {
      const paths = await inbox.chooseFiles();
      if (paths.length) setTray((t) => [...t, ...trayFromPaths(paths)]);
    } catch (e) {
      onError(errText(e));
    }
  };

  const onPaste = (e: React.ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files ?? []);
    if (files.length) {
      e.preventDefault();
      setTray((t) => [...t, ...trayFromFiles(files)]);
    }
  };

  const insertEmoji = (emoji: string) => {
    const el = ta.current;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? text.length;
    const next = text.slice(0, start) + emoji + text.slice(end);
    onChange(next);
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      const pos = start + emoji.length;
      el.setSelectionRange(pos, pos);
    });
  };

  const banner = editing ?? replyTo;
  return (
    <div className="ib-comp">
      {banner && (
        <div className="ib-comp__reply">
          <span className="ib-comp__replytext">
            {editing ? (
              <>
                <b>Editing</b> your message
              </>
            ) : (
              <>
                Replying to <b>{replyTo!.fromMe ? "yourself" : replyTo!.senderName || chatName}</b>: {messageSnippet(replyTo!) || kindLabel(replyTo!.kind)}
              </>
            )}
          </span>
          <button
            type="button"
            className="ib-iconbtn"
            aria-label={editing ? "Stop editing" : "Cancel reply"}
            onClick={() => {
              if (editing) {
                onCancelEdit();
                setText("");
              } else onCancelReply();
            }}
          >
            <Icon name="close" size={14} strokeWidth={2} />
          </button>
        </div>
      )}

      {tray.length > 0 && (
        <div className="ib-tray" role="group" aria-label="Files to send">
          <div className="ib-tray__files">
            {tray.map((t) => (
              <div key={t.id} className="ib-tray__file">
                {t.source === "blob" && t.preview ? <img src={t.preview} alt="" /> : <Icon name="file" size={22} />}
                <span className="ib-tray__name" title={t.name}>
                  {t.name}
                </span>
                <button
                  type="button"
                  className="ib-iconbtn"
                  aria-label={`Remove ${t.name}`}
                  onClick={() => {
                    releaseTray([t]);
                    setTray((cur) => cur.filter((x) => x.id !== t.id));
                  }}
                >
                  <Icon name="close" size={13} strokeWidth={2} />
                </button>
              </div>
            ))}
          </div>
          <div className="ib-tray__row">
            <SrLabel htmlFor="ib-caption">Caption</SrLabel>
            <input
              id="ib-caption"
              className="input input--sm"
              placeholder={tray.length > 1 ? "Caption for the first file (optional)" : "Add a caption (optional)"}
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void sendTray()}
              autoFocus
            />
            <button
              type="button"
              className="btn btn--sm"
              onClick={() => {
                releaseTray(tray);
                setTray(() => []);
                setCaption("");
              }}
            >
              Cancel
            </button>
            <button type="button" className="btn btn--primary btn--sm" disabled={sending} onClick={sendTray}>
              {sending ? "Sending…" : tray.length > 1 ? `Send ${tray.length}` : "Send"}
            </button>
          </div>
        </div>
      )}

      {recording ? (
        <VoiceRecorder chat={chat} quotedId={replyTo?.id ?? ""} onDone={() => setRecording(false)} onSent={(m) => {
          onSent(m);
          onCancelReply();
        }} onError={onError} />
      ) : (
        <div className="ib-comp__row">
          <div className="ib-emoji">
            <button
              type="button"
              className="ib-iconbtn"
              aria-label="Emoji"
              title="Emoji"
              aria-expanded={emojiOpen}
              aria-controls="ib-emoji-grid"
              onClick={() => setEmojiOpen((v) => !v)}
            >
              <Icon name="smile" size={20} />
            </button>
            {emojiOpen && (
              <div
                id="ib-emoji-grid"
                className="ib-emoji__grid"
                role="group"
                aria-label="Emoji"
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    setEmojiOpen(false);
                    ta.current?.focus();
                  }
                }}
              >
                {EMOJI.map((e, i) => (
                  <button key={e} type="button" className="ib-emoji__e" aria-label={e} autoFocus={i === 0} onClick={() => insertEmoji(e)}>
                    {e}
                  </button>
                ))}
              </div>
            )}
          </div>
          {!editing && (
            <button type="button" className="ib-iconbtn" aria-label="Attach files" title="Attach files" onClick={attach}>
              <Icon name="clip" size={20} />
            </button>
          )}
          {options.length > 0 && (
            <ul id="ib-mentions" className="ib-mentions" role="listbox" aria-label="Mention someone">
              {options.map((m, i) => (
                <li
                  key={m.jid}
                  id={`ib-mention-${i}`}
                  role="option"
                  aria-selected={i === popIndex}
                  className={i === popIndex ? "ib-mentions__o ib-mentions__o--on" : "ib-mentions__o"}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    chooseMember(m);
                  }}
                  onMouseEnter={() => setPopIndex(i)}
                >
                  <Avatar jid={m.jid} initials={initialsOf(m.name)} size={26} />
                  <span className="ib-mentions__name">{m.name}</span>
                  {m.phone && <span className="ib-mentions__phone">{m.phone}</span>}
                </li>
              ))}
            </ul>
          )}
          {eoptions.length > 0 && (
            <ul id="ib-mentions" className="ib-mentions" role="listbox" aria-label="Emoji">
              {eoptions.map((h, i) => (
                <li
                  key={h.name}
                  id={`ib-mention-${i}`}
                  role="option"
                  aria-selected={i === popIndex}
                  className={i === popIndex ? "ib-mentions__o ib-mentions__o--on" : "ib-mentions__o"}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    pick(i);
                  }}
                  onMouseEnter={() => setPopIndex(i)}
                >
                  <span className="ib-mentions__emoji" aria-hidden="true">
                    {h.emoji}
                  </span>
                  <span className="ib-mentions__name">:{h.name}:</span>
                </li>
              ))}
            </ul>
          )}
          <SrLabel htmlFor="ib-input">{`Message ${chatName}`}</SrLabel>
          <textarea
            id="ib-input"
            ref={ta}
            className="ib-comp__in"
            rows={1}
            placeholder={editing ? "Edit your message" : "Type a message"}
            value={text}
            onChange={(e) => {
              const done = completeEmoji(e.target.value, e.target.selectionStart);
              if (done) {
                onChange(done.text);
                setEpop(null);
                setPop(null);
                putCaret(done.caret);
                return;
              }
              onChange(e.target.value);
              findMention(e.target.value, e.target.selectionStart);
            }}
            onClick={(e) => findMention(e.currentTarget.value, e.currentTarget.selectionStart)}
            role="combobox"
            aria-expanded={count > 0}
            aria-controls="ib-mentions"
            aria-autocomplete="list"
            aria-activedescendant={count ? `ib-mention-${popIndex}` : undefined}
            onPaste={onPaste}
            onKeyDown={(e) => {
              if (count) {
                if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                  e.preventDefault();
                  const d = e.key === "ArrowDown" ? 1 : -1;
                  setPopIndex((i) => (i + d + count) % count);
                  return;
                }
                if ((e.key === "Enter" || e.key === "Tab") && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  pick(Math.min(popIndex, count - 1));
                  return;
                }
                if (e.key === "Escape") {
                  e.preventDefault();
                  setPop(null);
                  setEpop(null);
                  return;
                }
              }
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                void sendText();
              } else if (e.key === "Escape") {
                if (editing) {
                  onCancelEdit();
                  setText("");
                } else if (replyTo) onCancelReply();
                setEmojiOpen(false);
              }
            }}
            onBlur={() => typingOn.current && setTimeout(stopTyping, 1500)}
          />
          {text.trim() || editing ? (
            <button type="button" className="ib-sendbtn" aria-label={editing ? "Save edit" : "Send"} title={editing ? "Save" : "Send"} disabled={sending || !text.trim()} onClick={sendText}>
              <Icon name={editing ? "check" : "send"} size={18} strokeWidth={2} />
            </button>
          ) : (
            <button type="button" className="ib-sendbtn" aria-label="Record a voice note" title="Record a voice note" onClick={() => setRecording(true)}>
              <Icon name="mic" size={18} />
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function VoiceRecorder({
  chat,
  quotedId,
  onDone,
  onSent,
  onError,
}: {
  chat: string;
  quotedId: string;
  onDone: () => void;
  onSent: (m: ChatMessage) => void;
  onError: (t: string) => void;
}) {
  const [secs, setSecs] = useState(0);
  const [state, setState] = useState<"starting" | "recording" | "sending" | "error">("starting");
  const [err, setErr] = useState("");
  const rec = useRef<MediaRecorder | null>(null);
  const stream = useRef<MediaStream | null>(null);
  const chunks = useRef<Blob[]>([]);
  const started = useRef(0);
  const sendOnStop = useRef(false);

  useEffect(() => {
    let alive = true;
    const start = async () => {
      if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
        setErr("Voice notes can't be recorded in this window. Send an audio file with the paper clip instead.");
        setState("error");
        return;
      }
      const mime = pickRecorderMime((t) => MediaRecorder.isTypeSupported(t));
      try {
        const s = await navigator.mediaDevices.getUserMedia({ audio: true });
        if (!alive) {
          s.getTracks().forEach((t) => t.stop());
          return;
        }
        stream.current = s;
        const r = mime ? new MediaRecorder(s, { mimeType: mime }) : new MediaRecorder(s);
        rec.current = r;
        chunks.current = [];
        r.ondataavailable = (e) => e.data.size && chunks.current.push(e.data);
        r.onstop = async () => {
          s.getTracks().forEach((t) => t.stop());
          if (!sendOnStop.current) return;
          const type = r.mimeType || mime || "audio/mp4";
          const blob = new Blob(chunks.current, { type });
          const seconds = Math.max(1, Math.round((Date.now() - started.current) / 1000));
          setState("sending");
          try {
            const m = await inbox.sendVoice({ chat, dataBase64: await blobToBase64(blob), mime: type, seconds, quotedId });
            onSent(m);
            onDone();
          } catch (e) {
            onError(errText(e));
            onDone();
          }
        };
        r.start(250);
        started.current = Date.now();
        setState("recording");
      } catch (e) {
        const name = (e as { name?: string })?.name;
        setErr(
          name === "NotAllowedError"
            ? "Deputy can't use the microphone. Allow it in System Settings › Privacy & Security › Microphone, then try again."
            : `Could not start recording: ${errText(e)}`,
        );
        setState("error");
      }
    };
    void start();
    return () => {
      alive = false;
      if (rec.current && rec.current.state !== "inactive" && !sendOnStop.current) rec.current.stop();
      stream.current?.getTracks().forEach((t) => t.stop());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (state !== "recording") return;
    const t = setInterval(() => setSecs(Math.round((Date.now() - started.current) / 1000)), 250);
    return () => clearInterval(t);
  }, [state]);

  const cancel = () => {
    sendOnStop.current = false;
    if (rec.current && rec.current.state !== "inactive") rec.current.stop();
    onDone();
  };
  const send = () => {
    sendOnStop.current = true;
    rec.current?.stop();
  };

  if (state === "error") {
    return (
      <div className="ib-comp__row ib-rec">
        <span className="error-text small" role="alert">
          {err}
        </span>
        <button type="button" className="btn btn--sm" onClick={onDone} autoFocus>
          OK
        </button>
      </div>
    );
  }
  return (
    <div className="ib-comp__row ib-rec" role="group" aria-label="Recording a voice note">
      <button type="button" className="ib-iconbtn ib-iconbtn--bad" aria-label="Cancel recording" title="Cancel" onClick={cancel} disabled={state === "sending"}>
        <Icon name="trash" size={18} />
      </button>
      <span className="ib-rec__dot" aria-hidden="true" />
      <span className="ib-rec__time tabular" aria-live="off">
        {state === "starting" ? "Starting…" : state === "sending" ? "Sending…" : duration(secs)}
      </span>
      <span className="ib-rec__hint muted small">{state === "recording" ? "Recording" : ""}</span>
      <button type="button" className="ib-sendbtn" aria-label="Send voice note" title="Send" onClick={send} disabled={state !== "recording"} autoFocus>
        <Icon name="send" size={18} strokeWidth={2} />
      </button>
    </div>
  );
}
