import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Home } from "./Home";

// Stub the data panels so this test targets only the banner wiring.
vi.mock("../components/BonusPanel", () => ({ BonusPanel: () => null }));
vi.mock("../components/MatchesColumn", () => ({ MatchesColumn: () => null }));
vi.mock("../components/StandingCard", () => ({ StandingCard: () => null }));
vi.mock("../components/LeaderboardPanel", () => ({ LeaderboardPanel: () => null }));
vi.mock("../components/HallOfFame", () => ({ HallOfFame: () => null }));

function wrap(ui: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>;
}

describe("Home promo banners", () => {
  it("renders the GOAT card as a new-tab link to /goat-game.html", () => {
    render(wrap(<Home />));
    const link = screen.getByRole("link", { name: /chased by the goat/i });
    expect(link).toHaveAttribute("href", "/goat-game.html");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("keeps Penalty Shootout an external link", () => {
    render(wrap(<Home />));
    const link = screen.getByRole("link", { name: /penalty shootout/i });
    expect(link).toHaveAttribute("target", "_blank");
  });
});
