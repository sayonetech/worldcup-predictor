import { describe, it, expect, vi, beforeEach } from "vitest";
import { getGameLeaderboard, saveGameRun } from "./game";

beforeEach(() => vi.restoreAllMocks());

describe("game api", () => {
  it("fetches the leaderboard with credentials", async () => {
    const board = { distance: [], coins: [], me: { best_distance: 0, coin_pool: 0 }, run_token: "t" };
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(board), { status: 200 }),
    );
    const got = await getGameLeaderboard();
    expect(got.run_token).toBe("t");
    expect(spy).toHaveBeenCalledWith(expect.stringContaining("/game/leaderboard"), expect.objectContaining({ credentials: "include" }));
  });

  it("POSTs a run and returns the next token", async () => {
    const res = { best_distance: 100, coin_pool: 12, run_token: "next" };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(res), { status: 200 }));
    const got = await saveGameRun({ run_token: "t", distance: 100, coins: 12, duration_ms: 5000 });
    expect(got.run_token).toBe("next");
  });

  it("throws on 4xx", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}", { status: 422 }));
    await expect(saveGameRun({ run_token: "t", distance: 9, coins: 0, duration_ms: 1 })).rejects.toThrow();
  });
});
