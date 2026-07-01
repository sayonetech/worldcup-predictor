# Integration Guide — Chased by the GOAT

How to embed the **Chased by the GOAT** game bundle into a host application
(e.g. the SayScore World Cup app). The bundle is a single, self-contained ES
module that mounts the game into a DOM element you provide, and reports each run's
final distance back to you to persist. **The bundle never stores or sends anything
itself** — you own the data.

- **One import, one call:** `mountGoatGame(container, config) → handle`
- **Self-contained:** all CSS, fonts, and assets are inlined in one `.mjs` — no
  runtime dependencies, no network requests, no CDN.
- **Isolated:** renders only inside your container, scopes all listeners, and tears
  down cleanly via `handle.destroy()`. Multiple instances can coexist.

---

## 1. Install

ESM-only; not published to a registry (yet). Consume from a local checkout or a
packed tarball.

```bash
# pack a tarball from this repo
pnpm build && pnpm pack            # → chased-by-the-goat-1.0.0.tgz

# install it in the host app
pnpm add /path/to/chased-by-the-goat-1.0.0.tgz
```

The shipped `files` are `dist/chased-by-the-goat.mjs`, `types/index.d.ts`, this
guide, and the README. Resolution (`package.json#exports`):

```jsonc
"exports": { ".": { "types": "./types/index.d.ts", "default": "./dist/chased-by-the-goat.mjs" } }
```

```ts
import { mountGoatGame } from 'chased-by-the-goat';   // types resolve automatically
```

---

## 2. Quick start (vanilla)

```js
import { mountGoatGame } from 'chased-by-the-goat';

const game = mountGoatGame(document.getElementById('game'), {
  player: { id: 'emp_123', name: 'Renjith' },
  leaderboard: [
    { name: 'Ronaldo', team: 'Portugal',  distance: 2480 },
    { name: 'Messi',   team: 'Argentina', distance: 2120 },
  ],
  onGameEnd(result) {
    // result = { id, name, distance, coins, timestamp } — persist it server-side.
    // distance = pure metres; coins = this run's haul (pool it: pool += result.coins).
    api.saveScore(result);
  },
});

// later, when leaving the page:
game.destroy();
```

---

## 3. React (the SayScore host)

