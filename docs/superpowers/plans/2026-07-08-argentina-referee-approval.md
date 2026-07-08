# Referee "Approved" Thumbs-Up (Backing Argentina) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a prediction save succeeds on an Argentina match where the saved scoreline has Argentina winning or drawing, briefly slide in a referee image giving a thumbs-up.

**Architecture:** A new self-timing presentational overlay `RefereeApproval` is conditionally mounted inside `MatchCard` on a keyed trigger. `MatchCard` computes `backsArgentina` from the team codes and the saved `h`/`a`, and in the mutation's `onSuccess` bumps a `refereeKey` to (re)mount the overlay. The overlay animates via CSS and unmounts itself after ~1.9 s (or immediately under reduced motion / on image error). The referee raster is a user-supplied file at `frontend/public/referee.png`.

**Tech Stack:** React 18 + TypeScript, Vitest + Testing Library, plain CSS (`v2-components.css`), Vite `public/` static asset.

## Global Constraints

- **Scope: frontend, visual-only.** Never touches score values, validation, kickoff locking, or save behavior. It only reacts to a save that already succeeded.
- **Trigger both conditions:** match involves Argentina (`home.code === "ARG" || away.code === "ARG"`) AND scoreline backs Argentina (`argIsHome ? h >= a : a >= h`). A draw counts; an Argentina loss does not.
- **Asset is user-supplied:** `frontend/public/referee.png` (served at `/referee.png`). This change adds **no** binary asset.
- **Graceful fallback:** if the image fails to load, hide immediately — never show a broken-image icon.
- **Reduced motion:** the overlay renders `null` under `prefers-reduced-motion: reduce`, consistent with the ball/stamp gags. Use the safe `window.matchMedia?.(...)?.matches` form (jsdom has no `matchMedia`).
- **Decorative:** overlay is `aria-hidden`, `pointer-events: none`.
- **TDD, frequent commits.** Test hook is the `.referee-approval` wrapper (renders deterministically in jsdom; no dependency on image load).

---

### Task 1: Failing tests — referee only when backing Argentina

**Files:**
- Test (modify): `frontend/src/components/MatchCard.test.tsx` — add three `it(...)` blocks inside the existing `describe("MatchCard", ...)` (e.g. after the test at line 246, `"keeps normal goal animations when Argentina is not playing"`).

**Interfaces:**
- Consumes: existing `wrap`, `baseMatch`, `screen`, `userEvent`, `waitFor`, `vi`.
- Produces: the contract Task 2 implements — a `.referee-approval` element that appears in the card after a successful save iff it is an Argentina match and the scoreline backs Argentina.

- [ ] **Step 1: Add the three tests.**

Insert into `MatchCard.test.tsx` inside the `describe` block:

```tsx
  it("shows the referee approval after saving a pick that backs Argentina", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 1, away_score: 0, penalty_winner_team_id: null, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const argMatch: MatchDTO = {
      ...baseMatch,
      id: 7,
      home: { id: 7, name: "Argentina", code: "ARG" },
      away: { id: 8, name: "Brazil", code: "BRA" },
    };
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={argMatch} />);

    // h=1, a=0 → Argentina wins → backs Argentina
    await user.click(screen.getByRole("button", { name: /increase argentina/i }));
    await user.click(screen.getByRole("button", { name: /save prediction/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(container.querySelector(".referee-approval")).toBeInTheDocument(),
    );
  });

  it("does not show the referee when Argentina loses on the scoreline", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 0, away_score: 1, penalty_winner_team_id: null, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const argMatch: MatchDTO = {
      ...baseMatch,
      id: 9,
      home: { id: 9, name: "Argentina", code: "ARG" },
      away: { id: 10, name: "Brazil", code: "BRA" },
    };
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={argMatch} />);

    // h=0, a=1 → Argentina (home) loses → no referee
    await user.click(screen.getByRole("button", { name: /increase brazil/i }));
    await user.click(screen.getByRole("button", { name: /save prediction/i }));

    await screen.findByText("Saved"); // onSuccess has run
    expect(container.querySelector(".referee-approval")).toBeNull();
  });

  it("does not show the referee for a non-Argentina match", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 0, away_score: 0, penalty_winner_team_id: null, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={baseMatch} />); // Mexico vs South Africa

    await user.click(screen.getByRole("button", { name: /save prediction/i }));

    await screen.findByText("Saved");
    expect(container.querySelector(".referee-approval")).toBeNull();
  });
```

