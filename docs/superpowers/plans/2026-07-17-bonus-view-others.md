# View Others' Tournament Bonus Picks (After Lock) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After Tournament Bonus picks lock at `BONUS_LOCK_AT`, let a user expand the Bonus panel, pick another player from a dropdown, and see that player's picks read-only in the same 7 category rows.

**Architecture:** A new read-only endpoint `GET /api/bonus/predictions` mirrors the existing `GET /api/matches/{id}/predictions` privacy pattern — server-authoritative **403 before the lock boundary**, full reveal after. It is backed by a new sqlc query that inner-joins `bonus_predictions` to `users` (so only users with picks appear). The frontend adds a `useAllBonusPredictions(enabled)` hook (disabled before lock, so no guaranteed-403 request), a small `BonusUserSelect` dropdown reusing the existing `useDropdownPortalPosition` hook, and rebuilds `BonusPanel`'s existing `pickMap` from the viewed user's picks.

**Tech Stack:** Go 1.22 + chi + sqlc (MySQL 8) on the backend; React 18 + TypeScript + TanStack Query + Vitest/Testing-Library on the frontend.

## Global Constraints

- **Privacy boundary (spec §4):** others' bonus picks are revealed **only once `now >= BONUS_LOCK_AT`**. The server returns **403** while `now().Before(lock)`, regardless of client state.
- **Fail safe:** if the settings store can't be read, `bonusLockAt` returns `ok=false` and the handler must return **500** — never fall open. (Matches `TestGetBonus_SettingsErrorFailsSafe`.)
- **sqlc is authoritative:** edit SQL in `backend/internal/store/queries/`, then run `make sqlc`. Never hand-edit `backend/internal/store/sqlc/`. sqlc v1.31.1 is installed at `~/go/bin/sqlc`.
- **Read-only:** no editing others' picks; no change to bonus scoring, categories, or `BONUS_LOCK_AT`.
- **Header summary always shows YOUR picks** (`bonus.picks`), never the viewed user's.
- **`avatar_url` is `VARCHAR(1024) NOT NULL DEFAULT ''`** → Go `string`, TS `string` (never null).
- **TDD, frequent commits.** Conventional Commits enforced by the commit-msg hook.

---

### Task 1: Backend plumbing — sqlc query + store method

**Files:**
- Modify: `backend/internal/store/queries/bonus.sql` (append)
- Modify: `backend/internal/store/bonus.go` (add row type, interface method, `SQLStore` impl)
- Modify: `backend/internal/httpapi/bonus_handler_test.go` (`fakeBonusStore` must satisfy the widened interface)
- Generated (do not hand-edit): `backend/internal/store/sqlc/bonus.sql.go`

**Interfaces:**
- Consumes: existing `SQLStore.q`, the `store.BonusStore` interface.
- Produces: `store.BonusUserPickRow{UserID int64; Name string; AvatarURL string; Category string; RefID int64; Points *int64}` and `BonusStore.ListBonusPredictionsWithUsers(ctx context.Context) ([]BonusUserPickRow, error)` — consumed by Task 2's handler.

- [ ] **Step 1: Add the query.**

Append to `backend/internal/store/queries/bonus.sql`:

```sql
-- name: ListBonusPredictionsWithUsers :many
-- Every user's bonus picks with the player's name/avatar. Used to reveal others'
-- picks once bonus locks at BONUS_LOCK_AT (privacy, spec §4). The INNER JOIN means
-- only users who actually set picks appear. Alphabetical by player, then category.
SELECT
    u.id AS user_id, u.name, u.avatar_url,
    bp.category, bp.ref_id, bp.points
FROM bonus_predictions bp
JOIN users u ON u.id = bp.user_id
ORDER BY u.name, bp.category;
```

- [ ] **Step 2: Regenerate sqlc.**

Run: `make sqlc`
Expected: exits 0; `backend/internal/store/sqlc/bonus.sql.go` now contains `ListBonusPredictionsWithUsers` and a `ListBonusPredictionsWithUsersRow` struct.

