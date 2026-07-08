# Argentina "VAR — NO GOAL" Stamp Goal-Bounce — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the plain "wall-impact rebound" of the existing Argentina goal-denial Easter egg with a comedic red **VAR · NO GOAL** rubber-stamp that slaps onto the goal at impact, then fades as the ball is yanked back.

**Architecture:** Purely additive to the existing `argentina-advantage` code path in `GoalBallAnimation.tsx`. A new optional `stampPoint` on the `Shot` carries the goal-line impact coordinate; `AnimatedShot` renders a **planted stamp element** (a sibling of the moving ball span, positioned at `stampPoint`, so it stays put while the ball retreats) and animates it inside the existing `requestAnimationFrame` loop using the already-computed `impactTime`. CSS-only styling with existing design tokens.

**Tech Stack:** React 18 + TypeScript, Vitest + Testing Library, plain CSS (`v2-components.css`).

## Global Constraints

- **Scope: frontend, visual-only.** Never touches score values, validation, kickoff locking, or save behavior.
- **Argentina-only.** The stamp renders **only** in the existing `argentina-advantage` path (opponent scores, defender is `ARG`). Normal matches and Argentina scoring for itself are unchanged.
- **No new fonts, no new image assets.** Use existing tokens: `--danger`, `--font-display`.
- **Reduced motion:** no new handling — the component already early-returns under `prefers-reduced-motion: reduce`; the stamp inherits that.
- **Layer stays `aria-hidden`.** The stamp is decorative.
- **TDD, frequent commits.** Test hooks (`data-var-stamp` attribute + `.goal-var-stamp` class) must not depend on `requestAnimationFrame` firing.

---

### Task 1: Failing test — stamp presence only under Argentina advantage

**Files:**
- Test (modify): `frontend/src/components/MatchCard.test.tsx` — extend the existing test `"uses Argentina Advantage only when the opponent attempts to score against Argentina"` (starts at line 152).

**Interfaces:**
- Consumes: existing `MatchCard`, the `argentina-advantage` behavior, `container` from `wrap(...)`.
- Produces: the test contract that Task 2 implements — a `data-var-stamp="true"` attribute on the advantage shot span and exactly one `.goal-var-stamp` element in the container; neither present for the normal (Argentina-for-itself) shot.

- [ ] **Step 1: Add the "no stamp yet" assertions after the normal-shot checks.**

In `frontend/src/components/MatchCard.test.tsx`, immediately after the existing line:

```ts
    expect(argentinaShot).toHaveAttribute("data-end-area", "opponent-flag");
```

add:

```ts
    // Argentina scoring for itself is a normal shot — no VAR stamp.
    expect(argentinaShot).not.toHaveAttribute("data-var-stamp");
    expect(container.querySelectorAll(".goal-var-stamp")).toHaveLength(0);
```

- [ ] **Step 2: Add the "stamp present" assertions after the force-wall check.**

Immediately after the existing line:

```ts
    expect(shots[1].querySelector(".goal-force-wall")).toBeInTheDocument();
```

add:

```ts
    // The opponent's denied shot at Argentina now carries a VAR NO GOAL stamp.
    expect(shots[1]).toHaveAttribute("data-var-stamp", "true");
    expect(container.querySelectorAll(".goal-var-stamp")).toHaveLength(1);
```

- [ ] **Step 3: Run the test and verify it FAILS.**

Run: `cd frontend && pnpm vitest run src/components/MatchCard.test.tsx`
Expected: FAIL — the `data-var-stamp` attribute and `.goal-var-stamp` element do not exist yet (assertion errors on the two new blocks).

- [ ] **Step 4: Commit the failing test.**

```bash
git add frontend/src/components/MatchCard.test.tsx
git commit -m "test: expect VAR NO GOAL stamp on Argentina-denial shot"
```

---

### Task 2: Implement the planted VAR stamp

**Files:**
- Modify: `frontend/src/components/GoalBallAnimation.tsx`
- Modify: `frontend/src/styles/v2-components.css` (append after the `.goal-force-wall` rules, around line 397)

**Interfaces:**
- Consumes: existing `Point`, `Shot`, `AnimatedShot`, the `useArgentinaAdvantage` branch and its local `impact: Point`, the existing `impactTime` and `drawFrame` loop.
- Produces: `Shot.stampPoint?: Point`; a `.goal-var-stamp` element and `data-var-stamp="true"` attribute rendered only when `stampPoint` is set.