The `destroy()` contract maps onto React cleanup exactly — return it from the
effect so unmount (and Strict-Mode's dev double-invoke) never leaks an instance.

```tsx
import { useEffect, useRef } from 'react';
import { mountGoatGame, type GoatGameHandle, type GoatResult } from 'chased-by-the-goat';

export function GoatGame({
  player, leaderboard, onResult,
}: {
  player: { id: string; name: string };
  leaderboard: { name: string; team: string; distance: number }[];
  onResult: (r: GoatResult) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const handle = useRef<GoatGameHandle | null>(null);

  // mount once
  useEffect(() => {
    handle.current = mountGoatGame(ref.current!, { player, leaderboard, onGameEnd: onResult });
    return () => handle.current?.destroy();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // push the latest leaderboard whenever it changes — WITHOUT remounting (no intro
  // replay, no interruption). setLeaderboard keeps the player; it's the right call here.
  useEffect(() => {
    handle.current?.setLeaderboard(leaderboard);
  }, [leaderboard]);

  return <div ref={ref} style={{ width: '100%' }} />;
}
```

Then drive the refresh from your data layer (e.g. TanStack Query): save the score on
`onGameEnd`, invalidate the leaderboard query, and let the re-fetched data flow back in
through the `leaderboard` prop → the effect above calls `setLeaderboard`.

```tsx
const qc = useQueryClient();
const { data: board = [] } = useQuery({ queryKey: ['leaderboard'], queryFn: api.getLeaderboard });
const save = useMutation({
  mutationFn: api.saveScore,
  onSuccess: () => qc.invalidateQueries({ queryKey: ['leaderboard'] }), // re-fetch the changed board
});

<GoatGame player={me} leaderboard={board} onResult={(r) => save.mutate(r)} />
```

---

## 4. Configuration reference

```ts
mountGoatGame(container: HTMLElement, config: GoatConfig): GoatGameHandle
```

### `container` (required)
A DOM element you own. The game renders **inside** it and never touches anything
outside. The game fills the container's **width** and keeps a fixed **1840×600**
aspect ratio (letterboxed), scaling itself to fit — see §6 (Sizing).
Throws synchronously if `container` is not a DOM element.

### `config.player` (required)
```ts
{ id: string; name: string; coins?: number }
```
- `name` — shown in-game ("PLAYING AS …") and as the player's leaderboard row.
- `id` — **opaque**; the bundle never interprets it, only echoes it back in the
  result so you can attribute the score. Missing/empty `name` falls back to `"Player"`.
- `coins?` — **optional** the player's current **lifetime coin pool** (e.g. `150`). When
  supplied it's shown on the start/HUD/game-over ("Renjith · 150 🪙") and, at game-over,
  the run's haul is added on screen ("150 → 162 🪙"). **Display only** — read-only/echoed,
  never interpreted or mutated; a negative/NaN/non-number value is ignored (pool hidden).
  The bundle does not persist it — **you** keep the pool (see `onGameEnd.coins` below).

### `config.leaderboard` (optional)
```ts
Array<{ name: string; team: string; distance: number }>
```
- The **"Hall of the Chased"** board on the landing/game-over screens, **sorted by
  `distance` descending**, capped at **20 rows**. On game-over the player's just-finished
  run is inserted at its rank (gold-highlighted, marked unsaved — display only).
- **Omitted / empty / malformed → the board is hidden entirely** (no empty-state chrome) —
  symmetric with `coinLeaderboard`. Provide data to show it.
- `team` is rendered as a **country flag** in the leaderboard disc. **Valid teams**
  (case-insensitive; flags hardcoded): **Argentina, Brasil, Germany, Netharland,
  Spain, Portugal, France, Belgium** (standard spellings `Brazil`/`Netherlands`/
  `Holland` also accepted). Any other value renders a neutral 🏳️.
- Treated as **read-only** — the bundle never mutates your array or objects (it works on clones).

### `config.onGameEnd` (required)
```ts
(result: {
  id: string; name: string; distance: number; coins: number;
  durationMs: number; runToken?: string; timestamp: string;
}) => void
```
- Fires **exactly once per completed run** (never zero, never twice — including
  rapid input at the run-over boundary). "Run Again" starts a fresh run that fires
  its own single callback.
- `distance` is the final score in **pure metres survived** (integer). Collectible coins
  do **not** add to distance — they are reported separately as `coins`.
- `coins` is the integer number of coins **collected this run** (≥ 0) — a metric separate
  from `distance`. **You pool it:** after each run, `pool += result.coins`, persist the new
  total, and pass it back as `player.coins` on the next mount/`update()` to display it.
- `durationMs` is the run's length on the **active simulation clock** (integer ms) — the
  same timebase that advances distance, so it **excludes paused time** (tab-blur) and the
  intro/knockdown phases. Use it to validate the run server-side:
  `|distance − paceDistance(durationMs)| ≤ ε` (see §8).
- `runToken?` echoes `config.runToken` (or the last `handle.setRunToken`) **verbatim** — it is
  present only when a non-empty token was current when the run ended, and **omitted otherwise**.
  Opaque: the bundle never interprets it. Bind the result to your single-use token with it (see §8).
- `timestamp` is an **ISO-8601 string** (not a `Date`). `id`/`name` are echoed from `config.player`.
- The bundle does **not** persist anything (neither distance nor coins) — saving + pooling
  is entirely your responsibility.
- `coins`, `durationMs`, and `runToken` are **additive and backward-compatible** — existing
  hosts that destructure only the older fields are unaffected.
- Throws synchronously if `onGameEnd` is not a function.

### `config.coinLeaderboard` (optional)
```ts
Array<{ name: string; team?: string; coins: number }>
```
- A **second leaderboard** rendered **under** the distance board ("Hall of the Chased"),
  ranking players by **total coins** (the engagement board) — sorted by `coins` descending,
  capped at **20 rows**. `team` renders as the same country flag as the distance board.
- **Omitted / empty / malformed → the coins board is hidden entirely** (no regression for
  hosts that don't use it). Treated **read-only** — cloned, never mutated; coins clamped to a
  non-negative integer; malformed rows dropped (degrade-not-throw).
- This is the engagement counterpart of the distance board: you keep each player's lifetime
  pool (`pool += result.coins`, see `onGameEnd`), then pass the standings here. On game-over
  the player's **projected pool** (`player.coins` + the run's `coins`) is inserted at its rank.
- Refresh it live with `handle.setCoinLeaderboard(...)` (below).

### `config.runToken` (optional — anti-cheat)
```ts
runToken?: string
```
- An **opaque** host-supplied string the bundle **carries through and echoes back** in
  `result.runToken`. It is treated **exactly like `player.id`**: the bundle never interprets,
  parses, validates, transmits, renders, or logs it.
- **Missing / empty / non-string → ignored** (the result simply omits `runToken`; never throws).
- Intended use: your server issues a **signed, single-use** token and passes it in; the result
  echoes it so the server can **bind a result to a token**. After each run, submit the result,
  fetch a fresh token, and arm the next run with `handle.setRunToken(...)`. See §8.

---

## 5. Lifecycle / handle

```ts
interface GoatGameHandle {
  setLeaderboard(leaderboard: GoatLeaderboardEntry[]): void;          // refresh ONLY the distance board
  setCoinLeaderboard(coinLeaderboard: GoatCoinLeaderboardEntry[]): void; // refresh ONLY the coins board
  setPlayer(player: GoatPlayer): void;                               // refresh ONLY the player
  setRunToken(token: string): void;                                  // arm the next run's anti-cheat token
  update(config: GoatConfig): void;                                  // refresh all (full config)
  destroy(): void;                                                   // full teardown
}
```

- **`setCoinLeaderboard(coinLeaderboard)`** — the coins-board counterpart of `setLeaderboard`.
  Replaces only the stored coins board (keeps player + distance board + `onGameEnd`), re-renders
  in place on a start/over screen. Clones the array; an empty array hides the board. Never
  interrupts a run, replays the intro, or fires `onGameEnd`. Post-run flow: pool `result.coins`,
  re-fetch the coin standings, then `setCoinLeaderboard(fresh)`.

- **`setLeaderboard(leaderboard)`** — **the method you want for the post-run refresh.**
  Replaces only the stored leaderboard (keeps the current player + `onGameEnd`), and
  re-renders the board **in place** if the game-over or start screen is showing. Clones
  the array (never mutates yours). Never interrupts a run, replays the intro, or fires
  `onGameEnd`.
- **`setPlayer(player)`** — replaces only the `{ id, name }`, keeping the leaderboard.
- **`setRunToken(token)`** — replaces only the stored **anti-cheat token** (keeps player +
  leaderboards + `onGameEnd`). The token is **single-use**, so the post-run flow is: submit
  the result, fetch a fresh token, then `setRunToken(fresh)` to arm the next run. The token
  echoed in a result is **whichever is current when that run ends**. Like `setLeaderboard`,
  it never interrupts a run, replays the intro, or fires `onGameEnd`. An empty/non-string
  token clears it (the next result omits `runToken`). See §8.
- **`update(config)`** — replaces **both** player and leaderboard from a full config.
  ⚠️ Omitting `player` resets the name to `"Player"` — for a leaderboard-only refresh
  use `setLeaderboard()`.
- **`destroy()`** — removes the canvas, every event listener, the rAF loop, the
  `ResizeObserver`, and the registered fonts, leaving the container empty and
  re-mountable.

### Refreshing the leaderboard after every run (the common flow)

The game changes the standings every time it's played. The host owns the data, so the
loop is: **run ends → you save the score → you re-fetch the leaderboard → push it back.**
Do it inside `onGameEnd` so the game-over screen the player is looking at updates in place:

```js
const game = mountGoatGame(container, {
  player,
  leaderboard: await api.getLeaderboard(),
  async onGameEnd(result) {
    await api.saveScore(result);                  // persist the run
    const fresh = await api.getLeaderboard();     // re-fetch the now-changed board
    game.setLeaderboard(fresh);                   // refresh in place — no remount
  },
});
```

`setLeaderboard` is safe to call at any time: mid-run it just updates the stored board
and it shows on the next start/over screen; on the start or game-over screen it updates
immediately. It never disturbs an in-progress run.

---

## 6. Screens, controls & sizing

The bundle presents **two screens**, identical on web and mobile:

1. **Landing / leaderboard page** — a poster (title, "Playing as …", and after a run the result +
   roast + new best) with **Hall of the Chased** and the optional **coins board** below it, plus
   **Kick Off** / **Run Again**. This renders **inside your container** and is what shows before
   playing and after each run.
2. **Game screen** — pressing **Kick Off** takes over the **whole viewport** with just the game
   (titlebar + leaderboards hidden) and a **✕** close button. When a run ends (caught) it returns
   to the landing page with the updated standings; **Run Again** re-enters the game screen.

### Controls (same everywhere)
- **Tap / click anywhere = Jump.** **Press-and-hold the lower part of the play area = Duck**
  (release to stand). On desktop the **keyboard** also works: `Space`/`↑` jump, `↓` duck. A brief
  on-screen hint shows the two zones on the first run on touch devices.
- Start/restart is **button-only** — tapping the poster/background never starts a run.

### Going full-screen
- **Desktop / web:** the game screen is a **CSS full-viewport takeover** (no browser-fullscreen
  permission prompt); **✕** or **Esc** returns to the landing page.
- **Mobile (coarse-pointer / ≤820px):** Kick Off additionally requests the real **Fullscreen API** +
  **orientation-lock to landscape** where supported (Android Chrome). Where those are unavailable —
  notably **iPhone Safari** (no orientation-lock; Fullscreen API disabled on iPhone since iOS 17.4) —
  it falls back to a **faux-landscape 90° rotation** that fills the viewport.
- Everything reverses on **`destroy()`** (and on ✕ / Esc / system-back): fullscreen exited (only if
  this instance owns it), orientation unlocked, all presentation classes removed — **zero residue**,
  scoped to the bundle's own root (never `document.body`).

### Sizing the container
- The **landing** fills your container's width; the **game screen** fills the viewport. You only need
  to size the container:
  ```css
  .goat-host { width: 100%; max-width: 1100px; margin: 0 auto; }
  ```
- The playfield keeps a fixed **1840×600** logical space, is **DPR-aware** (crisp on retina), and is
  **frame-rate / resolution independent** — resizing or moving across displays never changes game speed
  or difficulty. Backgrounding the tab **pauses** the run and resumes cleanly (no tunneling, no
  corrupted distance).

### Embedding caveats
- **iframe opt-in (`allow="fullscreen"`):** to let real fullscreen engage inside an `<iframe>`, add
  `allow="fullscreen"`. Same-origin iframes already permit it; cross-origin requires the explicit
  `allow`. The bundle cannot set this itself. Without it it **degrades gracefully** (CSS takeover on
  desktop; faux-landscape on phones).
- **Transformed-ancestor caveat:** for the desktop CSS takeover and the iPhone faux-landscape to fill
  the *real* viewport, avoid `transform` / `filter` / `perspective` / `contain:paint|layout|strict` on
  **ancestors of the mount container** — any of those becomes the containing block for the fixed
  full-viewport surface, so it won't fill the screen. (The real-fullscreen path on Android / iPad is
  immune — the fullscreen element is in the top layer.)