- [ ] **Step 3: Add the row type and interface method.**

In `backend/internal/store/bonus.go`, after the `BonusPredictionRow` struct (around line 29), add:

```go
// BonusUserPickRow is one user's single bonus pick alongside that player's
// identity. Used to reveal all users' bonus picks after BONUS_LOCK_AT (spec §4).
type BonusUserPickRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Category  string
	RefID     int64
	Points    *int64
}
```

Then in the `BonusStore` interface, after the `ListBonusPredictionsForUser` line, add:

```go
	ListBonusPredictionsWithUsers(ctx context.Context) ([]BonusUserPickRow, error)
```

- [ ] **Step 4: Implement it on `SQLStore`.**

In `backend/internal/store/bonus.go`, after `ListBonusPredictionsForUser` (ends ~line 96), add:

```go
func (s *SQLStore) ListBonusPredictionsWithUsers(ctx context.Context) ([]BonusUserPickRow, error) {
	rows, err := s.q.ListBonusPredictionsWithUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list bonus predictions with users: %w", err)
	}
	out := make([]BonusUserPickRow, 0, len(rows))
	for _, r := range rows {
		row := BonusUserPickRow{
			UserID:    r.UserID,
			Name:      r.Name,
			AvatarURL: r.AvatarUrl,
			Category:  string(r.Category),
			RefID:     r.RefID,
		}
		if r.Points.Valid {
			v := int64(r.Points.Int32)
			row.Points = &v
		}
		out = append(out, row)
	}
	return out, nil
}
```

Note: the generated field names follow the existing convention (`r.AvatarUrl`, `r.RefID`, `r.Points` as `sql.NullInt32`, `r.Category` as an enum needing `string(...)`) — exactly as `ListMatchPredictionsWithUsers` and `ListAllBonusPredictions` already do. `go build` will surface any mismatch; adapt to the generated names, not the other way round.

- [ ] **Step 5: Satisfy the interface in the test fake.**

In `backend/internal/httpapi/bonus_handler_test.go`, add a field to `fakeBonusStore` (after `results []store.BonusResult`, ~line 54):

```go
	allPicks []store.BonusUserPickRow
```

and add the method after `ListBonusPredictionsForUser` (~line 75):

```go
func (f *fakeBonusStore) ListBonusPredictionsWithUsers(context.Context) ([]store.BonusUserPickRow, error) {
	return f.allPicks, nil
}
```

- [ ] **Step 6: Verify the backend still builds and all tests pass.**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`
Expected: build + vet clean; all existing tests PASS (no behavior added yet).

- [ ] **Step 7: Commit.**

```bash
git add backend/internal/store/queries/bonus.sql backend/internal/store/bonus.go backend/internal/store/sqlc/ backend/internal/httpapi/bonus_handler_test.go
git commit -m "feat(store): query all users' bonus picks with user identity"
```

---

### Task 2: Backend — lock-gated `GET /api/bonus/predictions`

**Files:**
- Modify: `backend/internal/httpapi/bonus_handler.go` (add DTO + handler)
- Modify: `backend/internal/httpapi/router.go` (register the route, next to the existing bonus routes ~line 55)
- Test: `backend/internal/httpapi/bonus_handler_test.go`

**Interfaces:**
- Consumes: `store.BonusUserPickRow` + `BonusStore.ListBonusPredictionsWithUsers` (Task 1); existing `d.bonusLockAt(r)`, `d.resolveRefLabel(r, cat, refID)`, `userFromContext`, `now()`, `writeJSON`, `writeError`, `bonusPickDTO`.
- Produces: `GET /api/bonus/predictions` → `200` `[]bonusUserPicksDTO` (`{user_id, name, avatar_url, is_me, picks[]}`) or `403` before lock. Consumed by Task 3's frontend.

- [ ] **Step 1: Write the failing tests.**

Append to `backend/internal/httpapi/bonus_handler_test.go`:

```go
func TestGetAllBonusPredictions_BeforeLockForbidden(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{allPicks: []store.BonusUserPickRow{
		{UserID: 1, Name: "Ana", Category: "winner", RefID: 9},
	}}
	d := &Deps{Bonus: st, Players: &fakePlayerStore{}, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus/predictions", nil), 1)
	rec := httptest.NewRecorder()
	d.GetAllBonusPredictions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 before lock", rec.Code)
	}
}

