import { useState } from "react";
import { BonusPanel } from "../components/BonusPanel";
import { MatchesColumn } from "../components/MatchesColumn";
import { StandingCard } from "../components/StandingCard";
import { LeaderboardPanel } from "../components/LeaderboardPanel";
import { HallOfFame } from "../components/HallOfFame";

type HomeMobileView = "fixtures" | "ranks";

type HomeProps = {
  mobileView?: HomeMobileView;
};

export function Home({ mobileView }: HomeProps) {
  // Mobile: toggle between "left column" (bonus + fixtures) and "ranks" (sidebar)
  const [localMobileView, setLocalMobileView] = useState<HomeMobileView>("fixtures");
  const activeMobileView = mobileView ?? localMobileView;
  const showMobileToggle = mobileView === undefined;

  return (
    <div className="home">
      {/* Mobile toggle — hidden on desktop via CSS */}
      {showMobileToggle && (
        <div className="home__toggle" role="group" aria-label="View">
          <button
            type="button"
            aria-pressed={activeMobileView === "fixtures"}
            className={`home__toggle-btn${activeMobileView === "fixtures" ? " is-active" : ""}`}
            onClick={() => setLocalMobileView("fixtures")}
          >
            Fixtures
          </button>
          <button
            type="button"
            aria-pressed={activeMobileView === "ranks"}
            className={`home__toggle-btn${activeMobileView === "ranks" ? " is-active" : ""}`}
            onClick={() => setLocalMobileView("ranks")}
          >
            Ranks
          </button>
        </div>
      )}

      <div className="mobile-page-head">
        <div className="mobile-page-title">
          {activeMobileView === "fixtures" ? "Predictions" : "Standings"}
        </div>
        <div className="mobile-page-sub">
          {activeMobileView === "fixtures"
            ? "Pick scorelines before kickoff and climb the table."
            : "Where you sit, who's leading, and the weekly winners."}
        </div>
      </div>

      {/* Two-column grid (single column on mobile) */}
      <div className="home__grid page">
        {/* LEFT: Bonus panel + Matches column */}
        <div className={`home__main main-left${activeMobileView === "ranks" ? " is-hidden-mobile" : ""}`}>
          <section className="promo-banner-wrap promo-banner-wrap--split" aria-label="Mini-games">
            <a
              className="promo-banner"
              href="https://d23okley85vr35.cloudfront.net/"
              target="_blank"
              rel="noreferrer"
              aria-label="Open Penalty Shootout in a new tab"
            >
              <img
                className="promo-banner__image"
                src="/penalty-shootout-banner.png"
                alt="Penalty Shootout. One kick. One chance. Be the hero."
              />
            </a>

            <a
              className="promo-banner promo-banner--goat"
              href="/goat-game.html"
              target="_blank"
              rel="noreferrer"
              aria-label="Play Chased by the GOAT in a new tab"
            >
              <img
                className="promo-banner__image"
                src="/goat-game-banner.png"
                alt=""
              />
            </a>
          </section>

          <BonusPanel />
          <MatchesColumn />
        </div>

        {/* RIGHT: Sidebar (standing + leaderboard + hall of fame) */}
        <aside className={`home__aside main-right${activeMobileView === "fixtures" ? " is-hidden-mobile" : ""}`}>
          <StandingCard />
          <LeaderboardPanel />
          <HallOfFame />
        </aside>
      </div>
    </div>
  );
}
