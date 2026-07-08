import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MatchCard } from "./MatchCard";
import type { MatchDTO } from "../lib/matches";

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

// ── Kickoff helpers (relative to real Date.now so no fake timers needed) ───

/** Returns an ISO UTC string for now + `offsetHours` hours. */
function inHours(offsetHours: number): string {
  return new Date(Date.now() + offsetHours * 3600_000).toISOString();
}

/** Returns an ISO IST string for now + `offsetHours` hours (approximation for display). */
function inHoursIST(offsetHours: number): string {
  const ms = Date.now() + offsetHours * 3600_000 + 5.5 * 3600_000;
  return new Date(ms).toISOString().replace("Z", "+05:30");
}

// Within-window match: kickoff = now + 24 h (inside 72-h window)
const baseMatch: MatchDTO = {
  id: 1,
  match_number: 1,
  stage: "group",
  round: "Group Stage",
  group: "A",
  label: "Group A",
  get kickoff_utc() { return inHours(24); },
  get kickoff_ist() { return inHoursIST(24); },
  status: "scheduled",
  locked: false,
  home: { id: 1, name: "Mexico", code: "MEX" },
  away: { id: 2, name: "South Africa", code: "RSA" },
  venue: { name: "Estadio Azteca", city: "Mexico City", country: "Mexico" },
  home_score: null,
  away_score: null,
  prediction: null,
};

// Outside-window match: kickoff = now + 5 days (> 72 h)
const farMatch: MatchDTO = {
  id: 2,
  match_number: 2,
  stage: "group",
  round: "Group Stage",
  group: "A",
  label: "Group A",
  get kickoff_utc() { return inHours(5 * 24); },
  get kickoff_ist() { return inHoursIST(5 * 24); },
  status: "scheduled",
  locked: false,
  home: { id: 1, name: "Mexico", code: "MEX" },
  away: { id: 2, name: "South Africa", code: "RSA" },
  venue: { name: "Estadio Azteca", city: "Mexico City", country: "Mexico" },
  home_score: null,
  away_score: null,
  prediction: null,
};

afterEach(() => vi.restoreAllMocks());

