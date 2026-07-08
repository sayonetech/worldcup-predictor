# Design — Referee "Approved" thumbs-up when you back Argentina

**Date:** 2026-07-08
**Scope:** Frontend visual-only. One new component + trigger in `MatchCard` + CSS + one
user-supplied image asset. No backend, no data, no scoring.

## Motivation

Companion tease to the "VAR — NO GOAL" stamp: the running joke is that the referee always
favours Argentina. So when you save a prediction that **backs Argentina** on an Argentina
match, a referee slides in and gives a thumbs-up — "approved," of course.

## Behavior

- **Trigger.** On a **successful** prediction save (`usePutPrediction` `onSuccess`), show the
  referee **only if both**:
  1. the match involves Argentina — `home.code === "ARG" || away.code === "ARG"`, and
  2. the saved scoreline **backs Argentina** — Argentina wins or draws:
     `argIsHome ? h >= a : a >= h`.
     (A draw counts as approval. If Argentina loses on the scoreline, no referee — regardless
     of a knockout shootout-winner pick.)
- **What shows.** A referee image slides in from the card's bottom-right corner, gives a
  small bob/nod, holds ~1 s, then slides out — ~1.8 s total (roughly matching the existing
  "✓ Saved" flash). Repeat saves re-trigger it (keyed remount).
- **Never affects** score values, validation, kickoff locking, or save behavior. It is a
  reaction to a save that already succeeded, on an `aria-hidden` decorative layer.

## The asset (user-supplied)

- File: **`frontend/public/referee.png`**, referenced as `/referee.png` (Vite serves
  `public/` at web root). The user creates a "troll" cut-out (transparent background,
  thumbs-up, optional caption). Recommended ~200–400 px tall, transparent PNG.
- **Graceful fallback.** If `/referee.png` fails to load (`onError` on the `<img>`), the whole
  overlay hides itself. So before the file exists, users just see the normal "✓ Saved" — never
  a broken-image icon.
- No caption pill by default — the troll image carries the joke, avoiding redundant text.

## Implementation

- **New component `frontend/src/components/RefereeApproval.tsx`** — presentational, self-timing
  overlay. Renders an `aria-hidden` layer containing `<img src="/referee.png">`. It:
  - runs the slide-in/bob/slide-out via a CSS animation class,
  - calls an `onDone` callback on animation end (fallback timer ~1900 ms) so the parent can unmount it,
  - on `<img>` `onError`, calls `onDone` immediately (hide; missing asset ⇒ nothing shows),
  - returns `null` under `prefers-reduced-motion: reduce` (consistent with the ball/stamp gags).
- **`frontend/src/components/MatchCard.tsx`:**
  - derive `argIsHome`, `isArgentinaMatch`, and `backsArgentina` from `home`/`away` codes and `h`/`a`,
  - add `refereeKey` state; in the save `onSuccess`, if `isArgentinaMatch && backsArgentina`,
    bump `refereeKey` to trigger,
  - mount `{refereeKey > 0 && <RefereeApproval key={refereeKey} onDone={...} />}` inside the card.
- **`frontend/src/styles/v2-components.css`:** `.referee-approval` layer + `.referee-approval__img`
  + `@keyframes referee-approve` (slide-in from bottom-right, bob, slide-out). Uses existing
  tokens; no new fonts.

## Testing (TDD)

In `frontend/src/components/MatchCard.test.tsx`, after a mocked successful save:

- **Shows** when it's an Argentina match AND the scoreline backs Argentina (ARG win or draw):
  `.referee-approval` present.
- **Absent** when it's not an Argentina match.
- **Absent** when it's an Argentina match but the scoreline has Argentina losing.

Assert on the wrapper element (`.referee-approval`) — independent of image load / rAF (jsdom
does not fire `<img>` load/error, so the wrapper renders deterministically in tests).

## Out of scope

- No backend, scoring, or data-model change.
- No caption pill, no sound, no full-screen takeover.
- The image itself is user-supplied; this change does not add or generate any binary asset.

## Spec sync

Add a bullet near the existing goal-feedback Easter egg in `docs/REQUIREMENTS.md` (§ around
line 41): saving a prediction that backs Argentina on an Argentina match shows a brief,
visual-only referee "approved" reaction; it never changes save, lock, or score behavior.
