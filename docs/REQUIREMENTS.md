# SayScore — Requirements & Design Specification

**SayScore** is an internal, mobile-first web app for SayOne employees to predict FIFA World Cup 2026 match scores, earn points, and compete on weekly and overall leaderboards. (Repository slug: `worldcup-predictor`.) This document is the single source of truth for scope, scoring rules, the design system, the technical stack, and the dev/ops setup. Frontend and backend live in one GitHub repository (monorepo), and the build is driven with the Superpowers plugin for Claude Code (see the README).

---

## 1. Overview & goals

Employees predict the score of every World Cup match before kickoff. Predictions are editable until kickoff, then locked. Points are awarded automatically once results settle. Winners are recognised weekly and at the end of the tournament. The app must be fast, mobile-friendly, and require near-zero manual operation once fixtures are seeded.

All times are displayed to users in **IST (Asia/Kolkata)**. Internally, timestamps are stored in **UTC**.

---

## 2. User roles

There are two roles. Role is stored on the user record and enforced by server-side middleware.

- **User** — signs in, makes/edits match predictions before kickoff, makes tournament bonus predictions before the bonus lock, views fixtures and leaderboards.
- **Admin** — everything a user can do, plus all elevated operations: trigger the results sync from the results API, add / edit / remove matches and the schedule, correct match results and penalty-shootout winners, edit configurable settings (e.g. the results-cron time, bonus lock time), promote/demote other users, and force a points recompute.

Any admin edit to a match (fixture detail or result) sets a `manual_override` flag on that match so the scheduled results job never overwrites a human correction.

---

## 3. Functional requirements

### 3.1 Authentication — Google Workspace SSO, domain-restricted

- Sign-in is Google only, restricted to the `sayonetech.com` Workspace domain.
- Frontend uses Google Identity Services to obtain an ID token; the backend verifies the token (signature, audience = our client ID, `email_verified = true`) and **gates on the `hd` (hosted-domain) claim equal to `sayonetech.com`**. The email-string check is a secondary guard, not the gate.
- On success the backend issues an httpOnly, Secure, SameSite session cookie. No passwords are ever stored.
- First sign-in auto-provisions a `User` record. Initial admins are seeded by email via configuration.

### 3.2 Fixtures & predictions

- The fixtures list is grouped by IST date. Each match shows both teams, kickoff time in IST, and a live countdown.
- A user enters a predicted score per match. Predictions **open 3 days (72h) before kickoff** and can be changed any number of times within that window, **until kickoff**.
- **Prediction window is enforced server-side**: a write for a match kicking off more than 72h away is rejected (`422`, "predictions open 3 days before kickoff"). The UI shows an "opens on …" hint and a popup for not-yet-open matches, but the server is authoritative. (Decision added post-v1 at product request; see §17.)
- **Locking is enforced server-side** from the stored kickoff timestamp: any write where `now >= kickoff_utc` is rejected, regardless of client state. The UI also reflects the locked state, but the server is authoritative.
- Incrementing a team score gives lightweight goal feedback: a football travels from that team's `+` control toward the opposing flag on a responsive curved shot path. Left-team goals travel left-to-right; right-team goals travel right-to-left. Repeated clicks may play independently, and reduced-motion users receive no travelling-ball animation. As a visual-only Easter egg, an opponent's shot toward Argentina reaches the Argentina box, meets a subtle wall-impact cue, deflects upward, then loses momentum as it arcs down to midfield and fades; Argentina's own shots and matches without Argentina use the normal path. This feedback never changes score values, validation, locking, or save behavior.
- For knockout matches where the user's predicted score is a **draw**, the user may additionally pick the penalty-shootout winner (see 3.3).

### 3.3 Scoring rules

Per match:

- **Exact score** (both numbers correct): **5 points**
- **Correct result** (right winner, or correctly predicted a draw) but wrong score: **3 points**
- **Incorrect**: **0 points**

Knockout penalty bonus + advancement rule:

- Applies only to knockout matches that go to a penalty shootout.
- **Advancement-first:** when a knockout draw goes to a shootout, a draw prediction scores **only if it also names the correct shootout winner** (the advancing team). A wrong or missing winner pick scores **0**, even when the regulation/extra-time scoreline was exact.
- When the shootout winner is correct, the usual tier applies (5 exact / 3 correct-result) **plus +1** penalty bonus.
- See §5 for the precise, implementable definition (confirmed in §17).

### 3.4 Tournament bonus predictions

On or before the **bonus lock** (28 June 2026, end of day IST — configurable), users may predict:

| Category | Points if correct |
|---|---|
| World Cup Winner | 30 |
| Runner-Up | 20 |
| Golden Ball Winner | 10 |
| Golden Boot Winner | 10 |
| Golden Glove Winner | 10 |
| Young Player Award Winner | 10 |
| Fair Play Trophy Winner | 10 |

Maximum bonus = **100**. These are scored once, after the tournament concludes, and added to each participant's total.

Team and player pickers use searchable dropdown menus that render above surrounding cards without changing the Tournament Bonus card's clipped visual surface. Menus remain anchored to their controls, flip when viewport space is limited, and stay within the desktop/mobile viewport.

### 3.5 Leaderboards

- **Weekly**: every Monday, sum points from matches whose **kickoff** falls in the previous IST week (Mon 00:00 → Sun 23:59 IST). Attribution is by kickoff timestamp (deterministic, so a late result-correction never shifts points between weeks). Highest total(s) are the Weekly Winner(s). **Weekly ties stand** — they produce multiple co-winners, and **every co-winner is paid the full prize** (the §5.1 tie-break does **not** apply to the weekly prize; it only decides distinct 1st/2nd for the overall standings). Prize: ₹500 Amazon Gift Card per weekly winner.
- **Overall**: all match points + bonus points combined, for the final standings. **Final-standings ties are broken by the cascade in §5.1** (so 1st and 2nd are distinct winners). Prizes: 1st ₹5,000, 2nd ₹2,500.
- **Hall of Fame**: past weekly champions are retained and shown to **all** users (newest week first) — each week lists its co-winner(s), their points, and the ₹500 gift-card payout status. Read-only for regular users; admins additionally toggle the payout status (§3.6). The UI pages **one week at a time** with Prev/Next.
- **Presentation**: the leaderboard panel shows the **top 5** for the selected period (Overall/Weekly) with a **"View full leaderboard"** modal — scrollable, paginated — for the complete list. The current user's row is marked "(You)", and an off-page "Your rank: N" line shows when they fall outside the visible rows. The modal also has an **"Exact Score" filter** toggle that reveals each player's count of **exact-score (5-pt) picks** next to their points (display-only — it does not re-rank or hide anyone).

### 3.6 Admin features

Fixtures sync (initial seed + re-sync), manual match create/edit/delete, result and penalty-winner correction, settings management (cron time, bonus lock time, admin list), and a manual "recompute points" action. All destructive actions require an explicit confirm.

**Implemented (M8a):** the manual match CRUD, result/penalty-winner correction, and user promote/demote tools described above now exist as standard `RequireAdmin` routes (registered in all environments) — see §11. Every admin match write sets `manual_override`; result correction immediately re-scores predictions (idempotent) under the kickoff precondition.

The admin match-management UI presents **New Match**, **Edit Match**, and **Set Result** forms in one reusable responsive modal system rather than expanding forms inline in the fixture list. The modal uses the dark design system, traps focus, supports close button / backdrop / Escape dismissal when no save is pending, locks page scroll while open, and preserves all existing validation and mutation behavior.

**Implemented (M8b):** **settings management** and the **manual recompute action** now exist as standard `RequireAdmin` routes (all environments) — see §11. Settings management edits exactly three validated keys — `results_cron`, `weekly_cron`, `bonus_lock_at` — via `GET/PUT /api/admin/settings`; values are validated before any write (no partial write). `bonus_lock_at` takes effect **live** (read per request by the bonus handler); `results_cron`/`weekly_cron` apply on the **next process restart**. The manual recompute (`POST /api/admin/recompute`) is an **idempotent points rebuild** that re-derives `predictions.points` (+ penalty bonus) and `bonus_predictions.points` from the stored results — it never touches match results or already-declared weekly winners (`weekly_results`). Fixtures sync remains deferred.