func TestGetAllBonusPredictions_AtExactLockBoundaryRevealed(t *testing.T) {
	old := now
	lockAt := time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)
	now = func() time.Time { return lockAt } // now == lock → revealed (same boundary as the write lock)
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{allPicks: []store.BonusUserPickRow{
		{UserID: 1, Name: "Ana", Category: "winner", RefID: 9},
	}}
	d := &Deps{
		Bonus:    st,
		Players:  &fakePlayerStore{teamNames: map[int64]string{9: "Brazil"}},
		Settings: &fakeSettings{lockAt: lockAt},
	}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus/predictions", nil), 1)
	rec := httptest.NewRecorder()
	d.GetAllBonusPredictions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 at exact lock boundary", rec.Code)
	}
}

func TestGetAllBonusPredictions_SettingsErrorFailsSafe(t *testing.T) {
	d := &Deps{Bonus: &fakeBonusStore{}, Players: &fakePlayerStore{}, Settings: &fakeSettings{lockErr: errors.New("settings down")}}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus/predictions", nil), 1)
	rec := httptest.NewRecorder()
	d.GetAllBonusPredictions(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (must not fall open on a settings error)", rec.Code)
	}
}

func TestGetAllBonusPredictions_AfterLockGroupsByUser(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{allPicks: []store.BonusUserPickRow{
		{UserID: 1, Name: "Ana", Category: "winner", RefID: 9},
		{UserID: 1, Name: "Ana", Category: "golden_boot", RefID: 42},
		{UserID: 2, Name: "Bob", Category: "winner", RefID: 7},
	}}
	d := &Deps{
		Bonus: st,
		Players: &fakePlayerStore{
			teamNames:   map[int64]string{9: "Brazil", 7: "France"},
			playerNames: map[int64]string{42: "Mbappe"},
		},
		Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)},
	}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus/predictions", nil), 2)
	rec := httptest.NewRecorder()
	d.GetAllBonusPredictions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []bonusUserPicksDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("users = %d, want 2 grouped users", len(got))
	}
	if got[0].UserID != 1 || len(got[0].Picks) != 2 {
		t.Fatalf("first user = %+v, want user 1 with 2 picks", got[0])
	}
	if got[0].IsMe {
		t.Error("user 1 must not be is_me when the caller is user 2")
	}
	if !got[1].IsMe {
		t.Error("user 2 must be is_me when the caller is user 2")
	}
	if got[0].Picks[0].Label != "Brazil" {
		t.Errorf("label = %q, want %q", got[0].Picks[0].Label, "Brazil")
	}
	if got[0].Picks[0].RefType != "team" || got[0].Picks[1].RefType != "player" {
		t.Errorf("ref types = %q/%q, want team/player", got[0].Picks[0].RefType, got[0].Picks[1].RefType)
	}
}
```

- [ ] **Step 2: Run the tests to verify they FAIL.**

Run: `cd backend && go test ./internal/httpapi/ -run TestGetAllBonusPredictions`
Expected: FAIL — compile error `d.GetAllBonusPredictions undefined` and `undefined: bonusUserPicksDTO`.

- [ ] **Step 3: Implement the DTO + handler.**

In `backend/internal/httpapi/bonus_handler.go`, after the `bonusResponse` struct (~line 37), add:

```go
type bonusUserPicksDTO struct {
	UserID    int64          `json:"user_id"`
	Name      string         `json:"name"`
	AvatarURL string         `json:"avatar_url"`
	IsMe      bool           `json:"is_me"`
	Picks     []bonusPickDTO `json:"picks"`
}

