import { useState, useEffect, useRef, useId } from "react";
import type { MatchDTO, TeamDTO } from "../lib/matches";
import { usePutPrediction, PredictionLockedError } from "../lib/matches";
import { istTime } from "../lib/ist";
import { Flag } from "./Flag";
import { Countdown } from "./Countdown";
import { GoalBallAnimation } from "./GoalBallAnimation";
import type { GoalBallAnimationHandle } from "./GoalBallAnimation";
import { RefereeApproval } from "./RefereeApproval";
import { ClockIcon, LockIcon, CheckIcon, MinusIcon, PlusIcon } from "./icons";

// ── Prediction window: 72 h (3 days) before kickoff ─────────────────────────
const WINDOW_MS = 72 * 60 * 60 * 1000; // 3 days in ms

/** Returns true when the match is within the 3-day prediction window (and not yet locked). */
function isPredictionOpen(kickoff_utc: string): boolean {
  const kickoffMs = new Date(kickoff_utc).getTime();
  return kickoffMs - Date.now() <= WINDOW_MS;
}

/** Returns the open date in IST, formatted as "d MMM, h:mm am/pm" */
function formatOpenDateIST(kickoff_utc: string): string {
  const openMs = new Date(kickoff_utc).getTime() - WINDOW_MS;
  const d = new Date(openMs);
  return d.toLocaleString("en-IN", {
    timeZone: "Asia/Kolkata",
    day: "numeric",
    month: "short",
    hour: "numeric",
    minute: "2-digit",
    hour12: true,
  });
}

// ── Pill stepper (design: ±, circle buttons, pill container) ────────────────
function PillStepper({
  label,
  value,
  onChange,
  disabled,
  plusRef,
  onIncrement,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  disabled: boolean;
  plusRef?: React.RefObject<HTMLButtonElement | null>;
  onIncrement?: () => void;
}) {
  return (
    <div className={`stepper-pill${disabled ? " off" : ""}`}>
      <button
        type="button"
        className="step-btn"
        aria-label={`Decrease ${label} score`}
        disabled={disabled || value <= 0}
        onClick={() => onChange(Math.max(0, value - 1))}
      >
        <MinusIcon />
      </button>
      <span className="step-val mono" aria-label={`${label} score: ${value}`}>{value}</span>
      <button
        type="button"
        ref={plusRef}
        className="step-btn"
        aria-label={`Increase ${label} score`}
        disabled={disabled || value >= 99}
        onClick={() => {
          onChange(Math.min(99, value + 1));
          onIncrement?.();
        }}
      >
        <PlusIcon />
      </button>
    </div>
  );
}

// ── Match group/round tag ──────────────────────────────────────────────────
function stageTag(match: MatchDTO): string {
  if (match.group) return `Group ${match.group}`;
  return match.round;
}

// ── "Not open yet" popup dialog ────────────────────────────────────────────
interface WindowDialogProps {
  homeName: string;
  awayName: string;
  kickoff_utc: string;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  onClose: () => void;
}