**Implemented (M8c):** the admin **Bonus Outcomes** screen now exists — admins enter/edit the seven tournament-award winners (team awards via a team picker, individual awards via a player search) backed by `GET/PUT /api/admin/bonus/results` (see §11). `GET` returns all seven categories in canonical order with their current outcome + resolved label and a `set` flag. **Saving auto-scores:** the `PUT` materializes `bonus_predictions.points` immediately, so the overall leaderboard reflects the winners at once (idempotent; manual recompute remains the bulk safety-net). With this, **Milestone 8 (admin tools) is COMPLETE** — match/result/user management (M8a), settings + recompute (M8b), and bonus outcomes (M8c) are all delivered. The only deferred admin item is fixtures sync.

**Mark weekly prize paid** — admins mark a weekly winner's ₹500 gift card paid or unpaid via `PUT /api/admin/winners/paid`; the status surfaces in the Hall of Fame (§3.5) for everyone. This is a **standard admin route** (`RequireAdmin`) registered in **all** environments — like the manual job triggers below.

**Manual job triggers** — admins can manually fire the scheduled jobs (`results-ingest`, `weekly-winner`, `bonus-score`) on demand. This is a **standard `RequireAdmin` route registered in all environments** (no longer debug/dev-gated), so an admin can run the daily ingest / weekly calc from production if a scheduled cron run was missed. The admin UI shows the controls to any admin (`me.role === "admin"`). Scoring is idempotent, so re-running a job is safe.

### 3.7 Help / how to play

A **Help** button in the top bar opens a "How to play" modal summarising the rules for players: the scoring tiers (§3.3) and knockout penalty bonus, the tournament bonus and its lock (§3.4), the weekly/overall leaderboards and prizes (§3.5), the 3-day prediction window (§3.2), and prediction privacy (§4). Its content mirrors this spec.

### 3.9 Chat assistant

SayScore includes a first-party, prompt-only AI chat assistant powered by OpenAI.

- **Launcher**: a bottom-right floating button opens a "Solid Stadium Card" chat panel. The panel always renders; when the backend is unconfigured (503) it shows an "unavailable" message inline rather than hiding the launcher.
- **Auth-gated**: `POST /api/chat` is inside the `RequireAuth` group — unauthenticated requests receive 401. The route also carries its own per-user rate limiter (separate from the general write limiter) to bound OpenAI costs.
- **Streaming**: the backend proxies the OpenAI completion as **Server-Sent Events** (SSE). Each `data:` frame carries a JSON-encoded token delta; `data: [DONE]` signals end of stream; `event: error` frames carry mid-stream failures (the 200 status is already committed at that point; pre-stream failures remain 4xx/503).
- **System prompt**: loaded from the file path in `OPENAI_SYSTEM_PROMPT_FILE` at server start. Clients send only user/assistant turns in the request body — the system prompt is **never** read from the request.
- **Per-user personalization**: the prompt may contain `{{user_name}}` / `{{user_first_name}}` placeholders, substituted per request with the signed-in user's display name (first name derived from it; a blank name falls back to "there"). Only the name is sent — no other profile fields.
- **Last-20 window**: the handler trims incoming history to the last 20 messages before forwarding to OpenAI, bounding token cost.
- **Disabled when unconfigured**: if `OPENAI_API_KEY` is empty, `POST /api/chat` returns `503 Service Unavailable`. No DB tables are added by this feature.
- **Frontend history**: conversation history lives in `sessionStorage` only — no server-side persistence, no DB. Clearing the tab or refreshing the page starts a fresh session.
- **Model & temperature**: model defaults to `gpt-4.1-mini-2025-04-14` (override with `OPENAI_MODEL`); sampling temperature defaults to `0.8` (override with `OPENAI_TEMPERATURE`).

### 3.8 Match celebrations

When a **celebrated team (Brazil only)** wins a match, each user sees a one-time
full-screen **victory celebration** on their next visit after the match goes FINAL —
**once per user per match, across devices** (tracked server-side in `celebration_views`).
The overlay (canvas confetti/fireworks + a synthesized carnival soundtrack + a "VITÓRIA"
reveal) shows an inline **scorecard** of the won match (e.g. `BRA 3 – 1 JOR`) and a "Skip"
button. A "celebrated win" is a FINAL match whose winner (higher score, or the knockout
shootout winner on a draw) is an allowlisted team (`BRA`). If multiple unseen celebrated
wins exist, only the **most recent** plays; the rest are marked seen. The allowlist is
server-side and extensible (Brazil only for now).

**Audio (browser-policy note):** the soundtrack only plays when the celebration is started by a
user gesture (browser autoplay rules). The **automatic** on-login celebration is therefore
**visual-only** — it auto-plays muted and resumes audio only if the user taps the overlay. This
is intentional, not a defect.

### 3.10 GOAT mini-game (engagement)

SayScore embeds **Chased by the GOAT**, a self-contained arcade endless-runner, as a standalone
engagement feature. It is **completely separate from predictions, scoring, and the prize
leaderboards** — game results never affect points, weekly winners, the ₹500 prize, or the §5.1
standings, and **no prize/payment data is attached to the game** (decision §17.1).

- **Bundle**: ships as a prebuilt, dependency-free ESM bundle (`chased-by-the-goat`, vendored as a
  packed tarball under `frontend/goat-game/`). Mounted via `mountGoatGame(container, config)`, it
  renders inside a host-owned container and **never performs I/O itself** — the host owns all
  persistence (see the bundle's `INTEGRATION.md`).
- **Two metrics per run**: `distance` (metres survived, integer) and `coins` (collected this run,
  integer ≥ 0). They are independent — coins do not add to distance.
- **Two leaderboards**: a **distance board** ranked by each user's **best** run (`MAX(distance)`),
  and a **coins board** ranked by each user's **lifetime pool** (`SUM(coins)` over all their runs).
  Both cap at 20 rows and appear only inside the game's own screens (the bundle's landing/game-over
  boards); they are **not** part of `GET /api/leaderboard`.
- **Launch surface**: the game is opened from the **Predictions screen's top promo strip**, which is
  split into two cards — the existing external **Penalty Shootout** link and a **Chased by the GOAT**
  card that opens the game in a **new browser tab** at the standalone page `/goat-game.html` (exactly
  like the external Penalty Shootout `<a target="_blank">`). The standalone page has a FIFA-World-Cup-style
  dark background and reuses the same first-party session cookie and `/api/game/*` endpoints (same origin).
  No nav tab.
- **Saving**: each completed run fires the bundle's `onGameEnd`; the host persists it via
  `POST /api/game/runs`. Storage is **append-only** (one `game_runs` row per run); the boards are
  plain aggregates over those rows, never incremented in place — consistent with the
  materialized-points philosophy (§5).
- **Server-authoritative validation (anti-cheat)**: the score is computed in the browser, so every
  run is validated server-side before it is stored — see **§18**.
- **Auth & rate limiting**: both endpoints sit inside the `RequireAuth` group.
  `GET /api/game/leaderboard` carries its **own** per-user rate limiter (separate from the write
  limiter) because it mints a single-use run token on every call — this bounds token issuance
  and `seenJTI` set growth (~30/min sustained, burst 10).
  `POST /api/game/runs` inherits the per-user write rate limiter (§12).

---

## 4. Privacy of predictions

Other participants' predictions for a match are **hidden until that match locks** (kickoff). After lock, predictions and earned points may be shown. This prevents copying and keeps the game fair. (Decision recorded in §17.)

**Implemented:** `GET /api/matches/{id}/predictions` returns every player's pick for a match — name, scoreline, shootout winner, and earned points once FINAL — but **only once the match has locked** (`403` before kickoff, server-authoritative). The UI reveals them in a scrollable modal on locked/past matches ("Others' picks").

---

## 5. Scoring engine specification

The engine is pure and **idempotent**: it recomputes points from the stored result rather than incrementing, so re-running it (or the daily job running twice) can never double-count.