// GetAllBonusPredictions reveals every player's tournament-bonus picks — but only
// once picks lock at BONUS_LOCK_AT. Before that boundary it returns 403, mirroring
// the kickoff gate on match predictions (privacy, spec §4). Read-only.
func (d *Deps) GetAllBonusPredictions(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	lock, ok := d.bonusLockAt(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "settings unavailable")
		return
	}
	// Privacy gate: hidden until picks lock — same boundary as the write lock.
	if now().Before(lock) {
		writeError(w, http.StatusForbidden, "bonus picks are hidden until lock")
		return
	}

	rows, err := d.Bonus.ListBonusPredictionsWithUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bonus picks")
		return
	}

	out := make([]bonusUserPicksDTO, 0)
	index := make(map[int64]int, len(rows))
	for _, row := range rows {
		i, seen := index[row.UserID]
		if !seen {
			out = append(out, bonusUserPicksDTO{
				UserID:    row.UserID,
				Name:      row.Name,
				AvatarURL: row.AvatarURL,
				IsMe:      row.UserID == u.ID,
				Picks:     []bonusPickDTO{},
			})
			i = len(out) - 1
			index[row.UserID] = i
		}
		cat := bonus.Category(row.Category)
		out[i].Picks = append(out[i].Picks, bonusPickDTO{
			Category: row.Category,
			RefType:  string(bonus.RefTypeOf(cat)),
			RefID:    row.RefID,
			Label:    d.resolveRefLabel(r, cat, row.RefID),
			Points:   row.Points,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Register the route.**

In `backend/internal/httpapi/router.go`, directly after the existing `priv.Get("/bonus", d.GetBonus)` line, add:

```go
			priv.Get("/bonus/predictions", d.GetAllBonusPredictions)
```

- [ ] **Step 5: Run the tests to verify they PASS.**

Run: `cd backend && go test ./internal/httpapi/ -run TestGetAllBonusPredictions -v`
Expected: all four tests PASS.

- [ ] **Step 6: Run the full backend suite.**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 7: Commit.**

```bash
git add backend/internal/httpapi/bonus_handler.go backend/internal/httpapi/router.go backend/internal/httpapi/bonus_handler_test.go
git commit -m "feat(api): reveal all users' bonus picks after lock via GET /bonus/predictions"
```

---

### Task 3: Frontend — user selector in the Bonus panel

**Files:**
- Modify: `frontend/src/lib/bonus.ts` (type + fetcher + hook)
- Create: `frontend/src/components/BonusUserSelect.tsx`
- Modify: `frontend/src/components/BonusPanel.tsx`
- Modify: `frontend/src/styles/v2-components.css` (append after the bonus select rules, ~line 271)
- Test: `frontend/src/components/BonusPanel.test.tsx`

**Interfaces:**
- Consumes: `GET /api/bonus/predictions` (Task 2); existing `useDropdownPortalPosition(open, ref, width)`, `ChevronIcon`, `CheckIcon`, `BonusPick`.
- Produces: `BonusUserPicks` type, `useAllBonusPredictions(enabled: boolean)`, and `BonusUserSelect({ users, selectedUserId, onSelect })`.

- [ ] **Step 1: Write the failing tests.**

In `frontend/src/components/BonusPanel.test.tsx`, add `useAllBonusPredictions` to the mock factory — change the `vi.mock("../lib/bonus", ...)` return block to include it:

```tsx
    useBonus: vi.fn(),
    useTeams: vi.fn(),
    usePlayerSearch: vi.fn(),
    useSaveBonus: vi.fn(),
    useAllBonusPredictions: vi.fn(),
```

and extend the import line below it:

```tsx
import { useBonus, useTeams, usePlayerSearch, useSaveBonus, useAllBonusPredictions } from "../lib/bonus";
```

Add this fixture next to `lockedBonus` (~line 42):

```tsx
const allBonusPicks: import("../lib/bonus").BonusUserPicks[] = [
  {
    user_id: 1, name: "You", avatar_url: "", is_me: true,
    picks: [{ category: "winner", ref_type: "team", ref_id: 1, label: "Brazil (BRA)" }],
  },
  {
    user_id: 2, name: "Kiran", avatar_url: "", is_me: false,
    picks: [{ category: "winner", ref_type: "team", ref_id: 2, label: "Argentina (ARG)" }],
  },
];
```

In `beforeEach`, add a default so every existing test keeps working:

```tsx
    (useAllBonusPredictions as ReturnType<typeof vi.fn>).mockReturnValue({ data: [], isLoading: false });
```

Then add the three tests inside `describe("BonusPanel", ...)`:

```tsx
  // ── View others' picks (locked only) ──────────────────────────────────────
  it("does not show the user selector before lock", async () => {
    (useBonus as ReturnType<typeof vi.fn>).mockReturnValue({ data: unlocked, isLoading: false, isError: false });
    const user = userEvent.setup();
    wrap(<BonusPanel />);
    await user.click(screen.getByRole("button", { name: /set tournament bonus picks/i }));
    expect(screen.queryByRole("button", { name: /view bonus picks by user/i })).toBeNull();
  });

  it("shows the user selector once picks are locked", async () => {
    (useBonus as ReturnType<typeof vi.fn>).mockReturnValue({ data: lockedBonus, isLoading: false, isError: false });
    (useAllBonusPredictions as ReturnType<typeof vi.fn>).mockReturnValue({ data: allBonusPicks, isLoading: false });
    const user = userEvent.setup();
    wrap(<BonusPanel />);
    await user.click(screen.getByRole("button", { name: /set tournament bonus picks/i }));
    expect(screen.getByRole("button", { name: /view bonus picks by user/i })).toBeInTheDocument();
  });

  it("swaps the category rows to the selected user's picks", async () => {
    (useBonus as ReturnType<typeof vi.fn>).mockReturnValue({ data: lockedBonus, isLoading: false, isError: false });
    (useAllBonusPredictions as ReturnType<typeof vi.fn>).mockReturnValue({ data: allBonusPicks, isLoading: false });
    const user = userEvent.setup();
    wrap(<BonusPanel />);
    await user.click(screen.getByRole("button", { name: /set tournament bonus picks/i }));

    // Default view is "You" → your own winner pick (Brazil).
    expect(screen.getByRole("button", { name: /select team for world cup winner/i })).toHaveTextContent("Brazil");

    await user.click(screen.getByRole("button", { name: /view bonus picks by user/i }));
    await user.click(screen.getByRole("menuitem", { name: /kiran/i }));

    // Now showing Kiran's winner pick (Argentina).
    expect(screen.getByRole("button", { name: /select team for world cup winner/i })).toHaveTextContent("Argentina");
  });
```

- [ ] **Step 2: Run the tests to verify they FAIL.**

Run: `cd frontend && pnpm vitest run src/components/BonusPanel.test.tsx`
Expected: FAIL — `useAllBonusPredictions` is not exported from `../lib/bonus`, and no "view bonus picks by user" button exists.

- [ ] **Step 3: Add the type, fetcher, and hook.**

In `frontend/src/lib/bonus.ts`, after the `BonusResponse` type (~line 37), add:

```ts
export type BonusUserPicks = {
  user_id: number;
  name: string;
  avatar_url: string;
  is_me: boolean;
  picks: BonusPick[];
};
```

and at the end of the file add:

```ts
export async function getAllBonusPredictions(): Promise<BonusUserPicks[]> {
  const res = await fetch(`${BASE}/bonus/predictions`, { credentials: "include" });
  if (!res.ok) throw new Error(`bonus predictions failed: ${res.status}`);
  return res.json();
}

// Others' bonus picks are only served once picks lock (403 before). Pass
// enabled=false before lock to avoid a guaranteed-403 request.
export function useAllBonusPredictions(enabled: boolean) {
  return useQuery({
    queryKey: ["bonus-predictions"],
    queryFn: getAllBonusPredictions,
    enabled,
  });
}
```

- [ ] **Step 4: Create the selector component.**

Create `frontend/src/components/BonusUserSelect.tsx`:

```tsx
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
```

- [ ] **Step 5: Wire it into `BonusPanel`.**

In `frontend/src/components/BonusPanel.tsx`:

(a) Extend the `../lib/bonus` import to add the hook and type:

```tsx
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
```

(b) Add the component import after the `PlayerCombobox` import (line 12):

```tsx
import { BonusUserSelect } from "./BonusUserSelect";
```

(c) Add state + query directly after the `optimisticLabels` line (~line 197) — **before** the `if (isLoading || teamsLoading)` early return, so hooks stay unconditional:

```tsx
  const [viewUserId, setViewUserId] = useState<number | null>(null);
  // Only fetch once locked+open: the server 403s before the lock boundary.
  const { data: allPicks = [] } = useAllBonusPredictions(locked && open);
```

(d) Replace the existing `pickMap` construction (lines 217-219):

```tsx
  const pickMap = new Map<string, BonusPick>(
    (bonus?.picks ?? []).map((p) => [p.category, p]),
  );
```

with:

```tsx
  const viewingOther = viewUserId !== null;
  const viewedPicks = viewingOther
    ? (allPicks.find((u) => u.user_id === viewUserId)?.picks ?? [])
    : (bonus?.picks ?? []);
  const pickMap = new Map<string, BonusPick>(
    viewedPicks.map((p) => [p.category, p]),
  );
```

Leave `setPicked` / `earnedPts` (lines 221-224) untouched — the header always reports **your** picks.

(e) Render the selector at the top of the collapsible body. Change:

```tsx
        <div className="bonus-body" id="bonus-body">
          <div className="bonus-grid" role="list" aria-label="Tournament Bonus categories">
```

to:

```tsx
        <div className="bonus-body" id="bonus-body">
          {locked && (
            <BonusUserSelect
              users={allPicks}
              selectedUserId={viewUserId}
              onSelect={setViewUserId}
            />
          )}
          <div className="bonus-grid" role="list" aria-label="Tournament Bonus categories">
```

(f) Make the selects inert while viewing someone else. Change the `TeamSelect` prop `disabled={isDisabled}` to:

```tsx
                      disabled={isDisabled || viewingOther}
```

and the `PlayerCombobox` props `disabled={isDisabled}` / `currentLabel={optimisticLabels[cat.key] ?? pick?.label}` to:

```tsx
                        disabled={isDisabled || viewingOther}
                        currentRefId={pick?.ref_id}
                        currentLabel={viewingOther ? pick?.label : (optimisticLabels[cat.key] ?? pick?.label)}
```

- [ ] **Step 6: Add the styles.**

In `frontend/src/styles/v2-components.css`, after the `.bonus-select:hover, ... .bonus-sel-btn.open { ... }` rule (~line 271), append:

```css
.bonus-viewas { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.bonus-viewas-label { font-size: 12px; font-weight: 600; color: var(--text-3); text-transform: uppercase; letter-spacing: 0.04em; }
.bonus-viewas .bonus-sel-btn { width: 172px; flex: none; }
```

- [ ] **Step 7: Run the tests to verify they PASS.**

Run: `cd frontend && pnpm vitest run src/components/BonusPanel.test.tsx`
Expected: all PASS, including the three new tests and every pre-existing one.

- [ ] **Step 8: Lint + type-check + full suite.**

Run: `cd frontend && pnpm exec eslint src/components/BonusUserSelect.tsx src/components/BonusPanel.tsx src/lib/bonus.ts && pnpm tsc --noEmit && pnpm vitest run`
Expected: eslint clean, no type errors, all suites PASS.

- [ ] **Step 9: Commit.**

```bash
git add frontend/src/lib/bonus.ts frontend/src/components/BonusUserSelect.tsx frontend/src/components/BonusPanel.tsx frontend/src/components/BonusPanel.test.tsx frontend/src/styles/v2-components.css
git commit -m "feat(fe): view other players' bonus picks after lock"
```

---

### Task 4: Sync the spec

**Files:**
- Modify: `docs/REQUIREMENTS.md` (§4 privacy section)

**Interfaces:**
- Consumes: nothing.
- Produces: spec text matching shipped behavior (keeps the automated PR spec check green).

- [ ] **Step 1: Find the §4 privacy section.**

Run: `grep -n "hidden until" docs/REQUIREMENTS.md`
Expected: prints the §4 line(s) describing that others' predictions stay hidden until a match locks at kickoff.

- [ ] **Step 2: Add the bonus-reveal rule.**

Immediately after that §4 bullet about match predictions being revealed at kickoff, add:

```text
- Tournament Bonus picks follow the same rule at their own boundary: every user's bonus picks stay private until picks lock at `BONUS_LOCK_AT`, and are revealed to all users afterwards. Once locked, the Bonus panel shows a read-only "Viewing" selector listing you plus every other user who set picks; choosing a user shows their picks in the category rows. The server is authoritative — `GET /api/bonus/predictions` returns 403 before the lock boundary — and the reveal is read-only (no one can edit another user's picks).
```

- [ ] **Step 3: Commit.**

```bash
git add docs/REQUIREMENTS.md
git commit -m "docs: spec — bonus picks revealed to all users after BONUS_LOCK_AT"
```

---

## Self-Review

**Spec coverage:**
- New sqlc query (Task 1 Step 1) ✅; handler with 403 gate + grouping + labels + `is_me` (Task 2 Step 3) ✅; route (Task 2 Step 4) ✅; `BonusUserPicks` + `useAllBonusPredictions(enabled)` (Task 3 Step 3) ✅; `BonusUserSelect` reusing `useDropdownPortalPosition` (Task 3 Step 4) ✅; `viewUserId`, gated on `locked`, `pickMap` from viewed picks, header untouched, `optimisticLabels` ignored for others, selects disabled (Task 3 Step 5) ✅; frontend tests for hidden/shown/swap (Task 3 Step 1) ✅; spec sync (Task 4) ✅.
- **One deliberate deviation:** the spec listed a backend test for "users with no bonus picks are absent". That property comes from the SQL `INNER JOIN`, not the handler — a handler test with a fake store would only prove the fake returns what it was given, i.e. prove nothing. It is documented in the query comment (Task 1 Step 1) instead. Added in its place: an **exact-lock-boundary** test and a **settings-fail-safe 500** test, which guard the genuinely security-critical branches and match the existing `PutBonus` test suite's shape.
- Also added beyond spec: `avatar_url` corrected to non-null `string` (schema is `NOT NULL DEFAULT ''`), noted in Global Constraints.

**Placeholder scan:** none — every step has literal code or an exact command.

**Type consistency:** `store.BonusUserPickRow{UserID, Name, AvatarURL, Category, RefID, Points *int64}` defined (T1 S3), implemented (T1 S4), returned by the fake (T1 S5), consumed by the handler (T2 S3) — `row.Points (*int64)` feeds `bonusPickDTO.Points (*int64)` ✅. `ListBonusPredictionsWithUsers(ctx) ([]BonusUserPickRow, error)` identical in interface, impl, and fake ✅. `bonusUserPicksDTO` JSON (`user_id/name/avatar_url/is_me/picks`) matches TS `BonusUserPicks` exactly ✅. `BonusUserSelect({users, selectedUserId, onSelect})` defined (T3 S4) matches its call site (T3 S5e) ✅. The aria-label `"View bonus picks by user"` is identical in the component and all three tests ✅.
