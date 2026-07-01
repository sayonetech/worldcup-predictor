import { useMe } from "../lib/auth";
import { GoatGame } from "./GoatGame";

export function GoatGamePage() {
  const { data: me, isLoading } = useMe();

  if (isLoading) {
    return (
      <div className="game-page game-page--center">
        <div className="skeleton skeleton--long" style={{ height: "72px", width: "100%", maxWidth: "420px", borderRadius: "var(--r-md)" }} />
      </div>
    );
  }

  if (!me) {
    return (
      <div className="game-page game-page--center">
        <p className="game-page__unauth">
          <a className="game-page__back-link" href="/">
            Sign in to SayScore to play
          </a>
        </p>
      </div>
    );
  }

  return (
    <div className="game-page">
      <GoatGame />
    </div>
  );
}