```
score(prediction, match):
  if match.status != FINAL: return 0, penalty=0
  ph, pa = prediction.home, prediction.away          # predicted
  ah, aa = match.home_score, match.away_score         # actual (full/extra time)

  if ph == ah and pa == aa:           pts = 5         # exact
  elif sign(ph - pa) == sign(ah - aa): pts = 3        # correct result (incl. draw==draw)
  else:                                pts = 0

  correct_shootout = (match.stage == KNOCKOUT and match.went_to_penalties
                      and prediction.penalty_winner == match.penalty_winner)

  # Advancement-first: in a knockout that went to a shootout, a DRAW prediction
  # must also name the correct advancing team (shootout winner) to score at all.
  # A wrong or missing pick zeroes the whole prediction, even on an exact draw.
  if match.stage == KNOCKOUT and match.went_to_penalties and ph == pa
     and not correct_shootout:
        return 0, penalty=0

  bonus = 0
  if correct_shootout and ph == pa and pts > 0:        # predicted draw, earned points, right winner
        bonus = 1

  return pts, bonus
```

- **Advancement-first knockout rule:** for a knockout match that goes to a penalty shootout, predicting a draw is only worth points if you also picked the correct shootout winner (the team that advances). Get the advancing team wrong — or omit the pick — and the prediction scores **0**, regardless of how accurate the regulation/extra-time scoreline was. When the winner is correct, the usual score tier (5 exact / 3 correct-result) applies, plus the **+1** penalty bonus. (This makes the advancing team the first requirement, so no one can out-score a player who correctly predicted the advancing team.)
- `sign(0)` represents a draw; a predicted draw matching an actual draw scores the correct-result path.
- Points are stored on the prediction row (`points`, `penalty_bonus`) when a match goes FINAL, so leaderboard queries are simple sums over a date window.
- Bonus-prediction points are computed once at tournament end from the seven award outcomes.

### 5.1 Final-standings tie-break (overall only)

Weekly winners allow ties (co-winners). The **overall** final standings must produce distinct 1st/2nd, so equal total points are broken by this cascade — each step applied only if the previous is still tied:

1. **Total points** (the rank metric: match points + penalty bonuses + tournament bonus points). As of M7 the overall total is **live**, summing `predictions.points + penalty_bonus` plus each user's `SUM(bonus_predictions.points)`.
2. **Most exact-score predictions** (count of 5-point matches).
3. **Most correct-result predictions** (count of 3-point matches).
4. **Most correct tournament bonus picks** (count of bonus categories scored — `COUNT(bonus_predictions WHERE points > 0)`). **Live as of M7** (previously contributed 0 while the bonus feature was unbuilt).
5. **Shared rank** — if still tied, the rank is shared (effectively impossible in practice; documented for completeness).

This cascade is computed from stored per-prediction points, so it is deterministic and recompute-safe.

---

## 6. Scheduled jobs

Run in-process via a cron scheduler on a single backend instance (use a leader-lock if you later scale to multiple instances). The process timezone is `Asia/Kolkata`.

- **results-ingest** — default `0 3,8,13 * * *` (03:00 / 08:00 / 13:00 IST), stored as a configurable setting. WC 2026 matches finish across a wide IST window (~23:30 → ~12:30, since kickoffs run 21:30 IST through 09:30 IST next morning), so three intraday runs keep results fresh: 03:00 catches the evening matches plus the large 00:30-IST batch (≈19 games), 08:00 the overnight batch, 13:00 the morning finishers (including any knockout shootout). Each run pulls **FINISHED** matches from the results API over a recent **~2-day UTC window** (deliberately overlapping prior runs so a missed cron tick is still caught; the idempotent recompute makes re-fetching already-scored matches free), updates `home_score`/`away_score` + `went_to_penalties` + `penalty_winner_team_id` for matches *not* flagged `manual_override`, then recomputes points idempotently for affected predictions (re-runs are safe).
- **weekly-winner** — default `0 9 * * 1` (Monday 09:00 IST). Runs after the Monday 03:00 + 08:00 result-ingest passes so the just-ended week's late Sunday-night matches are scored before the winner is finalized (running earlier, e.g. just after midnight, risks finalizing on incomplete data). Computes the previous Mon–Sun IST window, writes `weekly_results`, and marks winner(s), allowing ties.

Kickoff **locking is real-time** (enforced on every prediction write) and is *not* part of these jobs.

Both jobs are also invokable on demand via the **admin** job trigger (§3.6), available to admins in all environments.

- **`RESULTS_CRON_ENABLED`** (default `true`) gates the *scheduled* results-ingest only: when `false` the cron does not start, but the manual admin trigger still works. The local Docker stack sets it `false` so seeded demo data isn't overwritten by live results; production leaves it `true`.
- **Slack notifications (optional)** — if `SLACK_WEBHOOK_URL` is set, every job run (scheduled *and* manual) posts a human-readable completion message to Slack (job name, IST timestamp, a result summary, or the error). This makes the cron's liveness observable.

---

## 7. Design system

Design serves the task: predict fast, glance at standings. The bar is earned familiarity — it should feel as trustworthy as Linear or Stripe, not decorated.

> **Implementation note:** the frontend is built using the **`impeccable`** design skill, which realizes the tokens, layout, component states, and motion defined in this section. Treat §7 as the design contract that skill must satisfy.

> **🅥2 Visual language (current — supersedes the coral/OKLCH palette below).** Per a product decision (imported from the "Saxone Predictions" Claude Design project, 2026-06-18), v1 ships an **Apple-style "liquid glass"** look: a **system-dark** canvas with an **Apple-blue accent** (`#1a84ff` dark), frosted translucent card surfaces, the native **San Francisco / system font stack** (no web fonts; numerics use tabular system figures rather than JetBrains Mono), and Apple-generous radii/soft shadows. Football-themed motion is intentional here (a FIFA-card standing hero with mowed-pitch stripes + a spinning ball, and a thunderstorm backdrop revealed by lightning). The CSS variable **names** are unchanged (`--coral` is the accent, `--brand` aliases it, `--bg`/`--surface`/`--text`/etc.), so the §7.4–§7.7 contracts (layout, component states, motion, accessibility) still hold; only the **values** in §7.2 and the §7.3 font choice are superseded. Tokens live in `frontend/src/styles/tokens.css`. The two optional background **photos** (`standing-bg.png`, `stadium-ronaldo.png`) exceed the design connector's 256 KiB fetch cap — drop them into `frontend/public/` to activate; absent, a CSS gradient stands in.

### 7.1 Theme & rationale

Dark-first (canonical and **only** theme in v1; light remains deferred). The usage scene is an employee on their phone, late evening IST, locking a scoreline minutes before a North-America kickoff near midnight their time — often in a dim room. That justifies a dark default (by use, not fashion).

Color strategy (v2): a near-black Apple system canvas plus a single Apple-blue accent, with liquid-glass card surfaces. (The original "restrained indigo-ink + warm coral" strategy below is retained for historical rationale.)

### 7.2 Color tokens (OKLCH)

The SayOne brand color `#E95145` is the single warm accent. Because it is warm and close to danger-red, it carries **both** primary actions **and** achievement; the danger red is deepened and always icon-paired so it never reads as the primary coral.

```css
:root {
  /* Surfaces — dark indigo canvas (not navy, not terminal-black) */
  --bg:        oklch(0.17 0.020 280);
  --surface-1: oklch(0.21 0.022 280);   /* rows, panels */
  --surface-2: oklch(0.25 0.020 280);   /* elevated, inputs */
  --border:    oklch(0.32 0.020 280);

  /* Text */
  --ink:   oklch(0.96 0.010 280);       /* primary */
  --muted: oklch(0.72 0.015 280);       /* secondary — passes 4.5:1 on --bg */
  --faint: oklch(0.55 0.015 280);       /* hints, disabled labels */

  /* Brand — SayOne #E95145 — primary action + current selection + achievement */
  --brand:        oklch(0.64 0.190 28); /* ≈ #E95145 */
  --brand-hover:  oklch(0.68 0.180 28);
  --brand-active: oklch(0.58 0.180 28);
  --brand-tint:   oklch(0.40 0.100 28); /* selected / chip background on dark */
  --on-brand:     oklch(0.22 0.070 28); /* text & icons ON a brand fill */

  /* Semantics */
  --success: oklch(0.72 0.150 150);     /* correct-result badge, positive */
  --warning: oklch(0.80 0.130 75);
  --danger:  oklch(0.55 0.200 20);      /* destructive — deepened, always icon-paired */
}
```