describe("MatchCard", () => {
  // ── New prediction (no existing pick) ───────────────────────────────────
  it("shows 'Save prediction' button for a new pick (no existing prediction)", () => {
    wrap(<MatchCard match={baseMatch} />);
    expect(screen.getByRole("button", { name: /save prediction/i })).toBeInTheDocument();
  });

  it("saves a new 0-0 prediction (always saveable)", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 0, away_score: 0, penalty_winner_team_id: null, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    wrap(<MatchCard match={baseMatch} />);

    const saveBtn = screen.getByRole("button", { name: /save prediction/i });
    expect(saveBtn).toBeEnabled();
    await user.click(saveBtn);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toContain("/matches/1/prediction");
    expect(opts.method).toBe("PUT");
    expect(JSON.parse(opts.body)).toMatchObject({ home_score: 0, away_score: 0 });
  });

  it("shows 'Update pick' when a prediction already exists and score changes", async () => {
    const withPick: MatchDTO = {
      ...baseMatch,
      prediction: { home_score: 1, away_score: 0, penalty_winner_team_id: null, points: null, penalty_bonus: null },
    };
    const user = userEvent.setup();
    wrap(<MatchCard match={withPick} />);

    // Initially not dirty — should show ghost/disabled save (aria-label: "Update prediction")
    const saveBtn = screen.getByRole("button", { name: /update (pick|prediction)/i });
    expect(saveBtn).toBeDisabled();

    // Increase home score → dirty
    await user.click(screen.getByRole("button", { name: /increase mexico/i }));
    expect(screen.getByRole("button", { name: /update (pick|prediction)/i })).toBeEnabled();
  });

  it("animates a left-team goal from the left controls toward the right flag", async () => {
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={baseMatch} />);

    await user.click(screen.getByRole("button", { name: /increase mexico/i }));

    expect(screen.getByLabelText("Mexico score: 1")).toBeInTheDocument();
    const animation = container.querySelector(".goal-animation");
    expect(animation).toHaveAttribute("data-direction", "left-to-right");
    expect(animation).toHaveAttribute("data-start-side", "left");
    expect(animation).toHaveAttribute("data-target-side", "right");
    expect(animation?.querySelector("img")).toHaveAttribute("src", expect.stringContaining("football"));
  });

  it("animates a right-team goal from the right controls toward the left flag", async () => {
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={baseMatch} />);

    await user.click(screen.getByRole("button", { name: /increase south africa/i }));

    expect(screen.getByLabelText("South Africa score: 1")).toBeInTheDocument();
    const animation = container.querySelector(".goal-animation");
    expect(animation).toHaveAttribute("data-direction", "right-to-left");
    expect(animation).toHaveAttribute("data-start-side", "right");
    expect(animation).toHaveAttribute("data-target-side", "left");
  });

  it("supports repeated fast goal clicks with independent animations", async () => {
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={baseMatch} />);
    const increase = screen.getByRole("button", { name: /increase mexico/i });

    await user.click(increase);
    await user.click(increase);

    expect(screen.getByLabelText("Mexico score: 2")).toBeInTheDocument();
    expect(container.querySelectorAll(".goal-animation")).toHaveLength(2);
  });

  it("uses Argentina Advantage only when the opponent attempts to score against Argentina", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        home_score: 1,
        away_score: 1,
        penalty_winner_team_id: null,
        points: null,
        penalty_bonus: null,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const argentinaVsBrazil: MatchDTO = {
      ...baseMatch,
      id: 3,
      home: { id: 3, name: "Argentina", code: "ARG" },
      away: { id: 4, name: "Brazil", code: "BRA" },
    };
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={argentinaVsBrazil} />);

    await user.click(screen.getByRole("button", { name: /increase argentina/i }));
    const argentinaShot = container.querySelector(".goal-animation");
    expect(argentinaShot).toHaveAttribute("data-animation-mode", "normal");
    expect(argentinaShot).toHaveAttribute("data-end-area", "opponent-flag");
    // Argentina scoring for itself is a normal shot — no VAR stamp.
    expect(argentinaShot).not.toHaveAttribute("data-var-stamp");
    expect(container.querySelectorAll(".goal-var-stamp")).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: /increase brazil/i }));
    expect(screen.getByLabelText("Brazil score: 1")).toBeInTheDocument();
    const shots = container.querySelectorAll(".goal-animation");
    expect(shots[1]).toHaveAttribute("data-animation-mode", "argentina-advantage");
    expect(shots[1]).toHaveAttribute("data-start-side", "right");
    expect(shots[1]).toHaveAttribute("data-target-side", "left");
    expect(shots[1]).toHaveAttribute("data-end-area", "center");
    expect(shots[1]).toHaveAttribute("data-impact-side", "left");
    expect(shots[1]).toHaveAttribute(
      "data-motion-phases",
      "approach impact upward-rebound momentum-decay",
    );
    expect(shots[1]).toHaveAttribute("data-bounce-count", "0");
    expect(shots[1].querySelector(".goal-force-wall")).toBeInTheDocument();
    // The opponent's denied shot at Argentina now carries a VAR NO GOAL stamp.
    expect(shots[1]).toHaveAttribute("data-var-stamp", "true");
    expect(container.querySelectorAll(".goal-var-stamp")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: /save prediction/i }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({
      home_score: 1,
      away_score: 1,
    });
  });

  it("mirrors the Argentina wall impact when Argentina is the right team", async () => {
    const brazilVsArgentina: MatchDTO = {
      ...baseMatch,
      id: 5,
      home: { id: 4, name: "Brazil", code: "BRA" },
      away: { id: 3, name: "Argentina", code: "ARG" },
    };
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={brazilVsArgentina} />);

    await user.click(screen.getByRole("button", { name: /increase brazil/i }));

    expect(screen.getByLabelText("Brazil score: 1")).toBeInTheDocument();
    const shot = container.querySelector(".goal-animation");
    expect(shot).toHaveAttribute("data-animation-mode", "argentina-advantage");
    expect(shot).toHaveAttribute("data-direction", "left-to-right");
    expect(shot).toHaveAttribute("data-impact-side", "right");
    expect(shot).toHaveAttribute("data-bounce-count", "0");
  });

  it("keeps normal goal animations when Argentina is not playing", async () => {
    const brazilVsGermany: MatchDTO = {
      ...baseMatch,
      id: 4,
      home: { id: 4, name: "Brazil", code: "BRA" },
      away: { id: 5, name: "Germany", code: "GER" },
    };
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={brazilVsGermany} />);

    await user.click(screen.getByRole("button", { name: /increase brazil/i }));
    await user.click(screen.getByRole("button", { name: /increase germany/i }));

    const shots = container.querySelectorAll(".goal-animation");
    expect(shots).toHaveLength(2);
    expect(shots[0]).toHaveAttribute("data-animation-mode", "normal");
    expect(shots[1]).toHaveAttribute("data-animation-mode", "normal");
  });

  // ── Referee approval (backing Argentina) ─────────────────────────────────
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

  // ── Locked state ─────────────────────────────────────────────────────────
  it("renders locked state read-only when match.locked=true", () => {
    wrap(<MatchCard match={{ ...baseMatch, locked: true }} />);
    // Locked pill present
    expect(screen.getByText("Locked")).toBeInTheDocument();
    // Steppers should not be visible (read-only locked state)
    expect(screen.queryByRole("button", { name: /increase mexico/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /save prediction/i })).toBeNull();
    // Shows "Pick locked · —" (no prediction)
    expect(screen.getByText(/pick locked/i)).toBeInTheDocument();
  });

  it("shows locked pick score when match is locked with a prediction", () => {
    wrap(<MatchCard match={{
      ...baseMatch,
      locked: true,
      prediction: { home_score: 2, away_score: 1, penalty_winner_team_id: null, points: 5, penalty_bonus: null },
    }} />);
    expect(screen.getByText(/pick locked.*2.+1/i)).toBeInTheDocument(); // "Pick locked · 2–1"
  });

  // ── Server 409 lock ───────────────────────────────────────────────────────
  it("shows lock alert and disables save on server 409", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: async () => ({ error: "match is locked" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    wrap(<MatchCard match={baseMatch} />);

    await user.click(screen.getByRole("button", { name: /save prediction/i }));
    await screen.findByRole("alert");
    expect(screen.getByRole("alert")).toHaveTextContent(/locked at kickoff/i);
  });

  // ── Server 422 prediction window error ────────────────────────────────────
  it("shows window-closed alert on server 422 with prediction-window message", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      json: async () => ({ error: "predictions open 3 days before kickoff" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    wrap(<MatchCard match={baseMatch} />);

    await user.click(screen.getByRole("button", { name: /save prediction/i }));
    await screen.findByRole("alert");
    expect(screen.getByRole("alert")).toHaveTextContent(/not open yet/i);
  });

  // ── Saved flash ───────────────────────────────────────────────────────────
  it("shows 'Saved' flash after successful save", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 0, away_score: 0, penalty_winner_team_id: null, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    wrap(<MatchCard match={baseMatch} />);

    await user.click(screen.getByRole("button", { name: /save prediction/i }));
    await waitFor(() => expect(screen.queryByText(/Saved/i)).toBeInTheDocument());
  });

  // ── TBD match ─────────────────────────────────────────────────────────────
  it("renders TBD matches non-editable (no stepper, no save button)", () => {
    wrap(<MatchCard match={{ ...baseMatch, home: null, away: null, label: "W74 vs W77", stage: "knockout" }} />);
    expect(screen.getByText("W74 vs W77")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /increase/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /save prediction/i })).toBeNull();
  });

  // ── Knockout penalty control ───────────────────────────────────────────────
  it("shows shootout-winner control on knockout draw", () => {
    const ko: MatchDTO = {
      ...baseMatch,
      id: 90,
      stage: "knockout",
      round: "Round of 16",
      group: "",
      label: "Round of 16",
      home: { id: 1, name: "Brazil", code: "BRA" },
      away: { id: 2, name: "Spain", code: "ESP" },
    };
    wrap(<MatchCard match={ko} />);
    // Default 0-0 → draw → penalty control visible
    expect(screen.getByRole("group", { name: /shootout winner/i })).toBeInTheDocument();
  });

  it("hides shootout-winner control when score is not a draw", async () => {
    const ko: MatchDTO = {
      ...baseMatch,
      id: 90,
      stage: "knockout",
      round: "Round of 16",
      group: "",
      label: "Round of 16",
      home: { id: 1, name: "Brazil", code: "BRA" },
      away: { id: 2, name: "Spain", code: "ESP" },
    };
    const user = userEvent.setup();
    wrap(<MatchCard match={ko} />);

    // Initially a draw (0-0) → visible
    expect(screen.getByRole("group", { name: /shootout winner/i })).toBeInTheDocument();

    // Increase Brazil score → 1-0 → no longer a draw
    await user.click(screen.getByRole("button", { name: /increase brazil/i }));
    expect(screen.queryByRole("group", { name: /shootout winner/i })).toBeNull();
  });

  it("sends penalty_winner_team_id in payload on knockout draw", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ home_score: 0, away_score: 0, penalty_winner_team_id: 2, points: null, penalty_bonus: null }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    const ko: MatchDTO = {
      ...baseMatch,
      id: 90,
      stage: "knockout",
      round: "Round of 16",
      group: "",
      label: "Round of 16",
      home: { id: 1, name: "Brazil", code: "BRA" },
      away: { id: 2, name: "Spain", code: "ESP" },
    };
    wrap(<MatchCard match={ko} />);

    // Pick Spain as shootout winner
    await user.click(screen.getByRole("button", { name: "ESP" }));
    await user.click(screen.getByRole("button", { name: /save prediction/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body.penalty_winner_team_id).toBe(2);
  });

  // ── Prediction window gate (>3 days out) ──────────────────────────────────
  it("shows 'Predict' button (not steppers) for a match >3 days out", () => {
    wrap(<MatchCard match={farMatch} />);
    // Should show the Predict button, not steppers
    expect(screen.getByRole("button", { name: /predict.*not open yet/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /increase mexico/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /save prediction/i })).toBeNull();
  });

  it("shows 'Opens ... IST' hint text for a match >3 days out", () => {
    wrap(<MatchCard match={farMatch} />);
    // The hint text reads "Opens {date} IST"
    const hintEl = document.querySelector(".mc-opens-hint");
    expect(hintEl).toBeInTheDocument();
    expect(hintEl?.textContent).toMatch(/opens/i);
    expect(hintEl?.textContent).toMatch(/IST/);
  });

  it("clicking Predict on >3-day match opens the 'Not open yet' dialog", async () => {
    const user = userEvent.setup();
    wrap(<MatchCard match={farMatch} />);

    const predictBtn = screen.getByRole("button", { name: /predict.*not open yet/i });
    await user.click(predictBtn);

    // Dialog should appear
    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(dialog).toHaveAttribute("aria-modal", "true");
  });

  it("dialog shows correct title 'Not open yet'", async () => {
    const user = userEvent.setup();
    wrap(<MatchCard match={farMatch} />);

    await user.click(screen.getByRole("button", { name: /predict.*not open yet/i }));

    expect(screen.getByRole("heading", { name: /not open yet/i })).toBeInTheDocument();
  });

  it("dialog body contains home vs away team names and IST open date", async () => {
    const user = userEvent.setup();
    wrap(<MatchCard match={farMatch} />);

    await user.click(screen.getByRole("button", { name: /predict.*not open yet/i }));

    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent).toMatch(/Mexico/i);
    expect(dialog.textContent).toMatch(/South Africa/i);
    expect(dialog.textContent).toMatch(/IST/);
    expect(dialog.textContent).toMatch(/3 days/i);
  });

  it("dialog closes when OK is clicked", async () => {
    const user = userEvent.setup();
    wrap(<MatchCard match={farMatch} />);

    await user.click(screen.getByRole("button", { name: /predict.*not open yet/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^ok$/i }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("dialog closes when Escape is pressed", async () => {
    const user = userEvent.setup();
    wrap(<MatchCard match={farMatch} />);

    await user.click(screen.getByRole("button", { name: /predict.*not open yet/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("dialog closes when backdrop is clicked", async () => {
    const user = userEvent.setup();
    const { container } = wrap(<MatchCard match={farMatch} />);

    await user.click(screen.getByRole("button", { name: /predict.*not open yet/i }));
    const overlay = container.querySelector(".pw-overlay") as HTMLElement;
    expect(overlay).toBeInTheDocument();

    await user.click(overlay);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows normal editor (steppers + save) for match within 3-day window", () => {
    wrap(<MatchCard match={baseMatch} />);
    // Within-window match shows steppers
    expect(screen.getByRole("button", { name: /increase mexico/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save prediction/i })).toBeInTheDocument();
    // No "Predict" gate button
    expect(screen.queryByRole("button", { name: /predict.*not open yet/i })).toBeNull();
  });
});
