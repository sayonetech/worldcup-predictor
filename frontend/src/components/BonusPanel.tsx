import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  CATEGORIES,
  useBonus,
  useTeams,
  useSaveBonus,
  useAllBonusPredictions,
  type TeamOption,
  type BonusPick,
  type PlayerOption,
} from "../lib/bonus";
import { PlayerCombobox } from "./PlayerCombobox";
import { BonusUserSelect } from "./BonusUserSelect";
import { useDropdownPortalPosition } from "./DropdownPortal";
import { Flag } from "./Flag";
import {
  TrophyIcon,
  LockIcon,
  ChevronIcon,
  SearchIcon,
  CheckIcon,
  SparkIcon,
} from "./icons";

// ── IST countdown to lock_at ────────────────────────────────────────────────
function useCountdown(lockAt: string | undefined): string {
  const [display, setDisplay] = useState("");
  useEffect(() => {
    if (!lockAt) return;
    const tick = () => {
      const diff = new Date(lockAt).getTime() - Date.now();
      if (diff <= 0) { setDisplay("Locked"); return; }
      const d = Math.floor(diff / 86_400_000);
      const h = Math.floor((diff % 86_400_000) / 3_600_000);
      const m = Math.floor((diff % 3_600_000) / 60_000);
      const s = Math.floor((diff % 60_000) / 1_000);
      setDisplay(
        d > 0
          ? `${d}d ${h.toString().padStart(2, "0")}h ${m.toString().padStart(2, "0")}m`
          : `${h.toString().padStart(2, "0")}:${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`,
      );
    };
    tick();
    const id = window.setInterval(tick, 1_000);
    return () => window.clearInterval(id);
  }, [lockAt]);
  return display;
}

// ── Team-select dropdown with search ────────────────────────────────────────
interface TeamSelectProps {
  teams: TeamOption[];
  selectedId: number | undefined;
  disabled: boolean;
  ariaLabel: string;
  onSelect: (id: number) => void;
}

function TeamSelect({ teams, selectedId, disabled, ariaLabel, onSelect }: TeamSelectProps) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuStyle = useDropdownPortalPosition(open, wrapRef, 246);

  const selTeam = selectedId != null ? teams.find((t) => t.id === selectedId) : null;

  function toggle() {
    if (disabled) return;
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

  const ql = q.trim().toLowerCase();
  const filtered = ql
    ? teams.filter((t) => t.name.toLowerCase().includes(ql) || t.code.toLowerCase().includes(ql))
    : teams;

  return (
    <div className="bonus-select-wrap" ref={wrapRef}>
      <button
        type="button"
        className={`bonus-sel-btn${open ? " open" : ""}`}
        onClick={toggle}
        aria-label={ariaLabel}
        aria-haspopup="menu"
        aria-expanded={open}
        disabled={disabled}
      >
        {selTeam ? (
          <span className="bs-val">
            <Flag code={selTeam.code} size={22} />
            <span style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {selTeam.name}
            </span>
            <span className="bs-code">{selTeam.code}</span>
          </span>
        ) : (
          <span className="bs-placeholder">Select team…</span>
        )}
        <span className="bs-chev"><ChevronIcon /></span>
      </button>

      {open && menuStyle && createPortal(
        <div
          ref={menuRef}
          className="bonus-menu-wrap bonus-menu-wrap--portal"
          role="menu"
          aria-label={ariaLabel}
          style={menuStyle}
        >
          <div className="bonus-search-bar">
            <SearchIcon />
            <input
              ref={searchRef}
              className="bonus-search-input"
              placeholder="Search teams…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              aria-label="Search teams"
              onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
            />
          </div>
          <div className="bonus-menu-list">
            {filtered.length === 0 ? (
              <div className="bonus-menu-empty">No teams match &ldquo;{q}&rdquo;</div>
            ) : (
              filtered.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  className={`bonus-opt-btn${selectedId === t.id ? " selected" : ""}`}
                  role="menuitem"
                  onClick={() => { onSelect(t.id); setOpen(false); }}
                  onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
                >
                  <Flag code={t.code} size={20} />
                  {t.name}
                  <span className="bs-code">{t.code}</span>
                  {selectedId === t.id && <span className="bonus-opt-tick"><CheckIcon /></span>}
                </button>
              ))
            )}
          </div>
        </div>,
        document.body,
      )}
    </div>
  );
}

// ── Skeleton ────────────────────────────────────────────────────────────────
function BonusPanelSkeleton() {
  return (
    <div className="bonus-panel" aria-busy="true" aria-label="Loading Tournament Bonus">
      <div className="bonus-head" style={{ cursor: "default" }}>
        <span className="bonus-head-icon" aria-hidden="true"><TrophyIcon /></span>
        <div className="bonus-head-txt">
          <div className="skeleton skeleton--text" style={{ width: 180, height: 17, marginBottom: 6 }} />
          <div className="skeleton skeleton--text" style={{ width: 140, height: 12 }} />
        </div>
      </div>
    </div>
  );
}