Rules: `--brand` solid fill is reserved for **safe** primary actions and achievement only. Destructive actions use `--danger` with a leading icon (`trash`/`alert`) and a confirm step — never a coral solid fill. Body text never uses coral; coral is for accents, fills, and small emphasis.

### 7.3 Typography

_(v2 supersedes the font choice below: the app uses the native San Francisco / system stack for both UI and numerics, with tabular figures `font-feature-settings: "tnum"` for aligned numbers — no web fonts. The original Inter + JetBrains Mono pairing is kept here for rationale.)_

One UI sans plus a monospace for all numerics — a real contrast axis, not two similar sans fonts.

- **UI / body**: Inter (with tabular figures `font-feature-settings: "tnum"` where numbers align).
- **Numerics**: JetBrains Mono — score inputs, kickoff countdowns, leaderboard point columns. Gives a quiet scoreboard character.
- Fixed `rem` scale (not fluid), ratio ≈ 1.2: 12 / 13 / 14 (UI base) / 16 (reading) / 18 / 22 / 28 px. Weights: 400 and 500 (use 600 only for the rare strong heading). Sentence case everywhere.

### 7.4 Layout

- Mobile-first. Bottom tab bar for thumb reach: **Predict · Standings** (plus an **Admin** tab visible only to admins). Predict shows the fixtures column; Standings shows the ranks column; Bonus is embedded inline at the top of the Predict view (not its own tab). On wider screens the bottom tab bar is hidden and navigation moves to a **centered pill nav in the topbar** (Predictions · Admin), with the two columns shown side by side.
- **Profile** is a **dropdown menu opened from the user chip in the topbar** (not a tab): it shows the signed-in identity, a "How to play" action, and a **Log out** action. Log out is destructive — it uses an icon and an inline confirm step before signing out.
- Fixtures are a **vertical list grouped by IST date** — not an identical-card grid. Each row: teams, IST kickoff + countdown, inline score inputs, prediction state.
- Leaderboard is a true **ranked table**; rank 1 gets the brand-accent highlight.
- Admin screens use a slightly different neutral surface layer but the same component vocabulary.

### 7.5 Components & states

Every interactive component defines: default, hover, focus (visible ring), active, disabled, loading, error. Loading uses **skeletons**, not centered spinners. Empty states **teach** ("Fixtures load on first setup", "No predictions yet — tap a match"). Affordances are consistent across screens (same button shape, same form controls, same icon set). Icons: a single outline set (e.g. Tabler/Lucide outline).

A semantic z-index scale: dropdown → sticky → modal-backdrop → modal → toast → tooltip (no arbitrary 9999).

### 7.6 Motion

Most state transitions are 150–250 ms, use ease-out curves, and avoid bounce. Earned interaction feedback may run longer when the motion communicates a physical action: the prediction score `+` control may play a ~720 ms constant-speed curved football shot toward the opposing flag. The achievement chip may count up in brand color when a finished match settles. Every animation has a `prefers-reduced-motion` fallback (crossfade/instant or omission). No orchestrated page-load sequences.

### 7.7 Accessibility

Body text ≥ 4.5:1 contrast (verified for `--muted` on `--bg`); visible focus rings; full keyboard navigation; `aria-label`s on icon-only buttons; touch targets ≥ 44px.

---

## 8. Tech stack

**Backend**
- Go 1.22+, router `chi`
- MySQL 8 on AWS RDS; access via `database/sql` + `sqlc` (type-safe generated queries); driver `go-sql-driver/mysql`
- Migrations: `golang-migrate` (versioned SQL)
- Auth: `google.golang.org/api/idtoken` for ID-token verification; signed httpOnly session cookie
- Scheduler: `robfig/cron/v3` (in-process)
- Results data: **football-data.org v4** — competition `WC`, season `2398` (2026-06-11 → 2026-07-19); API key sent in the `X-Auth-Token` request header (not the URL). Used **only** for live results; fixtures are seeded statically (M2).
- Logging: stdlib `slog`; config via env (12-factor)

**Frontend**
- React 18 + TypeScript + Vite
- Tailwind CSS (tokens above wired as CSS variables) + shadcn/ui (Radix primitives)
- TanStack Query (data/cache), React Router, react-hook-form + zod (forms/validation)
- Google Identity Services for sign-in

**Why football-data.org**: it exposes match status, the full-time score, the winner, and crucially the shootout signal needed for the knockout penalty bonus, on a free tier sufficient for a few polls per day (10 req/min). Because fixtures are static (seeded in M2), the API is used **only** to ingest results.

**Results endpoint**: `GET /v4/competitions/WC/matches?status=FINISHED` (optionally bound with `&dateFrom=YYYY-MM-DD&dateTo=YYYY-MM-DD`, UTC dates, or narrow by `&stage=` / `&matchday=` / `&group=`); `GET /v4/matches/{id}` for a single match.

**Field mapping (football-data.org → SayScore):**

| API field | Maps to |
|---|---|
| `status == FINISHED` | score the match (set `matches.status = final`) |
| `score.fullTime.{home,away}` | `matches.home_score` / `away_score` (exact + correct-result points) |
| `score.winner` (`HOME_TEAM` / `AWAY_TEAM` / `DRAW`) | sanity-check the stored result |
| `score.duration == PENALTY_SHOOTOUT` | `matches.went_to_penalties = true` (the penalty-bonus trigger) |
| `score.winner` when a shootout | resolves `matches.penalty_winner_team_id`; `score.penalties` holds the tally |
| `stage` (`GROUP_STAGE` vs `LAST_32` … `FINAL`) | `matches.stage` (group vs knockout) |
| `id` | `matches.api_fixture_id` (match/update key) |
| `utcDate` | `matches.kickoff_utc` (displayed in IST) |

**Matching results to seeded matches**: football-data.org's `id` differs from the static seed's match number, so M5 maps each finished match to its seeded row by `(utcDate, teams)` once and stores the API `id` in `matches.api_fixture_id` for subsequent direct updates.

---

## 9. Architecture & monorepo layout

Single GitHub repository. The Go module lives inside `backend/`; module path: **`github.com/sayonetech/worldcup-predictor/backend`**.

```
.
├── backend/                 # Go service
│   ├── cmd/server/          # main entrypoint
│   ├── internal/
│   │   ├── http/            # chi routes, middleware (auth, role, CSRF)
│   │   ├── auth/            # Google ID-token verify, sessions
│   │   ├── scoring/         # pure scoring engine (heavily unit-tested)
│   │   ├── sportsapi/       # football-data.org results client
│   │   ├── jobs/            # results-ingest, weekly-winner
│   │   ├── store/           # sqlc-generated queries + db
│   │   └── config/
│   ├── migrations/          # golang-migrate SQL
│   ├── sqlc.yaml
│   └── Dockerfile
├── frontend/                # React + Vite app
│   ├── src/
│   │   ├── routes/          # Fixtures, Ranks, Bonus, Profile, Admin
│   │   ├── components/      # shared UI
│   │   ├── lib/             # api client, auth, query hooks
│   │   └── styles/tokens.css
│   ├── nginx.conf           # serves SPA, proxies /api
│   └── Dockerfile
├── deploy/
│   ├── docker-compose.yml   # local: mysql + backend + frontend(+adminer)
│   └── ...
├── docs/
│   └── REQUIREMENTS.md      # this file
├── .github/workflows/ci.yml
├── lefthook.yml             # pre-commit/pre-push for both stacks
├── Makefile
├── .env.example
└── README.md
```

The backend serves the JSON API; the frontend is a static SPA served by nginx, which also reverse-proxies `/api` to the backend (so the session cookie is first-party).

---

## 10. Data model (MySQL)

Indicative columns; refine in migrations.