function PredictionWindowDialog({
  homeName,
  awayName,
  kickoff_utc,
  triggerRef,
  onClose,
}: WindowDialogProps) {
  const titleId = useId();
  const descId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const openDate = formatOpenDateIST(kickoff_utc);

  // Focus close button on mount; restore to trigger on unmount.
  useEffect(() => {
    closeRef.current?.focus();
    // Snapshot the trigger element now so the cleanup can reference it even
    // after it may have unmounted (react-hooks/exhaustive-deps requirement).
    const trigger = triggerRef.current;
    return () => {
      trigger?.focus();
    };
  }, [triggerRef]);

  // Scroll lock while open.
  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, []);

  // Focus trap + Escape.
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      onClose();
      return;
    }
    if (e.key !== "Tab") return;

    const dialog = e.currentTarget;
    const focusable = Array.from(
      dialog.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      )
    ).filter((el) => !el.hasAttribute("disabled"));

    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const focused = document.activeElement;

    if (e.shiftKey) {
      if (focused === first) {
        e.preventDefault();
        last.focus();
      }
    } else {
      if (focused === last) {
        e.preventDefault();
        first.focus();
      }
    }
  };

  // Backdrop click closes dialog.
  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) onClose();
  };

  return (
    <div
      className="pw-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={descId}
      onKeyDown={handleKeyDown}
      onClick={handleBackdropClick}
    >
      <div className="pw-dialog">
        <div className="pw-header">
          <h2 id={titleId} className="pw-title">Not open yet</h2>
          <button
            type="button"
            ref={closeRef}
            className="pw-close"
            aria-label="Close"
            onClick={onClose}
          >
            <CloseIcon />
          </button>
        </div>
        <p id={descId} className="pw-body">
          Predictions for{" "}
          <strong>{homeName} vs {awayName}</strong>{" "}
          open 3 days before kickoff — from{" "}
          <strong className="mono">{openDate} IST</strong>.
        </p>
        <div className="pw-footer">
          <button
            type="button"
            className="btn-ghost pw-ok"
            onClick={onClose}
          >
            OK
          </button>
        </div>
      </div>
    </div>
  );
}

// Inline close icon — same outline icon set as icons.tsx / HowToPlayModal.
function CloseIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="18"
      height="18"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M18 6 6 18M6 6l12 12" />
    </svg>
  );
}

// ── MatchCard: full predictor card for an upcoming match ───────────────────
type Props = { match: MatchDTO };

export function MatchCard({ match }: Props) {
  const { home, away, kickoff_ist, label, venue } = match;
  const decided = home !== null && away !== null;

  // If TBD, render read-only placeholder (no predictor)
  if (!decided) {
    return (
      <article className="match-card" aria-label={label}>
        <header className="mc-head">
          <span className="eyebrow mc-grp">
            {stageTag(match)}
            {venue ? ` · ${venue.city}` : ""}
          </span>
          <span className="mc-meta">
            <span className="mono mc-time">{istTime(kickoff_ist)} IST</span>
          </span>
        </header>
        <div className="mc-teams" style={{ justifyContent: "center", margin: "16px 0 4px" }}>
          <span style={{ color: "var(--text-3)", gridColumn: "1 / -1", textAlign: "center" }}>
            {label}
          </span>
        </div>
      </article>
    );
  }

  return <MatchCardEditor match={match} home={home as TeamDTO} away={away as TeamDTO} />;
}