---

## 7. Behaviour notes

- **First start only** plays the one-time intro; **"Run Again"** restarts locally
  without replaying it and without auto-saving (each completed run fires `onGameEnd`).
- **No browser storage required.** Any local high-score is optional and degrades
  gracefully where storage is unavailable; the host owns real persistence.
- **Security:** host-supplied strings are rendered inertly (no HTML injection). A
  hostile `name`/`team` shows as literal text / a neutral flag — it cannot execute
  script in your origin.
- **Errors:** invalid `container` or non-function `onGameEnd` throw synchronously
  with a clear message (developer errors). Data problems (empty board, missing name,
  malformed rows) degrade gracefully rather than throwing.

---

## 8. Server-side run validation (anti-cheat)

The score is computed in the browser, so the host validates each run **server-side**. The
bundle stays pure (no network, no storage); it only **carries a token through** and **exposes
enough run metadata** for your server to sanity-check a result. Two pieces make this work:

1. **`runToken` binds a result to a token.** Issue a **signed, single-use** token, pass it as
   `config.runToken` (or arm the next run with `handle.setRunToken`), and it comes back verbatim
   as `result.runToken`. Reject any result whose token is missing, unknown, already-consumed, or
   doesn't match the player. (The bundle treats the token as an opaque blob — all meaning is yours.)
