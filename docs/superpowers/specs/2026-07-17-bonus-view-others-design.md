# Design — View other users' Tournament Bonus picks (after lock)

**Date:** 2026-07-17
**Scope:** One vertical slice — a new read-only backend endpoint + a user selector in the
Bonus panel. No change to bonus scoring, the lock time, or write behavior.

## Motivation

Match predictions are already revealed to everyone once a match locks at kickoff
(`GET /api/matches/{id}/predictions`, 403 before kickoff — spec §4). Tournament Bonus picks
have no equivalent: you can only ever see your own. Once bonus picks lock at `BONUS_LOCK_AT`
there is no privacy reason to keep them hidden, and players want to compare.

## Behavior

- When the Bonus panel is **expanded** and picks are **locked**, a **"Viewing"** user selector
  appears at the top of the panel body.
- The selector lists **"You"** plus every other user **who has set at least one bonus pick**
  (users with no picks are omitted). Default selection is **You**.
- Selecting a user renders **that user's picks in the same 7 category rows**, read-only.
- **Before lock, the selector does not render at all**, and the client never requests others'
  picks (the server would 403 anyway).
- The panel **header summary always shows your own** `earnedPts` / `setPicked`, regardless of
  who is selected — so a collapsed panel never misreports your score.

## Privacy rule

Others' bonus picks are revealed **only once `now >= BONUS_LOCK_AT`** — the same boundary as
the bonus write lock, mirroring the kickoff boundary used for match predictions (§4). The
**server is authoritative**: the endpoint returns **403** while `now().Before(lock)`, whatever
the client believes.

## Backend

1. **New sqlc query** — `backend/internal/store/queries/bonus.sql`:

   ```sql
   -- name: ListBonusPredictionsWithUsers :many
   SELECT bp.user_id, u.name, u.avatar_url, bp.category, bp.ref_id, bp.points
   FROM bonus_predictions bp
   JOIN users u ON u.id = bp.user_id
   ORDER BY u.name, bp.category;
   ```

   The inner join naturally yields only users who have picks. Regenerate with `make sqlc`;
   the generated code in `internal/store/sqlc/` is authoritative for names/types.

2. **New handler** `GetAllBonusPredictions` — `backend/internal/httpapi/bonus_handler.go`:
   - resolve lock via the existing `d.bonusLockAt(r)`; on failure → 500.
   - **privacy gate:** `if now().Before(lock)` → **403** `"bonus picks are hidden until lock"`.
   - load rows, group by `user_id` (preserving the query's name ordering), resolve each pick's
     display label with the existing `d.resolveRefLabel(r, cat, refID)`, and set `is_me` by
     comparing to the caller's ID.
   - respond `200` with `[]bonusUserPicksDTO`:
     `{ user_id, name, avatar_url, is_me, picks: []bonusPickDTO }`, reusing the existing
     `bonusPickDTO` shape (`category`, `ref_type`, `ref_id`, `label`, `points`).

3. **Route** — `backend/internal/httpapi/router.go`, alongside the existing bonus routes:
   `priv.Get("/bonus/predictions", d.GetAllBonusPredictions)` (authenticated, non-admin).

## Frontend

4. **`frontend/src/lib/bonus.ts`** — add:
   - `export type BonusUserPicks = { user_id: number; name: string; avatar_url: string | null; is_me: boolean; picks: BonusPick[] }`
   - `getAllBonusPredictions(): Promise<BonusUserPicks[]>` → `GET ${BASE}/bonus/predictions`, `credentials: "include"`.
   - `useAllBonusPredictions(enabled: boolean)` → `useQuery({ queryKey: ["bonus-predictions"], enabled })`.
     Callers pass `enabled=false` before lock to avoid a guaranteed-403 request — the same
     approach as `useMatchPredictions`.

5. **New component `frontend/src/components/BonusUserSelect.tsx`** — a compact dropdown
   listing "You" + the other users, reusing the existing `useDropdownPortalPosition` hook (no
   duplicated positioning logic) and following `TeamSelect`'s button/menu/outside-click
   pattern. Keeping it in its own file avoids growing `BonusPanel.tsx` (already 358 lines).
   Props: `{ users: BonusUserPicks[]; selectedUserId: number | null; onSelect: (id: number | null) => void }`
   (`null` = You).

6. **`frontend/src/components/BonusPanel.tsx`:**
   - add `const [viewUserId, setViewUserId] = useState<number | null>(null)`.
   - `const { data: allPicks = [] } = useAllBonusPredictions(locked && open)`.
   - `const viewingOther = viewUserId !== null`.
   - viewed picks: `viewingOther ? (allPicks.find((u) => u.user_id === viewUserId)?.picks ?? []) : (bonus?.picks ?? [])`; build the existing `pickMap` from these.
   - render `<BonusUserSelect>` at the top of the body **only when `locked`**.
   - pass `disabled={isDisabled || viewingOther}` to the selects (defensive — they are already
     disabled whenever `locked`, and the selector only exists when locked).
   - ignore `optimisticLabels` when `viewingOther` (those are your own in-flight edits).
   - header stats (`setPicked`, `earnedPts`) keep using `bonus.picks` — unchanged.

## Testing (TDD)

**Backend** (`bonus_handler_test.go`, table-driven, following existing helpers):
- 403 while `now()` is before `BONUS_LOCK_AT`.
- 200 after lock: picks grouped per user, labels resolved, `is_me` true only for the caller.
- Users with no bonus picks are absent from the response.

**Frontend** (`BonusPanel.test.tsx`):
- selector is **not** rendered when `locked` is false (and no request is made).
- selector **is** rendered when `locked` is true.
- selecting another user swaps the 7 rows to that user's picks; selecting "You" restores yours.

## Out of scope

- No editing others' picks; no per-user points badge in the selector (the leaderboard already
  shows totals); no change to bonus scoring, categories, or `BONUS_LOCK_AT`.
- No change to match-prediction reveal behavior.

## Spec sync

Update `docs/REQUIREMENTS.md` §4 (privacy) to record that Tournament Bonus picks are revealed
to all users once `BONUS_LOCK_AT` passes, viewable read-only via the Bonus panel's user
selector, and that the server returns 403 before that boundary.