- **users**: `id`, `email` (unique), `name`, `avatar_url`, `role` ENUM('user','admin'), `created_at`.
- **teams**: `id`, `api_team_id` (unique), `name`, `code`, `logo_url`.
- **players**: `id`, `source_id` (unique, football-data player id), `team_id` FK→`teams(id)`, `name`, `position`. Squad data backing the player-award bonus picks (golden ball/boot/glove, young player); seeded from a committed `data/players.csv`. Indexed on `name` for typeahead search.
- **matches**: `id`, `api_fixture_id` (unique), `stage` ENUM('group','knockout'), `round`, `home_team_id`, `away_team_id`, `kickoff_utc`, `status` ENUM('scheduled','live','final'), `home_score`, `away_score`, `went_to_penalties` BOOL, `penalty_winner_team_id` NULL, `manual_override` BOOL DEFAULT 0, `updated_at`.
- **predictions**: `id`, `user_id`, `match_id`, `home_score`, `away_score`, `penalty_winner_team_id` NULL, `points` NULL, `penalty_bonus` NULL, `created_at`, `updated_at`. **UNIQUE(user_id, match_id)**.
- **bonus_predictions**: `id`, `user_id`, `category` ENUM(winner, runner_up, golden_ball, golden_boot, golden_glove, young_player, fair_play), `ref_id` BIGINT (a team id for team awards — winner/runner_up/fair_play — or a player id for player awards — golden_ball/golden_boot/golden_glove/young_player; the category→ref-type mapping is a fixed code constant, so there is **no FK** on `ref_id` and integrity is validated server-side), `points` NULL (materialized once at tournament end). **UNIQUE(user_id, category)**.
- **bonus_results**: `category` (PK), `ref_id` (the actual outcome, same polymorphic-by-category semantics as `bonus_predictions.ref_id`), set by admin after the tournament.
- **weekly_results**: `id`, `user_id`, `week_start` (IST date), `points`, `is_winner` BOOL, `prize_paid` BOOL DEFAULT 0, `paid_at` DATETIME NULL. **UNIQUE(user_id, week_start)**.
- **settings**: `key` (PK), `value`, `updated_at` — a generic key/value store; the application reads/writes only the **three allowlisted keys** `results_cron`, `weekly_cron`, `bonus_lock_at` (implemented, M8b — migration `0009`). **Precedence:** env/config values seed any missing key on boot (idempotent; never overwriting an existing row), and the `settings` table is the runtime source of truth thereafter.
- **game_runs** (§3.10): `id`, `user_id` FK→`users(id)` (ON DELETE CASCADE), `distance` INT UNSIGNED, `coins` INT UNSIGNED DEFAULT 0, `played_at` TIMESTAMP (UTC). **Append-only — one row per completed run.** The §3.10 game boards are aggregates over this table (`MAX(distance)` and `SUM(coins)` per user); there are **no points/prize columns** — the game is off the prize path. Indexed on `user_id` and on `distance` (board ordering).
- **audit_log** (optional): admin actions for traceability.

---

## 11. API (REST, JSON)

Auth & session
- `POST /api/auth/google` — body: Google ID token → verifies, sets cookie, returns user.
- `POST /api/auth/logout`
- `GET  /api/me`

Predictions & fixtures
- `GET  /api/matches` — matches grouped by IST date, with the caller's predictions and lock state.
- `GET  /api/matches/:id`
- `PUT  /api/matches/:id/prediction` — create/update; **rejected if locked**.

Leaderboards
- `GET /api/leaderboard?period=week&week=YYYY-MM-DD`
- `GET /api/leaderboard?period=overall`
- `GET /api/winners` — past weekly champions for the Hall of Fame, grouped by week, newest first: `{ "weeks": [ { "week_start": "YYYY-MM-DD", "winners": [ { "user_id", "name", "avatar_url", "points", "prize_paid" } ] } ] }`. `week_start` is the IST calendar Monday; empty → `{ "weeks": [] }`.

Bonus
- `GET /api/bonus` — the caller's picks + lock state: `{ "lock_at": RFC3339, "locked": bool, "picks": [ { "category", "ref_type": "team"|"player", "ref_id", "label", "points"? } ] }`. `label` is the resolved team/player name; `points` is present once scored; categories with no pick are omitted.
- `PUT /api/bonus` — upsert picks; body `{ "picks": [ { "category", "ref_id" } ] }` (any subset of the 7). Validates each category and that `ref_id` exists in the correct table for the category's ref-type; returns the same shape as `GET /api/bonus`. **403 when `now >= BONUS_LOCK_AT`** (server-authoritative; the client clock is never trusted); 400 on a bad category / missing or wrong-type ref / invalid JSON.
- `GET /api/teams` — `[{ id, name, code }]` for the team-award dropdowns.
- `GET /api/players?q=<term>` — `[{ id, name, team_code, position }]`, name search, capped (20 rows) for the player-award typeahead.

Celebrations

- `GET    /api/celebrations` — unseen celebrated-team wins for the caller, newest-first:
  `{ "celebrations": [ { "match_id", "team_code", "team_score", "opponent_code",
  "opponent_score", "kickoff_utc" } ] }`. `RequireAuth`, all environments.
- `POST   /api/celebrations/seen` — body `{ "match_ids": [int,…] }` → `200 { "seen": N }`.
  Idempotently records the caller has seen those celebrations. `400` on empty/invalid body.

Admin (role=admin)

All admin routes below are **standard `RequireAdmin` routes, registered in all environments** (not debug-gated). Match management and user-role management are **implemented (M8a)**; settings and recompute are **implemented (M8b)**; bonus outcomes (read + auto-scoring write) are **implemented (M8c)** — with which **Milestone 8 (admin tools) is complete**; only fixtures sync remains **deferred**.

Match management (implemented, M8a) — every write sets `manual_override` so the results-ingest never overwrites an admin edit:

- `GET    /api/admin/matches` → 200 array of `{ id, match_number, stage, round, home_team_id, home_team, home_code, away_team_id, away_team, away_code, kickoff_utc, status, home_score, away_score, went_to_penalties, penalty_winner_team_id, manual_override }` (team names resolved for display, ordered by kickoff).
- `POST   /api/admin/matches` — body `{ home_team_id, away_team_id, kickoff_utc (RFC3339), stage ("group"|"knockout"), round }` → 201 `{ id }`. Sets `manual_override`, `status=scheduled`. 400 on same home/away team, bad stage, non-RFC3339 kickoff, or unknown team.
- `PUT    /api/admin/matches/:id` — same body; edits fixture detail only (not scores) → 200 `{ id }`. Sets `manual_override`. 404 if the match is absent.
- `DELETE /api/admin/matches/:id` → 204 (predictions **cascade**). 404 if absent.
- `PUT    /api/admin/matches/:id/result` — body `{ home_score, away_score, went_to_penalties, penalty_winner_team_id? }` → 200 `{ id, status: "final" }`. Sets `manual_override`, **preserves the existing `api_fixture_id`**, and **immediately re-scores all predictions on the match in one transaction (idempotent recompute)**. 400 on negative scores, penalties on a non-knockout match, a penalty winner that is neither the home nor away team, or **a match that has not yet kicked off** (`now < kickoff_utc`); 404 if absent.

User management (implemented, M8a):

- `GET    /api/admin/users` → 200 array of `{ id, email, name, avatar_url, role }`.
- `POST   /api/admin/users/:id/role` — body `{ role: "admin"|"user" }` → 200 `{ id, role }`. Guards: 400 on demoting **yourself**, demoting the **last remaining admin**, or a bad role; 404 if the user is unknown.

Settings & recompute (implemented, M8b) — standard `RequireAdmin` routes, registered in all environments:

- `GET    /api/admin/settings` → 200 `{ "results_cron": "...", "weekly_cron": "...", "bonus_lock_at": "<RFC3339>" }` — the three runtime-editable operational settings as a flat key/value map. 500 if the settings store cannot be read. **Precedence:** env/config seeds the `settings` table on boot; the DB is the runtime source of truth thereafter (§10).
- `PUT    /api/admin/settings` — body = any subset of the three keys (`results_cron`, `weekly_cron`, `bonus_lock_at`) → 200 with the **full updated map**. All keys+values are **validated before any write** (no partial write): 400 on an unknown key, an invalid cron expression (parsed with the same robfig standard parser the schedulers use, incl. `@`-descriptors like `@daily` / `@every 1h30m`), an invalid RFC3339 `bonus_lock_at`, an empty body, or invalid JSON; 500 on a store failure after validation passed. **Liveness:** `bonus_lock_at` takes effect **live** (the bonus handler reads it per request — still server-authoritative); `results_cron`/`weekly_cron` apply on the **next process restart**.
- `POST   /api/admin/recompute` → 200 `{ "matches_rescored": N, "predictions_updated": N, "bonus_updated": N }`. **Idempotent** points rebuild: re-derives `predictions.points` (+ penalty bonus) and `bonus_predictions.points` from the stored match results and `bonus_results` (absolute SET, never increment). Read-only over `matches`; **never** writes match results or `weekly_results` (already-declared weekly winners are immutable historical facts). 500 on failure.