- [ ] **Step 1: Add `stampPoint` to the `Shot` interface.**

In `GoalBallAnimation.tsx`, in the `Shot` interface (around line 50-60), add the field after `spinDirection`:

```ts
  spinDirection: 1 | -1;
  stampPoint?: Point;
```

- [ ] **Step 2: Set `stampPoint` when building an Argentina-advantage shot.**

In the `play` handler, declare a holder before the `if (useArgentinaAdvantage)` block. Find:

```ts
        let phases: MotionPhase[] = [{
          segment: normalSegment,
          duration: DURATION_MS,
        }];
```

and insert immediately before it:

```ts
        let stampPoint: Point | undefined;
```

Then, inside the `if (useArgentinaAdvantage) {` block, right after `impact` is defined:

```ts
          const impact = {
            x: end.x + towardCenter * nearTargetOffset,
            y: end.y + Math.min(8, Math.abs(start.y - end.y) * 0.08),
          };
```

add:

```ts
          stampPoint = impact;
```

Finally, in the `setShots(...)` object literal, add the field after `spinDirection`:

```ts
          spinDirection: side === "left" ? 1 : -1,
          stampPoint,
```

- [ ] **Step 3: Add a `stampRef` and render the planted stamp in `AnimatedShot`.**

In `AnimatedShot`, alongside the existing refs (after `wallRef`):

```ts
  const wallRef = useRef<HTMLSpanElement>(null);
  const stampRef = useRef<HTMLSpanElement>(null);
```

Then change the component's return from a single `<span>` to a fragment: add `data-var-stamp` to the shot span and render the planted stamp as a **sibling**. Replace the closing `</span>` of the shot element and the surrounding return so it reads:

```tsx
  return (
    <>
      <span
        ref={shotRef}
        className={`goal-animation ${shot.direction}`}
        data-direction={shot.direction}
        data-start-side={shot.startSide}
        data-target-side={shot.targetSide}
        data-animation-mode={shot.mode}
        data-end-area={shot.mode === "argentina-advantage" ? "center" : "opponent-flag"}
        data-motion-phases={shot.mode === "argentina-advantage"
          ? "approach impact upward-rebound momentum-decay"
          : "approach"}
        data-impact-side={shot.mode === "argentina-advantage" ? shot.targetSide : undefined}
        data-var-stamp={shot.stampPoint ? "true" : undefined}
        data-bounce-count="0"
      >
        <span className="goal-trail goal-trail--haze" />
        <span className="goal-trail goal-trail--core" />
        <span ref={forceRef} className="goal-force-ripple" />
        <span ref={wallRef} className="goal-force-wall" />
        <span className="goal-ball-glow" />
        <img ref={ballRef} className="goal-ball" src={footballImage} alt="" />
      </span>
      {shot.stampPoint ? (
        <span ref={stampRef} className="goal-var-stamp" aria-hidden="true">
          <span className="goal-var-stamp__top">⚠ VAR</span>
          <span className="goal-var-stamp__main">NO GOAL</span>
        </span>
      ) : null}
    </>
  );
```

- [ ] **Step 4: Animate the stamp inside the existing `drawFrame` loop.**

In `drawFrame`, after the existing `if (wallRef.current) { ... }` block and before the `if (elapsed < shot.duration)` line, add:

```ts
      if (stampRef.current && shot.stampPoint && impactTime >= 0) {
        const since = elapsed - impactTime;
        const appear = Math.min(1, Math.max(0, since) / 90);
        const punch = Math.min(1, Math.max(0, since) / 140);
        const easedPunch = 1 - (1 - punch) ** 3;
        const stampScale = since < 0 ? 1.5 : 1.5 - 0.5 * easedPunch;
        const holdEnd = 950;
        const fadeDuration = 540;
        const fade = since <= holdEnd
          ? 1
          : Math.max(0, 1 - (since - holdEnd) / fadeDuration);
        const stampOpacity = since < 0 ? 0 : appear * fade;
        const stampRotate = -12 + easedPunch * 8;
        stampRef.current.style.opacity = String(stampOpacity);
        stampRef.current.style.transform =
          `translate3d(${shot.stampPoint.x}px, ${shot.stampPoint.y}px, 0)`
          + ` translate(-50%, -50%) rotate(${stampRotate}deg) scale(${stampScale})`;
      }
```