2. **`durationMs` lets you reproduce the distance.** In this auto-runner, distance is a
   near-deterministic function of **active survival time**. Reproduce the curve with the exported
   `paceDistance(activeMs)` and check it against the reported distance:

   ```js
   import { paceDistance } from 'chased-by-the-goat';
   const expected = paceDistance(result.durationMs);
   const ok = Math.abs(result.distance - expected) <= EPS;   // EPS ≈ a few metres (see below)
   ```

`durationMs` is **active simulation time** — it is accumulated from the *same* fixed-timestep
loop that advances the world, and is **frozen whenever the run is paused** (the bundle pauses on
tab-blur). A run backgrounded for 30 s does **not** gain 30 s of `durationMs`, because it gained
no distance — so the check holds for legitimate players.

### The pacing model (port this to your server)

The game runs a fixed **60 fps logical** simulation. Distance is integrated **one frame at a
time**; `paceDistance` is the exported, canonical replay of the *exact* per-frame step the game
loop uses (`paceStep`), so the two can never drift.

**Constants** (px are internal logical units; the reported `distance` is metres = `floor(score)`):

| Constant | Value | Meaning |
|----------|-------|---------|
| `SPEED0` | `11` | starting scroll speed (px / 60fps-frame) |
| `MAX_SPEED` | `27` | tier-1 cap |
| `HARD_MAX` | `35` | tier-2 cap (slow creep) |
| `ULTRA_MAX` | `43` | tier-3 cap (end-game creep) |
| `ACCEL` | `0.0024` | base speed gained per frame |
| `SCORE_RATE` | `0.035` | metres added per (speed-unit · frame) |
| `ULTRA_AT` | `10000` | score (m) where tier 3 begins |
| `FRAME` | `1000 / 60` | ms per logical frame (≈ 16.6667) |