Other:

- `POST   /api/admin/fixtures/sync` *(deferred)*
- `PUT    /api/admin/winners/paid` — body `{ "week_start": "YYYY-MM-DD", "user_id", "paid": bool }` → 200 `{ "week_start", "user_id", "prize_paid" }`. Marks a weekly winner's ₹500 gift card paid/unpaid. 400 on a bad date / non-positive `user_id` / bad JSON; 404 when no matching winner row. A **standard `RequireAdmin` route, registered in all environments** (not debug-gated).
- `GET    /api/admin/bonus/results` *(implemented, M8c)* → 200 `{ "results": [ { "category", "points", "ref_type": "team"|"player", "ref_id", "label", "set" } ] }`. Returns **all seven** award categories in canonical order (`winner`, `runner_up`, `golden_ball`, `golden_boot`, `golden_glove`, `young_player`, `fair_play`) so the admin Bonus Outcomes screen renders every row, set or not. `points` is the category's tournament-bonus value (§3.4); `ref_type` is `team` (winner/runner_up/fair_play) or `player` (the four individual awards); for a set category `ref_id` is the stored outcome and `label` its resolved team/player name; **unset categories return `ref_id: 0`, `label: ""`, `set: false`**. A stale/missing ref resolves to an empty label (degrades gracefully). A **standard `RequireAdmin` route, registered in all environments** (not debug-gated).
- `PUT    /api/admin/bonus/results` *(auto-scores, M8c)* — body `{ "results": [ { "category", "ref_id" } ] }` → 200 `{ "saved": N }`. Upserts one or more tournament-award outcomes (validated like bonus picks: known category, `ref_id` exists in the correct table). **Saving auto-scores:** after the validate-all-then-upsert, the handler immediately materializes `bonus_predictions.points` from the new outcomes (idempotent recompute, never increment) so the overall leaderboard reflects the winners at once — no separate step. Outcomes are persisted **before** scoring, so a scoring failure returns 500 but never loses the saved outcomes (re-run via `POST /api/admin/recompute`). The response is intentionally **`{ "saved": N }` only** (N = outcomes upserted); recompute remains the bulk path. 400 on a bad category / wrong-type ref / invalid JSON. A **standard `RequireAdmin` route, registered in all environments** (not debug-gated).
- `POST   /api/admin/jobs/run` — body `{ "job": "results-ingest" | "weekly-winner" | "bonus-score" }`. A **standard `RequireAdmin` route, registered in all environments** (not debug-gated), so an admin can run a missed cron from production. `bonus-score` idempotently materializes `bonus_predictions.points` from `bonus_results` (recompute, never increment).

Chat (§3.9)

- `POST /api/chat` — `RequireAuth`. Body: `{ "messages": [ { "role": "user"|"assistant", "content": "…" } ] }`. Trims to the last 20 messages, injects the server-side system prompt, and streams the OpenAI completion as SSE (`Content-Type: text/event-stream`). Each `data:` frame is a JSON-encoded token delta; `data: [DONE]` closes the stream; `event: error` frames carry mid-stream failures. Returns `401` when unauthenticated, `400` on bad JSON / empty messages / invalid role, `503` when `OPENAI_API_KEY` is unset or the streamer is unconfigured, `429` when the per-user chat rate limit is exceeded. The system prompt is **never** read from the request body.

Game (§3.10)

- `GET  /api/game/leaderboard` — `RequireAuth`. Returns both game boards, the caller's standing, and a fresh anti-cheat token: `{ "distance": [ { "user_id", "name", "avatar_url", "distance" } ], "coins": [ { "user_id", "name", "avatar_url", "coins" } ], "me": { "best_distance", "coin_pool" }, "run_token": "<signed>" }`. Each board is capped at 20 rows, sorted descending. `run_token` is a signed, single-use token (§18.2) the client arms the next run with.
- `POST /api/game/runs` — `RequireAuth`. Body `{ "run_token", "distance", "coins", "duration_ms" }`. Validates the run **server-side** (§18): verifies the token (signature + single-use + TTL + caller match), then checks `distance` against the reproduced pacing curve and `coins` against the spawn cap. On success stores one `game_runs` row and returns `{ "best_distance", "coin_pool", "run_token": "<next>" }` (a fresh token to arm the next run). **`400`** on invalid JSON / missing or non-integer fields / negative values; **`403`** on a missing, malformed, expired, already-used, or caller-mismatched token; **`422`** when the reported score is inconsistent with the run (rejected as implausible — not stored). Inherits the per-user write rate limit (§12).

Ops
- `GET /healthz`

---

## 12. Security & privacy

- httpOnly + Secure + SameSite=Lax session cookie; short-lived, refreshed on activity. **Implemented (M9):** the "short-lived, refreshed on activity" requirement is satisfied by a **sliding refresh** in `RequireAuth` — the cookie is re-issued once more than 24h has elapsed since it was issued (i.e. remaining TTL < 7d − 24h ≈ 6d), sliding the 7-day window forward so active users stay logged in while idle sessions expire naturally after 7 days.
- **CSRF — documented decision (M9):** for this **internal, same-origin, SSO-gated** app, **`SameSite=Lax` is the accepted CSRF defense**; no CSRF token is implemented. This is a documented deviation from the earlier "SameSite plus a CSRF token" wording: the SPA is served first-party and proxies `/api` same-origin (the session cookie never crosses origins), so a Lax cookie already blocks cross-site state-changing requests. A CSRF token can be added later if the app is ever exposed cross-origin.
- Domain-restricted SSO via the `hd` claim (see 3.1).
- No secrets in the repo: `.env` is git-ignored; CI/production use GitHub Actions secrets / RDS secrets. **Secret rotation (M9):** `SESSION_SECRET` and `FOOTBALL_DATA_API_KEY` appeared in local dev and **MUST be rotated before production**; production injects them via GitHub Actions / RDS secrets, never the committed `.env`.
- **Rate limiting — implemented (M9):** in-memory **token-bucket** limiter (single-instance app). **Per-IP** on the auth endpoints — login (`POST /api/auth/google`) and logout (`POST /api/auth/logout`) — at ~10/min, burst 5. **Per-user** on **all authenticated writes** (mutating methods on predictions, bonus, and admin endpoints incl. recompute) at ~60/min, burst 20; reads pass through. Over-limit requests get **HTTP 429 with a `Retry-After`** header.
- **Request body size cap — implemented (M9):** all `/api` requests are capped at **1 MiB** via `http.MaxBytesReader`; an over-limit body fails JSON decode → 400.
- **Trusted-proxy note (M9):** the per-IP rate limiter keys on the client IP via chi `middleware.RealIP`, which reads `X-Forwarded-For` / `X-Real-IP` as set by the frontend nginx proxy (`frontend/nginx.conf`). In production the backend **MUST only be reachable via that proxy** (do **not** expose the backend port publicly) — otherwise `X-Forwarded-For` is client-spoofable. The rate limit is **best-effort throttling, not an authorization boundary**.
- Others' predictions hidden until match lock (§4).
- The manual job-trigger endpoint (`POST /api/admin/jobs/run`, §11) is an admin-only (`RequireAdmin`) route registered in all environments, so an authenticated admin can invoke a job in production; it is never reachable by non-admins.
- **Game run validation (§3.10/§18):** game scores are client-computed, so `POST /api/game/runs` is server-authoritative. A **signed, single-use, short-TTL run token** (issued by `GET /api/game/leaderboard`) binds each submission to a server nonce, and the reported `distance` must match the server-reproduced pacing curve for the reported active duration (itself bounded by the server-measured token age), with `coins` bounded by the spawn cap. Implausible or unbound runs are rejected, never stored. This is **best-effort anti-cheat for an internal, no-stakes feature** (the game is off the prize path) — it raises the cost of forgery but is not a cryptographic guarantee; full server-side input-replay is deferred (§18.5).

