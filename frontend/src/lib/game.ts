import { useQuery } from "@tanstack/react-query";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";

export type GameBoardRow = {
  user_id: number;
  name: string;
  avatar_url: string;
  team?: string;
  distance?: number;
  coins?: number;
};

export type GameLeaderboard = {
  distance: GameBoardRow[];
  coins: GameBoardRow[];
  me: { best_distance: number; coin_pool: number };
  run_token: string;
};

export type SaveRunInput = { run_token: string; distance: number; coins: number; duration_ms: number };
export type SaveRunResult = { best_distance: number; coin_pool: number; run_token: string };

export async function getGameLeaderboard(): Promise<GameLeaderboard> {
  const res = await fetch(`${BASE}/game/leaderboard`, { credentials: "include" });
  if (!res.ok) throw new Error(`game leaderboard failed: ${res.status}`);
  return res.json() as Promise<GameLeaderboard>;
}

export async function saveGameRun(input: SaveRunInput): Promise<SaveRunResult> {
  const res = await fetch(`${BASE}/game/runs`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(`save run failed: ${res.status}`);
  return res.json() as Promise<SaveRunResult>;
}

export function useGameLeaderboard() {
  return useQuery({ queryKey: ["game-leaderboard"], queryFn: getGameLeaderboard });
}