- [ ] **Step 2: Run the tests and verify the first FAILS.**

Run: `cd frontend && pnpm vitest run src/components/MatchCard.test.tsx`
Expected: the "backs Argentina" test FAILS (`.referee-approval` never found — `Unable to find element`); the two negative tests PASS (element correctly absent).

- [ ] **Step 3: Commit the failing test.**

```bash
git add frontend/src/components/MatchCard.test.tsx
git commit -m "test: expect referee approval only when a save backs Argentina"
```

---

### Task 2: Implement the RefereeApproval overlay and wire it in

**Files:**
- Create: `frontend/src/components/RefereeApproval.tsx`
- Modify: `frontend/src/components/MatchCard.tsx` (import at line 8; state near line 272; `onSuccess` at lines 325-328; mount after the `GoalBallAnimation` block ~line 353)
- Modify: `frontend/src/styles/v2-components.css` (append after the `.goal-var-stamp__main` rule)

**Interfaces:**
- Consumes: `home`/`away` (`TeamDTO` with `.code`), the `h`/`a` score state, the `usePutPrediction` `onSuccess` callback.
- Produces: `RefereeApproval` — `export function RefereeApproval({ onDone }: { onDone: () => void })`, rendering `<div className="referee-approval">` (returns `null` under reduced motion). `MatchCard` owns `refereeKey` state and mounts it keyed.

- [ ] **Step 1: Create the overlay component.**

Create `frontend/src/components/RefereeApproval.tsx`:

```tsx
import { useEffect, useRef } from "react";

const APPROVAL_MS = 1900;

/**
 * Visual-only Easter egg: a referee slides in and gives a thumbs-up.
 * Self-timing — calls onDone after the animation so the parent can unmount it.
 * Renders nothing under reduced motion; hides itself if /referee.png is missing.
 */
export function RefereeApproval({ onDone }: { onDone: () => void }) {
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  const reduceMotion =
    typeof window !== "undefined" &&
    !!window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;

  useEffect(() => {
    if (reduceMotion) {
      onDoneRef.current();
      return;
    }
    const timer = window.setTimeout(() => onDoneRef.current(), APPROVAL_MS);
    return () => window.clearTimeout(timer);
  }, [reduceMotion]);

  if (reduceMotion) return null;

  return (
    <div className="referee-approval" aria-hidden="true">
      <img
        className="referee-approval__img"
        src="/referee.png"
        alt=""
        onError={() => onDoneRef.current()}
      />
    </div>
  );
}
```

- [ ] **Step 2: Import it in `MatchCard.tsx`.**

After the existing line 8 (`import type { GoalBallAnimationHandle } from "./GoalBallAnimation";`), add:

```tsx
import { RefereeApproval } from "./RefereeApproval";
```

- [ ] **Step 3: Add the `refereeKey` state.**

In `MatchCard`, after the existing `const [saved, setSaved] = useState(false);` (line 272), add:

```tsx
  const [refereeKey, setRefereeKey] = useState(0);
```

- [ ] **Step 4: Trigger it in `onSuccess`.**

Replace the existing `onSuccess` block (lines 325-328):

```tsx
        onSuccess: () => {
          setSaved(true);
          // The useEffect on `saved` handles clearing after 1600 ms (with unmount cleanup)
        },
```

with:

```tsx
        onSuccess: () => {
          setSaved(true);
          // The useEffect on `saved` handles clearing after 1600 ms (with unmount cleanup)
          const argIsHome = home.code === "ARG";
          const isArgentinaMatch = argIsHome || away.code === "ARG";
          const backsArgentina = isArgentinaMatch && (argIsHome ? h >= a : a >= h);
          if (backsArgentina) setRefereeKey((key) => key + 1);
        },
```

- [ ] **Step 5: Mount the overlay inside the card.**

Immediately after the closing `/>` of the `<GoalBallAnimation ... />` element (the block ending around line 353), add:

```tsx
        {refereeKey > 0 && (
          <RefereeApproval key={refereeKey} onDone={() => setRefereeKey(0)} />
        )}
```

- [ ] **Step 6: Add the styles.**

In `frontend/src/styles/v2-components.css`, after the `.goal-var-stamp__main { ... }` line (added by the VAR-stamp change), append:

