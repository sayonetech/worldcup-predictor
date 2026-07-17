import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useDropdownPortalPosition } from "./DropdownPortal";
import { ChevronIcon, CheckIcon } from "./icons";
import type { BonusUserPicks } from "../lib/bonus";

type Props = {
  users: BonusUserPicks[];
  /** null = view your own picks */
  selectedUserId: number | null;
  onSelect: (userId: number | null) => void;
};

const ARIA_LABEL = "View bonus picks by user";

/**
 * Read-only "Viewing" dropdown shown in the Bonus panel once picks lock.
 * Lists "You" plus every other user who set picks.
 */
export function BonusUserSelect({ users, selectedUserId, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuStyle = useDropdownPortalPosition(open, wrapRef, 246);

  useEffect(() => {
    if (!open) return;
    function onPointerDown(e: PointerEvent) {
      const target = e.target as Node;
      const insideControl = wrapRef.current?.contains(target) ?? false;
      const insideMenu = menuRef.current?.contains(target) ?? false;
      if (!insideControl && !insideMenu) setOpen(false);
    }
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  const others = users.filter((u) => !u.is_me);
  const selected = selectedUserId == null
    ? null
    : others.find((u) => u.user_id === selectedUserId);

  return (
    <div className="bonus-viewas" ref={wrapRef}>
      <span className="bonus-viewas-label">Viewing</span>
      <button
        type="button"
        className={`bonus-sel-btn${open ? " open" : ""}`}
        onClick={() => setOpen((o) => !o)}
        aria-label={ARIA_LABEL}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <span className="bs-val">{selected ? selected.name : "You"}</span>
        <span className="bs-chev"><ChevronIcon /></span>
      </button>

      {open && menuStyle && createPortal(
        <div
          ref={menuRef}
          className="bonus-menu-wrap bonus-menu-wrap--portal"
          role="menu"
          aria-label={ARIA_LABEL}
          style={menuStyle}
        >
          <div className="bonus-menu-list">
            <button
              type="button"
              className={`bonus-opt-btn${selectedUserId == null ? " selected" : ""}`}
              role="menuitem"
              onClick={() => { onSelect(null); setOpen(false); }}
              onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
            >
              You
              {selectedUserId == null && <span className="bonus-opt-tick"><CheckIcon /></span>}
            </button>
            {others.map((u) => (
              <button
                key={u.user_id}
                type="button"
                className={`bonus-opt-btn${selectedUserId === u.user_id ? " selected" : ""}`}
                role="menuitem"
                onClick={() => { onSelect(u.user_id); setOpen(false); }}
                onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
              >
                {u.name}
                {selectedUserId === u.user_id && <span className="bonus-opt-tick"><CheckIcon /></span>}
              </button>
            ))}
          </div>
        </div>,
        document.body,
      )}
    </div>
  );
}
