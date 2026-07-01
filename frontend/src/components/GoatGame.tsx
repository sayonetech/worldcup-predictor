import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { mountGoatGame, type GoatGameHandle, type GoatResult } from "chased-by-the-goat";
import { useMe } from "../lib/auth";
import { useGameLeaderboard, saveGameRun } from "../lib/game";

// First name for the small game discs; when there's no stored name, derive it
// from the email's local part ("hiba.kareem@..." -> "Hiba").
function firstName(me: { name: string; email: string }): string {
  const fromName = me.name?.trim().split(/\s+/)[0];
  if (fromName) return fromName;
  const local = me.email.split("@")[0] ?? "";
  const first = local.split(/[._-]+/).filter(Boolean)[0];
  return first ? first.charAt(0).toUpperCase() + first.slice(1) : "Player";
}

export function GoatGame() {
  const { data: me, isPending: mePending } = useMe();
  const { data: board, isPending: boardPending } = useGameLeaderboard();
  const qc = useQueryClient();
  const hostRef = useRef<HTMLDivElement>(null);
  const handleRef = useRef<GoatGameHandle | null>(null);
  // Keep the freshest token/board in refs so onGameEnd (captured once at mount) reads current values.
  const tokenRef = useRef<string | undefined>(board?.run_token);

  // Mount once, after we have both the player and the first board+token.
  // NOTE: teardown is intentionally NOT here. `board` is a dep (we need the
  // first board+token to mount), and it changes after every run (save →
  // invalidate → refetch). If destroy() lived in this effect's cleanup, each
  // board change would tear down and remount the game — resetting it to a fresh
  // Kick Off landing and wiping the game-over screen. Teardown lives in a
  // dedicated unmount-only effect below; board refreshes update in place.
  useEffect(() => {
    if (!hostRef.current || !me || !board || handleRef.current) return;
    tokenRef.current = board.run_token;
    handleRef.current = mountGoatGame(hostRef.current, {
      player: { id: String(me.id), name: firstName(me), coins: board.me.coin_pool },
      leaderboard: (board.distance ?? []).map((r) => ({ name: r.name, team: r.team ?? "", distance: r.distance ?? 0 })),
      coinLeaderboard: (board.coins ?? []).map((r) => ({ name: r.name, team: r.team ?? "", coins: r.coins ?? 0 })),
      runToken: board.run_token,
      async onGameEnd(result: GoatResult) {
        try {
          const res = await saveGameRun({
            run_token: result.runToken ?? tokenRef.current ?? "",
            distance: result.distance,
            coins: result.coins,
            duration_ms: result.durationMs,
          });
          tokenRef.current = res.run_token;
          handleRef.current?.setRunToken(res.run_token); // arm next run
          await qc.invalidateQueries({ queryKey: ["game-leaderboard"] }); // refetch → effect below pushes boards
        } catch {
          // Save failed (rejected run / network) — refetch so a fresh run_token is armed for the next run.
          void qc.invalidateQueries({ queryKey: ["game-leaderboard"] });
        }
      },
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [me, board]);

  // Teardown only on real unmount — never on a board refresh. Strict-Mode's dev
  // double-invoke still destroys+remounts once at startup, which is fine.
  useEffect(() => {
    return () => {
      handleRef.current?.destroy();
      handleRef.current = null;
    };
  }, []);

  // Push refreshed boards + token in place whenever the query data changes (no remount).
  useEffect(() => {
    if (!handleRef.current || !board) return;
    handleRef.current.setLeaderboard((board.distance ?? []).map((r) => ({ name: r.name, team: r.team ?? "", distance: r.distance ?? 0 })));
    handleRef.current.setCoinLeaderboard((board.coins ?? []).map((r) => ({ name: r.name, team: r.team ?? "", coins: r.coins ?? 0 })));
    if (board.run_token && board.run_token !== tokenRef.current) {
      tokenRef.current = board.run_token;
      handleRef.current.setRunToken(board.run_token);
    }
  }, [board]);

  if (mePending || boardPending) {
    return (
      <div className="goat-host" style={{ width: "100%", padding: "24px 16px" }}>
        <div className="skeleton skeleton--long" style={{ height: "180px", width: "100%", borderRadius: "var(--r-md)" }} />
      </div>
    );
  }

  return <div className="goat-host" ref={hostRef} style={{ width: "100%" }} />;
}