// ── Main BonusPanel ──────────────────────────────────────────────────────────
export function BonusPanel() {
  const { data: bonus, isLoading, isError } = useBonus();
  const { data: teams = [], isLoading: teamsLoading } = useTeams();
  const saveMutation = useSaveBonus();

  const [open, setOpen] = useState(false);
  const countdown = useCountdown(bonus?.lock_at);
  const locked = bonus?.locked ?? false;

  // Optimistic label state for player picks (mirrors Bonus.tsx)
  const [optimisticLabels, setOptimisticLabels] = useState<Record<string, string>>({});

  const [viewUserId, setViewUserId] = useState<number | null>(null);
  // Only fetch once locked+open: the server 403s before the lock boundary.
  const { data: allPicks = [] } = useAllBonusPredictions(locked && open);

  if (isLoading || teamsLoading) return <BonusPanelSkeleton />;

  if (isError) {
    return (
      <div className="bonus-panel" aria-label="Tournament Bonus">
        <div className="bonus-head" style={{ cursor: "default" }}>
          <span className="bonus-head-icon"><TrophyIcon /></span>
          <div className="bonus-head-txt">
            <div className="bonus-head-title">Tournament Bonus</div>
          </div>
        </div>
        <p className="bonus-panel-error" role="alert">
          Couldn&apos;t load bonus picks. Please refresh and try again.
        </p>
      </div>
    );
  }

  const viewingOther = viewUserId !== null;
  const viewedPicks = viewingOther
    ? (allPicks.find((u) => u.user_id === viewUserId)?.picks ?? [])
    : (bonus?.picks ?? []);
  const pickMap = new Map<string, BonusPick>(
    viewedPicks.map((p) => [p.category, p]),
  );

  const setPicked = bonus?.picks.filter((p) => p.ref_id).length ?? 0;
  const totalCats = CATEGORIES.length;
  const maxPts = CATEGORIES.reduce((s, c) => s + c.points, 0);
  const earnedPts = (bonus?.picks ?? []).reduce((s, p) => s + (p.points ?? 0), 0);

  const handleTeamChange = (categoryKey: string, refId: number) => {
    saveMutation.mutate([{ category: categoryKey, ref_id: refId }]);
  };

  const handlePlayerSelect = (categoryKey: string, refId: number, label: string) => {
    setOptimisticLabels((prev) => ({ ...prev, [categoryKey]: label }));
    saveMutation.mutate([{ category: categoryKey, ref_id: refId }], {
      onSuccess: () => {
        setOptimisticLabels((prev) => {
          const next = { ...prev };
          delete next[categoryKey];
          return next;
        });
      },
      onError: () => {
        setOptimisticLabels((prev) => {
          const next = { ...prev };
          delete next[categoryKey];
          return next;
        });
      },
    });
  };

  const isDisabled = locked || saveMutation.isPending;

  return (
    <div className={`bonus-panel${open ? " open" : ""}`}>
      {/* ── Header / toggle ─────────────────────────────────────────────── */}
      <button
        type="button"
        className="bonus-head"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-controls="bonus-body"
        aria-label={open ? "Hide Tournament Bonus picks" : "Set Tournament Bonus picks"}
      >
        <span className="bonus-head-icon"><TrophyIcon /></span>

        <div className="bonus-head-txt">
          <div className="bonus-head-title">
            Tournament Bonus
            <span className="bonus-head-tag">
              <span className="mono">{earnedPts}</span>
              <span>/{maxPts} pts</span>
            </span>
          </div>
          <div className="bonus-head-sub">
            <span>{setPicked}/{totalCats} picks set · predict the big honours once</span>
            {locked ? (
              <span className="bonus-lock-pill"><LockIcon /> Locked</span>
            ) : bonus?.lock_at ? (
              <span className="bonus-lock-pill">
                <LockIcon />
                <span>Locks in <span className="mono">{countdown}</span></span>
              </span>
            ) : null}
          </div>
        </div>

        <span className={`bonus-toggle${open ? " open" : ""}`} aria-hidden="true">
          {open ? "Hide" : "Set picks"} <ChevronIcon />
        </span>
      </button>

      {/* ── Save error ───────────────────────────────────────────────────── */}
      {saveMutation.isError && (
        <p className="bonus-panel-error" role="alert">
          {saveMutation.error instanceof Error &&
            saveMutation.error.message.includes("403")
            ? "Picks are now locked — no further changes allowed."
            : "Couldn’t save your pick — please try again."}
        </p>
      )}

      {/* ── Body (collapsible) ──────────────────────────────────────────── */}
      {open && (
        <div className="bonus-body" id="bonus-body">
          {locked && (
            <BonusUserSelect
              users={allPicks}
              selectedUserId={viewUserId}
              onSelect={setViewUserId}
            />
          )}
          <div className="bonus-grid" role="list" aria-label="Tournament Bonus categories">
            {CATEGORIES.map((cat) => {
              const pick = pickMap.get(cat.key);
              const isSet = pick?.ref_id != null;

              return (
                <div
                  key={cat.key}
                  className={`bonus-grid-row${isSet ? " set" : ""}`}
                  role="listitem"
                >
                  <div className="bonus-row-l">
                    <span className="bonus-dot" aria-hidden="true" />
                    <div>
                      <div className="bonus-label">{cat.label}</div>
                      <div className="bonus-pts-small">{cat.points} pts</div>
                    </div>
                  </div>

                  {cat.refType === "team" ? (
                    <TeamSelect
                      teams={teams}
                      selectedId={pick?.ref_id}
                      disabled={isDisabled || viewingOther}
                      ariaLabel={`Select team for ${cat.label}`}
                      onSelect={(id) => handleTeamChange(cat.key, id)}
                    />
                  ) : (
                    <div className="bonus-select-wrap">
                      <PlayerCombobox
                        comboboxKey={cat.key}
                        ariaLabel={`Search players for ${cat.label}`}
                        disabled={isDisabled || viewingOther}
                        currentRefId={pick?.ref_id}
                        currentLabel={viewingOther ? pick?.label : (optimisticLabels[cat.key] ?? pick?.label)}
                        onSelect={(opt: PlayerOption) =>
                          handlePlayerSelect(cat.key, opt.id, `${opt.name} · ${opt.team_code}`)
                        }
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          <div className="bonus-body-foot">
            <span style={{ color: "var(--gold)" }}><SparkIcon /></span>
            Max {maxPts} bonus points · awarded after the final
          </div>
        </div>
      )}
    </div>
  );
}