```css
.referee-approval {
  position: absolute;
  right: 10px;
  bottom: 10px;
  z-index: 7;
  pointer-events: none;
  animation: referee-approve 1.9s ease-in-out forwards;
}
.referee-approval__img {
  display: block;
  width: auto;
  height: 96px;
  max-width: 45%;
  filter: drop-shadow(0 6px 14px rgba(0, 0, 0, 0.45));
}
@keyframes referee-approve {
  0%   { opacity: 0; transform: translate(24px, 24px) scale(0.8); }
  15%  { opacity: 1; transform: translate(0, 0) scale(1); }
  25%  { transform: translate(0, -4px) scale(1.02); }
  35%  { transform: translate(0, 0) scale(1); }
  80%  { opacity: 1; transform: translate(0, 0) scale(1); }
  100% { opacity: 0; transform: translate(24px, 24px) scale(0.9); }
}
```

- [ ] **Step 7: Run the tests — all three PASS.**

Run: `cd frontend && pnpm vitest run src/components/MatchCard.test.tsx`
Expected: PASS (the "backs Argentina" test now finds `.referee-approval`; negatives still absent).

- [ ] **Step 8: Type-check.**

Run: `cd frontend && pnpm tsc --noEmit`
Expected: no errors.

- [ ] **Step 9: Add your referee image, then manual verify.**

Save your troll cut-out to `frontend/public/referee.png`. Run `pnpm dev`, open the **Argentina vs Brazil** test match (match id 105), set a scoreline where Argentina wins or draws (e.g. tap **Argentina +** once), and **Save**. Confirm the referee slides in bottom-right, bobs, and slides out. Set a scoreline where Argentina loses and save — confirm **no** referee. (Before the PNG exists, saving shows only the normal "✓ Saved" — nothing broken.)

- [ ] **Step 10: Commit.**

```bash
git add frontend/src/components/RefereeApproval.tsx frontend/src/components/MatchCard.tsx frontend/src/styles/v2-components.css
git commit -m "feat: referee thumbs-up when a save backs Argentina"
```

---

### Task 3: Sync the spec

**Files:**
- Modify: `docs/REQUIREMENTS.md` (near the goal-feedback Easter egg, around line 41)

**Interfaces:**
- Consumes: nothing.
- Produces: spec text describing the referee reaction (keeps the automated PR spec check green).

- [ ] **Step 1: Add the Easter-egg bullet.**

In `docs/REQUIREMENTS.md`, immediately after the bullet that starts "Incrementing a team score gives lightweight goal feedback…" (the one ending "…never changes score values, validation, locking, or save behavior."), add a new bullet:

```text
- As a companion visual-only Easter egg, saving a prediction that backs Argentina (Argentina winning or drawing on the saved scoreline) in an Argentina match briefly slides in a referee "approved" thumbs-up over the card; it uses a user-supplied image (`frontend/public/referee.png`), is skipped under reduced motion, hides itself if the image is absent, and never changes save, lock, or score behavior.
```

- [ ] **Step 2: Commit.**

```bash
git add docs/REQUIREMENTS.md
git commit -m "docs: spec — referee approval Easter egg for backing Argentina"
```

---

## Self-Review

- **Spec coverage:** trigger both-conditions (Task 2 Step 4); overlay slide-in/bob/out + self-timing (Task 2 Steps 1, 6); user-supplied `/referee.png` (Task 2 Step 1 `src`, Step 9); graceful fallback on error (Task 2 Step 1 `onError`); reduced-motion null (Task 2 Step 1); decorative aria-hidden / pointer-events none (Task 2 Steps 1, 6); tests for the three cases (Task 1); spec sync (Task 3). All covered.
- **Placeholder scan:** none — every step has literal code/commands.
- **Type consistency:** `RefereeApproval({ onDone }: { onDone: () => void })` defined (Task 2 Step 1) and mounted with `onDone={() => setRefereeKey(0)}` (Step 5). `refereeKey: number` via `useState(0)` (Step 3), bumped with `setRefereeKey((key) => key + 1)` (Step 4), reset to `0` (Step 5), gate `refereeKey > 0` (Step 5). Test hook `.referee-approval` matches the CSS class and the component's `className` (Steps 1, 6). Increment button names `/increase argentina|brazil/i` and the `"Saved"` flash text match existing `MatchCard` behavior.
