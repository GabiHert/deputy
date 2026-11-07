import { useEffect, useRef, useState } from "react";
import { errText, inbox, type Contact } from "../../api";
import { Icon, SrLabel } from "../../components/ui";
import { Avatar } from "./Avatar";
import { phoneDigits } from "./helpers";

/** NewChatDialog finds a contact or group, or checks a typed number, and opens its chat. */
export function NewChatDialog({ onClose, onOpen }: { onClose: () => void; onOpen: (jid: string) => void }) {
  const [q, setQ] = useState("");
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const before = useRef<Element | null>(null);

  useEffect(() => {
    before.current = document.activeElement;
    return () => (before.current as HTMLElement | null)?.focus?.();
  }, []);

  useEffect(() => {
    let alive = true;
    const t = setTimeout(
      () =>
        inbox
          .contacts(q.trim())
          .then((c) => alive && setContacts(c.slice(0, 100)))
          .catch(() => alive && setContacts([])),
      q ? 150 : 0,
    );
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [q]);

  const digits = phoneDigits(q);
  const start = async () => {
    if (!digits) return;
    setBusy(true);
    setErr("");
    try {
      const c = await inbox.startChat(digits);
      onOpen(c.jid);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  // Keep Tab inside the dialog.
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      onClose();
      return;
    }
    if (e.key !== "Tab" || !box.current) return;
    const f = Array.from(box.current.querySelectorAll<HTMLElement>("button, input")).filter((x) => !x.hasAttribute("disabled"));
    if (!f.length) return;
    const first = f[0];
    const last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };

  return (
    <div className="ib-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className="ib-dialog" role="dialog" aria-modal="true" aria-labelledby="ib-new-title" onKeyDown={onKey}>
        <div className="ib-dialog__head">
          <h2 id="ib-new-title" className="ib-dialog__title">
            New chat
          </h2>
          <button type="button" className="ib-iconbtn" aria-label="Close" onClick={onClose}>
            <Icon name="close" size={16} strokeWidth={2} />
          </button>
        </div>
        <div className="search ib-search">
          <Icon name="search" size={15} strokeWidth={2} />
          <SrLabel htmlFor="ib-new-q">Name or phone number</SrLabel>
          <input
            id="ib-new-q"
            type="search"
            placeholder="Name or phone number, with country code"
            value={q}
            autoFocus
            autoComplete="off"
            onChange={(e) => {
              setQ(e.target.value);
              setErr("");
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (digits && !contacts.length) void start();
                else if (contacts[0]) onOpen(contacts[0].jid);
              }
            }}
          />
        </div>
        {err && (
          <span className="error-text" role="alert">
            {err}
          </span>
        )}
        <div className="ib-dialog__list">
          {digits && (
            <button type="button" className="ib-contact" onClick={start} disabled={busy}>
              <span className="avatar avatar--0 ib-avatar" style={{ width: 36, height: 36 }} aria-hidden="true">
                #
              </span>
              <span className="ib-contact__text">
                <span className="strong">{busy ? "Checking the number…" : `Message +${digits}`}</span>
                <span className="muted xsmall">Checks that the number is on WhatsApp</span>
              </span>
            </button>
          )}
          {contacts.map((c) => (
            <button key={c.jid} type="button" className="ib-contact" onClick={() => onOpen(c.jid)}>
              <Avatar jid={c.jid} initials={c.initials} size={36} />
              <span className="ib-contact__text">
                <span className="strong ellipsis">{c.name}</span>
                <span className="muted xsmall ellipsis">
                  {c.isGroup ? "Group" : [c.phone, c.pushName && c.pushName !== c.name ? `~${c.pushName}` : ""].filter(Boolean).join(" · ")}
                </span>
              </span>
            </button>
          ))}
          {!digits && contacts.length === 0 && <p className="muted small ib-empty">{q ? "No one by that name. Type a phone number to message it." : "No contacts yet."}</p>}
        </div>
      </div>
    </div>
  );
}