---

## 13. Pre-commit hooks (frontend + backend)

Use **Lefthook** — one fast binary that runs Go and JS linters on staged files in a single repo. `lefthook.yml`:

```yaml
pre-commit:
  parallel: true
  commands:
    backend-fmt:
      root: backend/
      glob: "*.go"
      run: gofmt -l -w {staged_files} && go vet ./...
    backend-lint:
      root: backend/
      glob: "*.go"
      run: golangci-lint run
    sqlc-check:
      root: backend/
      glob: "{queries/*.sql,sqlc.yaml}"
      run: sqlc diff
    frontend-lint:
      root: frontend/
      glob: "*.{ts,tsx}"
      run: pnpm eslint --fix {staged_files} && pnpm prettier --write {staged_files}
    frontend-types:
      root: frontend/
      glob: "*.{ts,tsx}"
      run: pnpm tsc --noEmit

pre-push:
  parallel: true
  commands:
    backend-test:
      root: backend/
      run: go test ./...
    frontend-test:
      root: frontend/
      run: pnpm vitest run

commit-msg:
  commands:
    conventional:
      run: pnpm commitlint --edit {1}
```

Install once per clone with `lefthook install` (wired into the Makefile / postinstall). This keeps formatting, linting, type-checks, and sqlc consistency enforced before code lands, with tests gated on push.

---

## 14. Docker & deployment

**Local & deploy both use Docker.** Local development runs the full stack via `deploy/docker-compose.yml` (MySQL + backend + frontend/nginx, plus optional Adminer for DB inspection). See `README.md` for step-by-step.

- **backend/Dockerfile** — multi-stage: build a static binary on `golang:1.22`, run on a minimal base (distroless/alpine). Migrations run via a one-shot command or an init step.
- **frontend/Dockerfile** — multi-stage: `node` build → static assets served by `nginx`, which also proxies `/api` to the backend.
- **Production** — the same images deploy to AWS (ECS Fargate or a single Docker host), pointing `DB_*` at **RDS MySQL** instead of the compose MySQL container. The scheduler runs in the single backend instance; if you scale out, add a leader lock. Set `TZ=Asia/Kolkata`.
- **Production compose (`deploy/docker-compose.prod.yml`)** — swaps the nginx frontend for **Caddy**: automatic Let's Encrypt HTTPS for `$SITE_ADDRESS`, SPA serving, the `/api` reverse-proxy, and an internal `robots.txt` (`Disallow: /` — the app is SSO-gated). MySQL and the backend are **not** published to the host; only Caddy (80/443). The whole stack is driven by a single `.env.prod` (no separate `backend/.env`/`frontend/.env`). `APP_ENV=production` turns on Secure cookies. Full guide: `deploy/README.md`.

### Environment variables

