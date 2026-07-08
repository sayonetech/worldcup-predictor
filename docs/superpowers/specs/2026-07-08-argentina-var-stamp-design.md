# Design — Argentina "VAR — NO GOAL" stamp goal-bounce

**Date:** 2026-07-08
**Scope:** Frontend visual-only. One component + its CSS. No backend, no data, no scoring.

## Motivation

The app already carries a visual-only Easter egg: when an opponent taps **+** to score
against Argentina, the ball flies at Argentina's box, meets a subtle wall-impact cue,
deflects upward, and loses momentum back to midfield — the "VARgentina" joke that
Argentina can't be scored on. Today the denial plays fairly straight (a physics-y
rebound). We want the denial to read as **funnier and more on-the-nose**: a referee-style
**VAR · NO GOAL** stamp slaps onto the goal at the moment of impact, then the ball retreats.

This is a comedic refinement of an existing behavior, not a new feature.

## Behavior

- **Trigger — unchanged.** Only the existing `argentina-advantage` path: the scoring
  team is *not* Argentina AND the defending team *is* Argentina
  (`GoalBallAnimation.tsx` — `useArgentinaAdvantage`). Every other match, and Argentina
  scoring for itself, keeps the normal ball-arc, untouched.
- **New gag.** At the impact moment (end of the approach phase, when the ball reaches the
  Argentina goal line), a red **`VAR · NO GOAL ⚠`** stamp punches in over the goal, holds
  for a beat, then fades as the ball is yanked back to center via the existing
  momentum-decay return.
- **Never affects** score values, validation, kickoff locking, or save behavior. Purely
  cosmetic, on an `aria-hidden` layer.

## Implementation

All changes live in `frontend/src/components/GoalBallAnimation.tsx` and its stylesheet.

1. **Carry the impact point.** The `argentina-advantage` branch already computes `impact`
   (the goal-line point). Store it on the `Shot` as an optional `stampPoint: Point`.
   Normal shots leave it `undefined`.
2. **Render the stamp anchored at the goal.** Add a stamp element that is a **sibling
   positioned at `stampPoint`**, not a child of the moving ball `span` — so it stays
   planted at the goal while the ball flees back to midfield. Rendered only when
   `stampPoint` is set. Text: `VAR · NO GOAL` with a ⚠ glyph.
3. **Animate by the existing clock.** Drive the stamp from the same `drawFrame` rAF loop
   using the existing `impactTime`, via a new `stampRef` — no new timers/intervals.
   Timeline relative to impact:
   - before impact: opacity 0, invisible
   - impact → +~120 ms: punchy scale-in 1.5 → 1.0 with a slight rotate/skew (rubber-stamp "slap")
   - hold through the early part of the return
   - fade out near the end of the shot
4. **Reduced motion.** No new handling needed: the component already early-returns under
   `prefers-reduced-motion: reduce` (no travelling ball, no gag). The stamp inherits that.
5. **Styling.** New `.goal-var-stamp` class using the existing `--danger` token; uppercase,
   bold, tabular system figures, drop-shadow, dark-first — consistent with spec §7. **No new
   fonts and no new image assets.**
6. **Test hook.** Expose a `data-var-stamp="true"` attribute (or the `.goal-var-stamp`
   element) only in `argentina-advantage` mode, so tests can assert presence/absence
   without depending on rAF pixel motion.

## Testing (TDD)

Extend the existing animation / `MatchCard` tests:

- Stamp element renders **only** when the opponent scores against Argentina
  (`argentina-advantage`).
- Stamp is **absent** for a normal match (no Argentina).
- Stamp is **absent** when Argentina scores for itself.
- Existing `argentina-advantage` motion assertions (data attributes / phases) still pass.

## Out of scope (explicitly not doing)

- No heckle toast, no leaderboard/bonus flair (the broader "tease pack" was declined).
- No goat sprite / new artwork (the "goat block" gag variant was declined).
- No backend, scoring, or data-model change of any kind.

## Spec sync

Update the existing Easter-egg sentence in `docs/REQUIREMENTS.md` (§ around line 41) to
describe the VAR stamp denial in place of "meets a subtle wall-impact cue, deflects upward,"
— keeping the spec and code in sync per CLAUDE.md.