- [ ] **Step 5: Add the stamp styles.**

In `frontend/src/styles/v2-components.css`, after the `.goal-animation[data-impact-side="left"] .goal-force-wall { ... }` rule (around line 397), append:

```css
.goal-var-stamp {
  position: absolute;
  left: 0;
  top: 0;
  z-index: 6;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  padding: 3px 8px;
  border: 2px solid var(--danger);
  border-radius: 6px;
  background: color-mix(in oklab, var(--danger) 16%, transparent);
  color: var(--danger);
  font-family: var(--font-display);
  font-variant-numeric: tabular-nums;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  line-height: 1;
  white-space: nowrap;
  opacity: 0;
  box-shadow:
    0 2px 10px rgba(0, 0, 0, 0.4),
    inset 0 0 0 1px color-mix(in oklab, var(--danger) 40%, transparent);
  pointer-events: none;
  will-change: transform, opacity;
}
.goal-var-stamp__top { font-size: 8px; font-weight: 800; opacity: 0.9; }
.goal-var-stamp__main { font-size: 13px; font-weight: 800; }
```

- [ ] **Step 6: Run the test and verify it PASSES.**

Run: `cd frontend && pnpm vitest run src/components/MatchCard.test.tsx`
Expected: PASS — including the two new assertion blocks from Task 1.

- [ ] **Step 7: Type-check.**

Run: `cd frontend && pnpm tsc --noEmit`
Expected: no errors.

- [ ] **Step 8: Manual visual verify (real browser).**

Run `pnpm dev`, open a match card where Argentina is one side, tap **+** for the *opponent*. Confirm: the ball flies to Argentina's goal, a red **⚠ VAR / NO GOAL** stamp punches in at the goal and holds while the ball retreats to midfield and fades. Tap **+** for Argentina itself and for a non-Argentina match — confirm **no stamp** appears.

- [ ] **Step 9: Commit.**

```bash
git add frontend/src/components/GoalBallAnimation.tsx frontend/src/styles/v2-components.css
git commit -m "feat: VAR NO GOAL stamp on Argentina goal-denial Easter egg"
```

---

### Task 3: Sync the spec

**Files:**
- Modify: `docs/REQUIREMENTS.md` (the Easter-egg sentence, around line 41)

**Interfaces:**
- Consumes: nothing.
- Produces: spec text matching the shipped behavior (keeps the automated PR review's spec check green).

- [ ] **Step 1: Update the Easter-egg description.**

In `docs/REQUIREMENTS.md`, find the sentence containing:

```text
As a visual-only Easter egg, an opponent's shot toward Argentina reaches the Argentina box, meets a subtle wall-impact cue, deflects upward, then loses momentum as it arcs down to midfield and fades;
```

Replace the phrase `meets a subtle wall-impact cue, deflects upward,` with:

```text
is denied at the goal by a "VAR — NO GOAL" stamp that punches in, then rebounds and
```

so the sentence reads: "...an opponent's shot toward Argentina reaches the Argentina box, is denied at the goal by a "VAR — NO GOAL" stamp that punches in, then rebounds and loses momentum as it arcs down to midfield and fades; Argentina's own shots and matches without Argentina use the normal path..."

- [ ] **Step 2: Commit.**

```bash
git add docs/REQUIREMENTS.md
git commit -m "docs: spec — Argentina denial Easter egg now shows a VAR NO GOAL stamp"
```

---

## Self-Review

- **Spec coverage:** Trigger unchanged (Task 2 Step 2 gates on the existing `useArgentinaAdvantage`); stamp punch-in/hold/fade (Task 2 Step 4); planted-at-goal, not following the ball (sibling element, Task 2 Step 3); `--danger`/`--font-display`, no new assets (Task 2 Step 5); reduced-motion inherited (no code — existing early-return); test hook independent of rAF (`data-var-stamp` + class, Task 1); spec sync (Task 3). All covered.
- **Placeholder scan:** none — every step has literal code/commands.
- **Type consistency:** `stampPoint?: Point` declared (Task 2 Step 1), assigned as `Point | undefined` (Step 2), read as `shot.stampPoint` (Steps 3-4). `stampRef: RefObject<HTMLSpanElement>`. `data-var-stamp` string values `"true"`/`undefined` match the test's `toHaveAttribute("data-var-stamp", "true")` and `not.toHaveAttribute`.