// ── Inner editor (only rendered when teams are known) ─────────────────────
function MatchCardEditor({
  match,
  home,
  away,
}: {
  match: MatchDTO;
  home: TeamDTO;
  away: TeamDTO;
}) {
  const { kickoff_utc, kickoff_ist, status, locked: serverLocked, stage, venue, group, round, prediction } = match;

  const [h, setH] = useState(prediction?.home_score ?? 0);
  const [a, setA] = useState(prediction?.away_score ?? 0);
  const [pen, setPen] = useState<number | null>(prediction?.penalty_winner_team_id ?? null);
  const [saved, setSaved] = useState(false);
  const [refereeKey, setRefereeKey] = useState(0);
  const [windowDialogOpen, setWindowDialogOpen] = useState(false);

  const predictBtnRef = useRef<HTMLButtonElement>(null);
  const cardRef = useRef<HTMLElement>(null);
  const leftPlusRef = useRef<HTMLButtonElement>(null);
  const rightPlusRef = useRef<HTMLButtonElement>(null);
  const leftFlagRef = useRef<HTMLSpanElement>(null);
  const rightFlagRef = useRef<HTMLSpanElement>(null);
  const goalAnimationRef = useRef<GoalBallAnimationHandle>(null);

  const mut = usePutPrediction(match.id);
  const locked409 = mut.error instanceof PredictionLockedError;
  const locked = serverLocked || locked409;

  // Check if the 3-day prediction window is open.
  const predOpen = isPredictionOpen(kickoff_utc);

  const isDraw = h === a;
  const showPenalty = stage === "knockout" && isDraw;

  // Fix: use useEffect to clear the "Saved" flash so the timeout is cleaned up on unmount
  useEffect(() => {
    if (!saved) return;
    const id = setTimeout(() => setSaved(false), 1_600);
    return () => clearTimeout(id);
  }, [saved]);

  // Dirty logic: new pick always saveable (incl. 0-0); existing only when changed.
  const dirty =
    !prediction ||
    h !== prediction.home_score ||
    a !== prediction.away_score ||
    (showPenalty ? pen : null) !== (prediction.penalty_winner_team_id ?? null);

  const hasPick = prediction != null;
  const tag = group ? `Group ${group}` : round;

  // Detect 422 "predictions open 3 days before kickoff" from server.
  const isWindowError =
    mut.error !== null &&
    !(mut.error instanceof PredictionLockedError) &&
    mut.error instanceof Error &&
    /predictions open 3 days/i.test(mut.error.message);

  const onSave = () => {
    mut.mutate(
      {
        home_score: h,
        away_score: a,
        penalty_winner_team_id: showPenalty ? pen : null,
      },
      {
        onSuccess: () => {
          setSaved(true);
          // The useEffect on `saved` handles clearing after 1600 ms (with unmount cleanup)
          const argIsHome = home.code === "ARG";
          const isArgentinaMatch = argIsHome || away.code === "ARG";
          const backsArgentina = isArgentinaMatch && (argIsHome ? h >= a : a >= h);
          if (backsArgentina) setRefereeKey((key) => key + 1);
        },
      },
    );
  };

  // Open date hint shown on card for not-yet-open matches.
  const openDateHint = !locked && !predOpen ? formatOpenDateIST(kickoff_utc) : null;

  return (
    <>
      <article
        ref={cardRef}
        className={`match-card${hasPick ? " has-pick" : ""}`}
        aria-label={`${home.name} versus ${away.name}`}
      >
        <GoalBallAnimation
          ref={goalAnimationRef}
          mode="argentina-advantage"
          leftTeamCode={home.code}
          rightTeamCode={away.code}
          containerRef={cardRef}
          leftSourceRef={leftPlusRef}
          rightSourceRef={rightPlusRef}
          leftTargetRef={leftFlagRef}
          rightTargetRef={rightFlagRef}
        />

        {refereeKey > 0 && (
          <RefereeApproval key={refereeKey} onDone={() => setRefereeKey(0)} />
        )}

        {/* ── Head ──────────────────────────────────────────────────────────── */}
        <header className="mc-head">
          <span className="eyebrow mc-grp">
            {tag}
            {venue ? ` · ${venue.city}` : ""}
          </span>
          <span className="mc-meta">
            <span className="mono mc-time">{istTime(kickoff_ist)} IST</span>
            {locked ? (
              <span className="pill lock"><LockIcon /> Locked</span>
            ) : status === "live" ? (
              <span className="pill live">Live</span>
            ) : (
              <span className="mc-count mono">
                <ClockIcon />
                <Countdown to={kickoff_utc} />
              </span>
            )}
          </span>
        </header>

        {/* ── Teams + live scoreline ─────────────────────────────────────────── */}
        <div className="mc-teams">
          <div className="mc-team home">
            <span ref={leftFlagRef} className="mc-flag-anchor">
              <Flag code={home.code} size={46} />
            </span>
            <div className="mc-team-txt">
              <span className="mc-team-name">{home.name}</span>
              <span className="mono mc-team-code">{home.code}</span>
            </div>
          </div>

          <div className="mc-vs">
            <span className="mc-score mono">{h}</span>
            <span className="mc-dash">–</span>
            <span className="mc-score mono">{a}</span>
          </div>

          <div className="mc-team away">
            <div className="mc-team-txt">
              <span className="mc-team-name">{away.name}</span>
              <span className="mono mc-team-code">{away.code}</span>
            </div>
            <span ref={rightFlagRef} className="mc-flag-anchor">
              <Flag code={away.code} size={46} />
            </span>
          </div>
        </div>

        {/* ── Predictor row ─────────────────────────────────────────────────── */}
        <div className="mc-predict">
          {locked ? (
            /* Read-only locked state */
            <span className="mc-pick-ro">
              Pick locked · {prediction ? `${prediction.home_score}–${prediction.away_score}` : "—"}
            </span>
          ) : predOpen ? (
            /* Active stepper row (within 3-day window) */
            <div className="mc-steps">
              <PillStepper
                label={home.name}
                value={h}
                plusRef={leftPlusRef}
                onIncrement={() => goalAnimationRef.current?.play("left")}
                onChange={(v) => {
                  setH(v);
                  // Clear pen when the new scoreline is no longer a draw
                  if (v !== a) setPen(null);
                }}
                disabled={mut.isPending}
              />
              <span className="mc-steps-vs">your scoreline</span>
              <PillStepper
                label={away.name}
                value={a}
                plusRef={rightPlusRef}
                onIncrement={() => goalAnimationRef.current?.play("right")}
                onChange={(v) => {
                  setA(v);
                  // Clear pen when the new scoreline is no longer a draw
                  if (h !== v) setPen(null);
                }}
                disabled={mut.isPending}
              />
            </div>
          ) : (
            /* Not-yet-open: "Opens {date}" hint + Predict button */
            <div className="mc-not-open">
              {openDateHint && (
                <span className="mc-opens-hint">
                  Opens <span className="mono">{openDateHint} IST</span>
                </span>
              )}
              <button
                type="button"
                ref={predictBtnRef}
                className="btn-predict-locked"
                aria-haspopup="dialog"
                aria-label={`Predict ${home.name} versus ${away.name} — not open yet`}
                onClick={() => setWindowDialogOpen(true)}
              >
                Predict
              </button>
            </div>
          )}

          {!locked && predOpen && (
            <div className="mc-action">
              {saved ? (
                <span className="mc-saved"><CheckIcon /> Saved</span>
              ) : (
                <button
                  type="button"
                  className={`btn-save${dirty ? "" : " ghost"}`}
                  onClick={onSave}
                  disabled={!dirty || mut.isPending}
                  aria-label={hasPick ? "Update prediction" : "Save prediction"}
                >
                  {mut.isPending
                    ? "Saving…"
                    : hasPick
                      ? "Update pick"
                      : "Save prediction"}
                </button>
              )}
            </div>
          )}
        </div>

        {/* ── Shootout winner (knockout draw only) ──────────────────────────── */}
        {!locked && predOpen && showPenalty && (
          <div className="match__penalty" role="group" aria-label="Shootout winner">
            <span className="match__penalty-label">Shootout winner</span>
            <div className="segmented">
              <button
                type="button"
                className={`segmented__opt${pen === home.id ? " is-active" : ""}`}
                aria-pressed={pen === home.id}
                onClick={() => setPen(home.id)}
                disabled={mut.isPending}
              >
                {home.code}
              </button>
              <button
                type="button"
                className={`segmented__opt${pen === away.id ? " is-active" : ""}`}
                aria-pressed={pen === away.id}
                onClick={() => setPen(away.id)}
                disabled={mut.isPending}
              >
                {away.code}
              </button>
            </div>
          </div>
        )}

        {/* ── 409 lock error ────────────────────────────────────────────────── */}
        {locked409 && (
          <p className="match__editor-error" role="alert">
            This match locked at kickoff.
          </p>
        )}

        {/* ── 422 prediction window error (e.g. clock skew) ─────────────────── */}
        {isWindowError && (
          <p className="match__editor-error" role="alert">
            Predictions for this match are not open yet — they open 3 days before kickoff.
          </p>
        )}
      </article>

      {/* ── Not-open-yet dialog (portal rendered after the article) ─────────── */}
      {windowDialogOpen && (
        <PredictionWindowDialog
          homeName={home.name}
          awayName={away.name}
          kickoff_utc={kickoff_utc}
          triggerRef={predictBtnRef}
          onClose={() => setWindowDialogOpen(false)}
        />
      )}
    </>
  );
}
