import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { GoatConfig, GoatGameHandle } from "chased-by-the-goat";

const mount = vi.hoisted(() =>
  vi.fn<(el: HTMLElement, cfg: GoatConfig) => GoatGameHandle>()
);
vi.mock("chased-by-the-goat", () => ({ mountGoatGame: mount }));
vi.mock("../lib/auth", () => ({ useMe: () => ({ data: { id: 7, name: "Renjith", avatar_url: "" }, isPending: false }) }));

const saveGameRunMock = vi.hoisted(() => vi.fn(async () => ({ best_distance: 1, coin_pool: 1, run_token: "tok-2" })));
vi.mock("../lib/game", () => ({
  useGameLeaderboard: () => ({ data: { distance: [], coins: [], me: { best_distance: 0, coin_pool: 0 }, run_token: "tok-1" }, isPending: false }),
  saveGameRun: saveGameRunMock,
}));

import { GoatGame } from "./GoatGame";

function wrap(ui: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>;
}

describe("GoatGame", () => {
  beforeEach(() => {
    saveGameRunMock.mockResolvedValue({ best_distance: 1, coin_pool: 1, run_token: "tok-2" });
    mount.mockReturnValue({
      setLeaderboard: vi.fn(),
      setCoinLeaderboard: vi.fn(),
      setPlayer: vi.fn(),
      setRunToken: vi.fn(),
      update: vi.fn(),
      destroy: vi.fn(),
    });
  });

  it("mounts the bundle once with the player + run token", async () => {
    render(wrap(<GoatGame />));
    await waitFor(() => expect(mount).toHaveBeenCalledTimes(1));
    const cfg = mount.mock.calls[0][1];
    expect(cfg.player).toMatchObject({ id: "7", name: "Renjith" });
    expect(cfg.runToken).toBe("tok-1");
    expect(typeof cfg.onGameEnd).toBe("function");
  });

  it("destroys the instance on unmount", async () => {
    const { unmount } = render(wrap(<GoatGame />));
    await waitFor(() => expect(mount).toHaveBeenCalled());
    const handle = mount.mock.results[0].value;
    unmount();
    expect(handle.destroy).toHaveBeenCalled();
  });

  it("invalidates the leaderboard query when saveGameRun rejects (stuck-token recovery)", async () => {
    saveGameRunMock.mockRejectedValueOnce(new Error("403 forbidden"));

    const qc = new QueryClient();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    render(<QueryClientProvider client={qc}><GoatGame /></QueryClientProvider>);
    await waitFor(() => expect(mount).toHaveBeenCalled());

    const cfg = mount.mock.calls[mount.mock.calls.length - 1][1];
    // Trigger onGameEnd — saveGameRun is rigged to reject.
    await act(async () => {
      await cfg.onGameEnd({ id: "7", name: "Renjith", distance: 100, coins: 1, durationMs: 5000, timestamp: new Date().toISOString(), runToken: "tok-1" });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["game-leaderboard"] })
    );
  });
});