**Per-frame step** — given the current `(speed, score)`, advance one frame of size `dt` (logical
frames; `dt = 1` at 60 fps). The **tier is chosen from the current state**, speed is bumped
**first** (clamped to the tier cap), then distance accrues **using the updated speed**:

```
function paceStep(speed, score, dt):
    if   speed <  MAX_SPEED:   cap = MAX_SPEED;  acc = ACCEL
    elif score <  ULTRA_AT:    cap = HARD_MAX;   acc = ACCEL * 0.16
    else:                      cap = ULTRA_MAX;  acc = ACCEL * 0.42
    speed = min(cap, speed + acc * dt)
    score = score + speed * SCORE_RATE * dt
    return (speed, score)
```

- **Tier selection** is by *current* `speed` (vs `MAX_SPEED`) and *current* `score` (vs `ULTRA_AT`),
  evaluated **before** the speed bump. Tier 1 ramps until speed reaches `MAX_SPEED`; then tier 2
  creeps toward `HARD_MAX` until `score` passes `ULTRA_AT`; then tier 3 creeps toward `ULTRA_MAX`.

**Total distance after `activeMs`** of active play — integrate from the canonical start
`(SPEED0, 0)` over `frames = activeMs / FRAME` logical frames: one full `dt = 1` step per whole
frame, plus one trailing `dt = frac` step for any partial frame, then floor:

```
function paceDistance(activeMs):
    if not (activeMs > 0): return 0
    frames = activeMs / FRAME
    speed, score = SPEED0, 0
    for i in range(floor(frames)):  speed, score = paceStep(speed, score, 1)
    frac = frames - floor(frames)
    if frac > 0:  _, score = paceStep(speed, score, frac)
    return floor(score)
```

**Tier transitions** (from a clean run at the canonical 60 fps): speed reaches `MAX_SPEED` at
≈ frame 6667 (≈ 111.1 s, ≈ 4434 m); `score` crosses `ULTRA_AT` (10000 m) at ≈ frame 12330
(≈ 205.5 s); speed reaches `HARD_MAX` at ≈ frame 18110 (≈ 301.8 s, ≈ 16492 m).

**Worked example** — `durationMs → expected distance` (`paceDistance(durationMs)`), for unit-testing
your Go port. These hold exactly at the logical 60 fps step; real runs differ only by a small
frame-rounding ε:

| Active time | `durationMs` | `paceDistance` (m) |
|------------:|-------------:|-------------------:|
| 1 s   | `1000`   | `23` |
| 2 s   | `2000`   | `46` |
| 5 s   | `5000`   | `119` |
| 10 s  | `10000`  | `246` |
| 15 s  | `15000`  | `380` |
| 20 s  | `20000`  | `522` |
| 30 s  | `30000`  | `829` |
| 60 s  | `60000`  | `1930` |
| 90 s  | `90000`  | `3303` |
| 120 s | `120000` | `4939` |
| 180 s | `180000` | `8454` |
| 300 s | `300000` | `16357` |
| 600 s | `600000` | `42306` |

**Choosing ε.** The server integrates at a uniform `dt = 1`; a real client integrates with
per-frame `dt` values that vary slightly with the actual refresh rate and are clamped on stutter
(`dt ≤ 2`). This produces a small, bounded divergence that grows slowly with run length — a
tolerance of a few metres (or a small percentage of distance) absorbs it without admitting cheats,
since a fabricated distance for a given `durationMs` is off by far more.

---

## 9. Artifact

- One file: `dist/chased-by-the-goat.mjs` (~**307 KB raw / ~170 KB gzip**). The remaining size
  is base64-inlined assets — the Saira / Saira Condensed font subsets and the crowd/cue audio
  samples — a deliberate single-file, zero-network, exact-fidelity trade-off (first paint gated on
  `document.fonts.ready`, so no FOUT). No CJS/UMD build; import as ESM.
- Full types: [`types/index.d.ts`](./types/index.d.ts).

---

## 10. Verify the built bundle locally

```bash
pnpm dev      # serves the dev page (index.html) — loads the game from source
pnpm build    # → dist/chased-by-the-goat.mjs (+ asserts the single-file shape)
pnpm test     # vitest unit + integration suite
pnpm typecheck# tsc --noEmit against a TS consumer of types/index.d.ts
```
