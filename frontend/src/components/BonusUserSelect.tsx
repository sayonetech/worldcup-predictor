import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useDropdownPortalPosition } from "./DropdownPortal";
import { ChevronIcon, CheckIcon, SearchIcon } from "./icons";
import type { BonusUserPicks } from "../lib/bonus";

type Props = {
  users: BonusUserPicks[];
  /** null = view your own picks */
  selectedUserId: number | null;
  onSelect: (userId: number | null) => void;
  /** true while the other users' picks are still in flight */
  isLoading?: boolean;
};

const ARIA_LABEL = "View bonus picks by user";
const YOU_LABEL = "You";

/**
 * Read-only user picker shown in the Bonus panel header once picks lock.
 * Lists "You" plus every other user who set picks, filtered by a search box.
 */
export function BonusUserSelect({ users, selectedUserId, onSelect, isLoading }: Props) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuStyle = useDropdownPortalPosition(open, wrapRef, 246);

  function toggle() {
    if (isLoading) return;
    setOpen((o) => {
      const next = !o;
      if (next) {
        setQ("");
        setTimeout(() => searchRef.current?.focus(), 30);
      }
      return next;
    });
  }

  // Close on outside click
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

  // "You" is filtered like any other entry, so a query never leaves it stranded.
  const ql = q.trim().toLowerCase();
  const showYou = !ql || YOU_LABEL.toLowerCase().includes(ql);
  const filtered = ql
    ? others.filter((u) => u.name.toLowerCase().includes(ql))
    : others;

  function choose(userId: number | null) {
    onSelect(userId);
    setOpen(false);
  }

  return (
    <div className="bonus-select-wrap" ref={wrapRef}>
      <button
        type="button"
        className={`bonus-sel-btn${open ? " open" : ""}`}
        onClick={toggle}
        aria-label={ARIA_LABEL}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-busy={isLoading || undefined}
        disabled={isLoading}
      >
        <span className="bs-val">
          {isLoading ? "Loading…" : selected ? selected.name : YOU_LABEL}
        </span>
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
          <div className="bonus-search-bar">
            <SearchIcon />
            <input
              ref={searchRef}
              className="bonus-search-input"
              placeholder="Search players…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              aria-label="Search players"
              onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
            />
          </div>
          <div className="bonus-menu-list">
            {!showYou && filtered.length === 0 ? (
              <div className="bonus-menu-empty">No players match &ldquo;{q}&rdquo;</div>
            ) : (
              <>
                {showYou && (
                  <button
                    type="button"
                    className={`bonus-opt-btn${selectedUserId == null ? " selected" : ""}`}
                    role="menuitem"
                    onClick={() => choose(null)}
                    onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
                  >
                    {YOU_LABEL}
                    {selectedUserId == null && <span className="bonus-opt-tick"><CheckIcon /></span>}
                  </button>
                )}
                {filtered.map((u) => (
                  <button
                    key={u.user_id}
                    type="button"
                    className={`bonus-opt-btn${selectedUserId === u.user_id ? " selected" : ""}`}
                    role="menuitem"
                    onClick={() => choose(u.user_id)}
                    onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
                  >
                    {u.name}
                    {selectedUserId === u.user_id && <span className="bonus-opt-tick"><CheckIcon /></span>}
                  </button>
                ))}
              </>
            )}
          </div>
        </div>,
        document.body,
      )}
    </div>
  );
}