Backend: `APP_ENV`, `HTTP_PORT`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `SESSION_SECRET`, `GOOGLE_CLIENT_ID`, `ALLOWED_EMAIL_DOMAIN=sayonetech.com`, `SEED_ADMIN_EMAILS`, `FOOTBALL_DATA_API_KEY`, `FOOTBALL_DATA_BASE_URL=https://api.football-data.org/v4`, `RESULTS_CRON=0 3,8,13 * * *`, `WEEKLY_CRON=0 9 * * 1`, `RESULTS_CRON_ENABLED=true`, `SLACK_WEBHOOK_URL` (optional), `BONUS_LOCK_AT=2026-06-28T23:59:00+05:30`, `TZ=Asia/Kolkata`, `OPENAI_API_KEY` (optional — chat disabled / 503 when unset), `OPENAI_SYSTEM_PROMPT_FILE` (path to a text file containing the chat system prompt; loaded at server start; supports `{{user_name}}`/`{{user_first_name}}` placeholders), `OPENAI_MODEL` (default `gpt-4.1-mini-2025-04-14`), `OPENAI_TEMPERATURE` (default `0.8`), and the GOAT mini-game knobs (§3.10/§18, all optional with conservative defaults): `GAME_TOKEN_TTL` (default `10m`), `GAME_DURATION_SLACK_MS` (default `1500`), `GAME_DIST_EPS_M` (default `25`), `GAME_DIST_EPS_FRAC` (default `0.02`), `GAME_COIN_MIN_SPACING_M` (default `300`, from the bundle's `MIN_COLLECTIBLE_SPACING_M` spawn gate), `GAME_COIN_SLACK` (default `3`), `GAME_MAX_DISTANCE` (default `0` = disabled until calibrated from live data). The run token is signed with the existing `SESSION_SECRET`.

Frontend (Vite): `VITE_GOOGLE_CLIENT_ID`, `VITE_API_BASE_URL`.

Production-only (`deploy/docker-compose.prod.yml`, via `.env.prod`): `SITE_ADDRESS` (Caddy public hostname for auto-HTTPS) and `DB_ROOT_PASSWORD`.

---

## 15. CI (GitHub Actions)

`.github/workflows/ci.yml` runs on PRs and main: backend (`golangci-lint`, `go test`, `sqlc diff`) and frontend (`eslint`, `tsc --noEmit`, `vitest`) in parallel, then builds both Docker images. Secrets are provided via repository/environment secrets.

**Fully implemented (M9).** The pipeline matches `.github/workflows/ci.yml`:

- **backend** job — `go vet`, `golangci-lint`, `go test ./...`, and `sqlc diff` (fails if generated code is stale).
- **frontend** job — `eslint`, `tsc --noEmit`, `vitest run`, and `pnpm build`.
- **docker** job (needs backend + frontend) — **build-only** validation of both production images (no registry push); the frontend image build passes the `VITE_*` build args.

---

## 16. Testing strategy

- **Scoring engine**: exhaustive table-driven unit tests (exact, correct-result, draw, knockout penalty bonus, idempotency). This is the highest-value test surface.
- **Locking**: tests that writes after `kickoff_utc` are rejected.
- **API**: handler tests with a test DB.
- **Frontend**: component tests (Vitest + Testing Library) for the prediction form and lock states; optional Playwright e2e for the predict → lock → score flow.

---

## 17. Resolved decisions

All previously-open questions were resolved in the brainstorming session (2026-06-13). Recorded here as the locked decisions:

1. **Penalty bonus interpretation** — ✅ **Confirmed as specified.** The +1 requires: knockout match, went to shootout, user predicted a draw, user's score earned points (exact or correct-result), and correct shootout winner. The penalty-winner pick is offered only when the predicted score is a draw (§3.2/§5).
2. **Bonus lock time** — ✅ **Confirmed:** 28 Jun 2026, 23:59 IST (the knockout-stage cutoff), configurable via `BONUS_LOCK_AT`.
3. **Prediction privacy** — ✅ **Confirmed:** others' predictions are hidden until a match locks at kickoff, then revealed (§4). Not always-hidden.
4. **Final-standings tie-break** — ✅ **Specified** (§5.1): cascade of total points → most exact scores → most correct-result hits → most correct bonus picks → shared rank. Applies to **overall** standings only; **weekly** winners still allow co-winners on ties (§3.5).
5. **Deployment target** — 🕒 **Deferred to deployment time.** The spec stays deployment-agnostic: Docker images for both tiers, MySQL via `DB_*` (RDS or container), single backend instance with in-process scheduler, add a leader lock before scaling out (§14). Choose ECS Fargate vs single Docker host at deploy time.
6. **Light theme** — ✅ **Confirmed:** dark-first is canonical for v1; light theme deferred, to be derived from the same §7.2 tokens later.

### 17.1 Additional decisions from the 2026-06-13 session

- **Weekly attribution by kickoff** (§3.5) — a match's points count toward the IST week containing its kickoff, so a late result-correction never shifts points across weeks.
- **Manual cron trigger** (§3.6, §6, §11) — admins can fire `results-ingest` / `weekly-winner` / `bonus-score` on demand; it is an admin-only route available in all environments (including production).
- **Go module path** (§9) — `github.com/sayonetech/worldcup-predictor/backend`, confirmed.
- **Frontend design skill** (§7) — the frontend is implemented with the `impeccable` design skill, treating §7 as its design contract.

### 17.3 GOAT mini-game (post-M9 engagement feature, 2026-06-30)

- **Added as net-new scope** beyond the M1–M9 core, with product sign-off. The embedded
  **Chased by the GOAT** runner (§3.10) is an engagement toy, **kept entirely off the prize path**:
  game scores have **no money/prize data**, do not feed `GET /api/leaderboard`, and never affect
  weekly winners or §5.1 standings. The decision (over merging into the overall board) was explicit:
  forgeable client scores must not sit next to the real ₹500 prize.
- **Anti-cheat posture (§18):** server-authoritative validation of every run via a signed
  single-use run token + reproduction of the bundle's pacing curve (Tier-2). Accepted as
  **best-effort** for a no-stakes internal feature; full server-side input-replay (Tier-1) is
  deferred. Tuning knobs are config (§14) and the absolute distance ceiling is calibrated from
  live data post-launch.

### 17.2 Milestone 9 — ops/security hardening + CI (final milestone)

- **Status: DONE.** M9 closed the remaining §12 security gaps and completed the §15 CI pipeline. With it, the project is **feature-complete and production-hardened across M1–M9** — (1) scaffold + SSO, (2) fixtures sync + IST list, (3) predictions + kickoff lock, (4) scoring engine, (5) results cron + points, (6) leaderboards, (7) tournament bonus + lock, (8) admin tools, (9) hardening + CI.
- **Security (§12):** in-memory token-bucket rate limiting — per-IP on auth (login + logout), per-user on all authenticated writes (incl. admin recompute), returning 429 + `Retry-After`; a 1 MiB request body cap on `/api`; and sliding session refresh (re-issue on activity once >24h old, 7-day window).
- **CSRF — documented decision (§12):** `SameSite=Lax` is the accepted defense for this internal, same-origin, SSO-gated app; no CSRF token is implemented.
- **Trusted proxy (§12):** the per-IP limiter trusts `X-Forwarded-For`/`X-Real-IP` from the frontend nginx; in production the backend must only be reachable via that proxy.
- **Secrets (§12):** `SESSION_SECRET` and `FOOTBALL_DATA_API_KEY` must be rotated before production.
- **CI (§15):** fully implemented per `.github/workflows/ci.yml` — backend (`golangci-lint` + `go vet` + `go test` + `sqlc diff`), frontend (`eslint` + `tsc --noEmit` + `vitest` + `build`), and a build-only Docker job for both images.

---

## 18. GOAT mini-game — scoring & anti-cheat (§3.10)

The mini-game's score is computed in the browser, so SayScore stores and ranks runs but trusts no
client number until it is validated server-side. This section specifies the run lifecycle, the
signed run token, and the pacing model the server uses to reproduce a run's distance. The
authoritative source for the pacing constants is the bundle's `INTEGRATION.md` §8; this section
mirrors it for the Go port.

### 18.1 Run lifecycle (host-driven; the bundle does no I/O)

1. The host renders the game and, when fetching the boards (`GET /api/game/leaderboard`), receives a
   fresh **`run_token`**, which it passes into the bundle (`config.runToken`).
2. The player plays. On game-over the bundle fires `onGameEnd` with `{ distance, coins, durationMs, runToken }`
   — the token echoed verbatim; `durationMs` = **active simulation time**, excluding paused/backgrounded
   time (the bundle pauses on tab-blur).
3. The host submits `POST /api/game/runs` with those fields. The server validates (§18.2–18.4), stores
   one `game_runs` row on success, and returns a **new** `run_token`.
4. The host arms the next run with `handle.setRunToken(next)`. The bundle's "Run Again" restarts
   locally and tokens are single-use, so each run needs a fresh token.

### 18.2 Run token (provenance + anti-replay)

A `run_token` is an HMAC-signed, single-use, short-lived bearer of `{ user_id, jti, issued_at }`,
signed with the existing `SESSION_SECRET` (the same HMAC primitive as the auth cookie,
`internal/auth`). On `POST /api/game/runs` the server:

- verifies the HMAC signature (rejects forged tokens);
- checks the token has not expired (TTL = `GAME_TOKEN_TTL`, default 10 min);
- checks the `jti` has not been consumed — **single-use**, tracked in an in-memory TTL set
  (consistent with the single-instance rate limiter; a restart clears it, which is acceptable);
- checks `user_id` matches the authenticated caller.

Any failing check → **`403`**; the run is not stored. The single-use `jti` prevents replaying one
good run to inflate the coin pool (a `SUM`).

### 18.3 Pacing validation (distance plausibility)

The game is an **auto-runner**: the player controls only jump/duck, **not horizontal speed**, so
distance is a near-deterministic function of active survival time. The server reproduces the
bundle's exact pacing curve and rejects any distance inconsistent with the reported duration.

Bound the duration first, so a client cannot claim more time than physically elapsed:

```
elapsed_ms = now − token.issued_at                              // server-measured, trusted
bounded_ms = min(report.duration_ms, elapsed_ms + GAME_DURATION_SLACK_MS)
expected   = paceDistance(bounded_ms)
accept iff |report.distance − expected| ≤ EPS
EPS        = max(GAME_DIST_EPS_M, GAME_DIST_EPS_FRAC × expected)
```

`EPS` absorbs the small, bounded divergence between the client's variable per-frame `dt` and the
server's uniform integration; a fabricated distance is off by far more. A failing check → **`422`**.
If `GAME_MAX_DISTANCE > 0`, also reject `distance > GAME_MAX_DISTANCE` (the calibrated absolute
ceiling, §18.5).

**Pacing model (port of bundle `INTEGRATION.md` §8).** Constants: `SPEED0=11`, `MAX_SPEED=27`,
`HARD_MAX=35`, `ULTRA_MAX=43` (px / 60fps-frame); `ACCEL=0.0024`; `SCORE_RATE=0.035` (m per
speed-unit·frame); `ULTRA_AT=10000` (m); `FRAME=1000/60` ms.

Per-frame step — the tier is chosen from the *current* state, speed is bumped **first** (clamped to
the tier cap), then distance accrues using the **updated** speed:

```
paceStep(speed, score, dt):
  if   speed < MAX_SPEED:  cap, acc = MAX_SPEED,  ACCEL
  elif score < ULTRA_AT:   cap, acc = HARD_MAX,   ACCEL*0.16
  else:                    cap, acc = ULTRA_MAX,  ACCEL*0.42
  speed = min(cap, speed + acc*dt)
  score = score + speed*SCORE_RATE*dt
  return speed, score
```

Total distance after `active_ms`: integrate from `(SPEED0, 0)` over `frames = active_ms/FRAME` — one
`dt=1` step per whole frame plus one trailing `dt=frac` step — then `floor(score)`.

**Reference points for the Go unit test** (from the bundle's worked table; `durationMs → metres`):
`1000→23`, `2000→46`, `5000→119`, `10000→246`, `15000→380`, `20000→522`, `30000→829`, `60000→1930`,
`90000→3303`, `120000→4939`, `180000→8454`, `300000→16357`, `600000→42306`.

### 18.4 Coins validation

Coins are player-collected, so they get a **band**, not an equality. The bundle gates collectible
spawns by a **minimum spacing** (`MIN_COLLECTIBLE_SPACING_M = 300` m — no two coins spawn closer than
that; the per-frame `COLLECTIBLE_SPAWN_CHANCE = 0.5` only *lowers* real density below this gate), so
the hard ceiling is one coin per `GAME_COIN_MIN_SPACING_M` metres:

```
0 ≤ coins ≤ floor(distance / GAME_COIN_MIN_SPACING_M) + GAME_COIN_SLACK
```

A failing check → **`422`**. (Each collected coin is `+1` to `state.coins`; the legacy `BONUS_M`
constant no longer affects distance/score, so coins are a pure count.)

### 18.5 Residual & tuning

With distance pinned to `paceDistance(duration)` and duration bounded by token age (≤ TTL), the
maximum forgeable distance is `paceDistance(GAME_TOKEN_TTL)` — achievable only by holding a token
open for that much real wall-clock time. Operators should set `GAME_TOKEN_TTL` to just above the
longest plausible real run and, once live, set `GAME_MAX_DISTANCE` to an absolute ceiling calibrated
from real leaderboard data. This is **best-effort anti-cheat for a no-stakes internal feature**;
closing the residual entirely (server-side input replay) is **deferred**.
