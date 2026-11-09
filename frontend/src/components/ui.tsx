import type { ReactNode } from "react";

const paths: Record<string, ReactNode> = {
  send: <path d="M5 12h14M13 6l6 6-6 6" />,
  pencil: (
    <>
      <path d="M4 20h4L19 9l-4-4L4 16v4z" />
      <path d="M13.5 6.5l4 4" />
    </>
  ),
  trash: <path d="M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12" />,
  sidebar: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M15 4v16" />
    </>
  ),
  stop: <rect x="7" y="7" width="10" height="10" rx="1.5" />,
  chats: <path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z" />,
  agents: (
    <>
      <rect x="4" y="7" width="16" height="12" rx="3" />
      <path d="M12 3v4M9.5 12h.01M14.5 12h.01M9.5 15.5h5" />
    </>
  ),
  activity: <path d="M3 12h4l3-8 4 16 3-8h4" />,
  settings: (
    <>
      <path d="M4 6h9M17 6h3M4 12h3M11 12h9M4 18h11M19 18h1" />
      <circle cx="15" cy="6" r="2" />
      <circle cx="9" cy="12" r="2" />
      <circle cx="17" cy="18" r="2" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  close: <path d="M6 6l12 12M18 6 6 18" />,
  check: <path d="m5 12 5 5 9-10" />,
  search: (
    <>
      <circle cx="11" cy="11" r="6" />
      <path d="m20 20-4.5-4.5" />
    </>
  ),
  refresh: <path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7" />,
  terminal: <path d="m5 8 4 4-4 4M12 17h7" />,
  commit: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M3 12h6M15 12h6" />
    </>
  ),
  folder: <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />,
  mic: (
    <>
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M5 11a7 7 0 0 0 14 0M12 18v3" />
    </>
  ),
  image: (
    <>
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <circle cx="9" cy="10" r="1.6" />
      <path d="m21 16-5-5-9 8" />
    </>
  ),
  chevron: <path d="m6 9 6 6 6-6" />,
  reply: <path d="M10 8 5 12l5 4M5 12h9a5 5 0 0 1 5 5v1" />,
  smile: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M8.5 14a4.5 4.5 0 0 0 7 0M9 9.5h.01M15 9.5h.01" />
    </>
  ),
  clip: <path d="m20 11-8 8a5 5 0 0 1-7-7l8-8a3.5 3.5 0 0 1 5 5l-8 8a2 2 0 0 1-3-3l7-7" />,
  copy: (
    <>
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V5a1 1 0 0 1 1-1h9" />
    </>
  ),
  play: <path d="M8 5v14l11-7z" />,
  file: (
    <>
      <path d="M6 3h8l5 5v13H6z" />
      <path d="M14 3v5h5" />
    </>
  ),
  pin: <path d="M9 4h6l-1 6 3 3H7l3-3zM12 13v7" />,
  muted: <path d="M11 5 6 9H3v6h3l5 4zM16 9l5 6M21 9l-5 6" />,
  down: <path d="M12 5v14M6 13l6 6 6-6" />,
  timer: (
    <>
      <circle cx="12" cy="13" r="8" />
      <path d="M12 9v4l2.5 2.5M10 2h4" />
    </>
  ),
  viewonce: (
    <>
      <circle cx="12" cy="12" r="9" strokeDasharray="4 2.2" />
      <path d="M10.5 9.5 12.5 8v8" />
    </>
  ),
};

export function Icon({ name, size = 18, strokeWidth = 1.8 }: { name: keyof typeof paths | string; size?: number; strokeWidth?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="icon"
    >
      {paths[name]}
    </svg>
  );
}

/** Logo is Positron's mark: a chat bubble with typing dots on a tilted orbit, with two particles. */
export function Logo({ size = 26 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" fill="none" aria-hidden="true" className="icon">
      <ellipse
        cx="32"
        cy="32"
        rx="27"
        ry="12"
        transform="rotate(-24 32 32)"
        stroke="currentColor"
        strokeWidth="3.2"
        opacity="0.45"
      />
      <path
        d="M19 19h26a5 5 0 0 1 5 5v14a5 5 0 0 1-5 5H30l-8 6v-6h-3a5 5 0 0 1-5-5V24a5 5 0 0 1 5-5z"
        style={{ fill: "var(--side)" }}
        stroke="currentColor"
        strokeWidth="5"
        strokeLinejoin="round"
      />
      <circle cx="23.5" cy="31" r="3" fill="currentColor" />
      <circle cx="32" cy="31" r="3" fill="currentColor" />
      <circle cx="40.5" cy="31" r="3" fill="#ff5c8a" />
      <circle cx="56.67" cy="21.02" r="4.6" fill="#ff5c8a" />
      <circle cx="7.33" cy="42.98" r="4.6" fill="currentColor" />
    </svg>
  );
}

export type Tone = "ok" | "warn" | "bad" | "acc" | "plain";

export function Pill({ tone = "plain", children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`pill pill--${tone}`}>{children}</span>;
}

export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
  id,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  id: string;
}) {
  return (
    <div className="field">
      <span id={id} className="field__label">
        {label}
      </span>
      <div role="radiogroup" aria-labelledby={id} className="seg">
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={value === o.value}
            className={value === o.value ? "seg__opt seg__opt--on" : "seg__opt"}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );
}

export function Check({
  id,
  checked,
  onChange,
  title,
  help,
  helpTone,
  disabled,
}: {
  id: string;
  checked: boolean;
  onChange?: (v: boolean) => void;
  title: string;
  help?: ReactNode;
  helpTone?: "warn";
  disabled?: boolean;
}) {
  return (
    <label className={disabled ? "check check--disabled" : "check"} htmlFor={id}>
      <input id={id} type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange?.(e.target.checked)} />
      <span>
        <span className="check__title">{title}</span>
        {help && <span className={helpTone === "warn" ? "check__help check__help--warn" : "check__help"}>{help}</span>}
      </span>
    </label>
  );
}

export function Switch({ on, onChange, label }: { on: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className="switch" onClick={() => onChange(!on)}>
      <span className={on ? "switch__track switch__track--on" : "switch__track"}>
        <span className="switch__knob" />
      </span>
      {on ? "On" : "Paused"}
    </button>
  );
}

/** Confirm is an in-page confirmation row; window.confirm is not used. */
export function Confirm({
  text,
  yes,
  onYes,
  onNo,
  danger,
}: {
  text: string;
  yes: string;
  onYes: () => void;
  onNo: () => void;
  danger?: boolean;
}) {
  return (
    <div className="confirm" role="alertdialog" aria-label={text}>
      <span>{text}</span>
      <div className="row">
        <button type="button" className={danger ? "btn btn--danger" : "btn btn--primary"} onClick={onYes}>
          {yes}
        </button>
        <button type="button" className="btn" onClick={onNo}>
          Cancel
        </button>
      </div>
    </div>
  );
}

export function SrLabel({ htmlFor, children }: { htmlFor: string; children: ReactNode }) {
  return (
    <label htmlFor={htmlFor} className="sr-only">
      {children}
    </label>
  );
}
