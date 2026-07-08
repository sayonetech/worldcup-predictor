import { useEffect, useRef, useState } from "react";
import { useMe, GoogleSignInButton, useLogout } from "./lib/auth";
import { Home } from "./routes/Home";
import { Admin } from "./routes/Admin";
import { HowToPlayModal } from "./components/HowToPlayModal";
import { VictoryCelebration } from "./components/VictoryCelebration";
// import { ChatWidget } from "./components/ChatWidget"; // hidden per request; see mount site below
import { ChevronDownIcon, HelpIcon, LogOutIcon, ShieldTabIcon, SparkIcon, StandingsIcon } from "./components/icons";
import { Avatar } from "./components/Avatar";
import { useCelebrations, useMarkCelebrationsSeen, type Celebration } from "./lib/celebrations";
// Brand wordmark from the design handoff (transparent webp — sits on the dark glass topbar)
import sayoneLogo from "./assets/sayone-logo.webp";

// SayOne redesign shell — Predictions + Admin tabs (Bonus is embedded in Home)
type View = "predictions" | "admin";
type MobileTab = "predict" | "standings" | "admin";

function useIsPhone() {
  const [isPhone, setIsPhone] = useState(() => {
    if (typeof window === "undefined" || !window.matchMedia) return false;
    return window.matchMedia("(max-width: 760px)").matches;
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const media = window.matchMedia("(max-width: 760px)");
    const update = () => setIsPhone(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  return isPhone;
}

export default function App() {
  const { data: me, isLoading } = useMe();
  const logout = useLogout();
  const [view, setView] = useState<View>("predictions");
  const [mobileTab, setMobileTab] = useState<MobileTab>("predict");
  const isPhone = useIsPhone();
  // Hooks must be declared unconditionally before any early return.
  const [helpOpen, setHelpOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [logoutConfirm, setLogoutConfirm] = useState(false);
  const profileRef = useRef<HTMLDivElement>(null);
  const isAdmin = me?.role === "admin";
  const { data: celebrations } = useCelebrations(!!me);
  const markSeen = useMarkCelebrationsSeen();
  const [dismissed, setDismissed] = useState(false);

  // Close the profile dropdown on outside-click / Escape.
  useEffect(() => {
    if (!profileOpen) return;

    function closeMenu() {
      setProfileOpen(false);
      setLogoutConfirm(false);
    }
    function onPointerDown(event: PointerEvent) {
      if (!profileRef.current?.contains(event.target as Node)) closeMenu();
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") closeMenu();
    }

    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [profileOpen]);

  // ---- Loading skeleton ----
  if (isLoading) {
    return (
      <div className="app-loading" aria-busy="true" aria-label="Loading SayScore">
        <div className="app-loading__pulse" />
      </div>
    );
  }

  // ---- Unauthenticated gate ----
  if (!me) {
    return (
      <main className="auth">
        <section className="auth__card" aria-labelledby="auth-title">
          <h1 id="auth-title" className="sr-only">SayScore</h1>
          <img
            className="auth__logo"
            src={sayoneLogo}
            alt="SayScore, by SayOne"
          />
          <p className="auth__tagline">
            Predict every FIFA World Cup 2026 match and climb the SayOne leaderboard.
          </p>

          <GoogleSignInButton />

          <p className="auth__note">
            Restricted to <strong>sayonetech.com</strong> accounts.
          </p>
        </section>
      </main>
    );
  }

  // ---- Authenticated shell ----
  const pending = celebrations ?? [];
  const activeCelebration: Celebration | null =
    !dismissed && pending.length > 0 ? pending[0] : null;

  function handleCelebrationDone() {
    setDismissed(true);
    if (pending.length > 0) {
      markSeen.mutate(pending.map((c) => c.match_id));
    }
  }

  const effectiveMobileTab: MobileTab = isAdmin || mobileTab !== "admin" ? mobileTab : "predict";
  const effectiveView: View = isAdmin || view !== "admin" ? view : "predictions";
  const activeView: View = isPhone
    ? effectiveMobileTab === "admin" ? "admin" : "predictions"
    : effectiveView;
  const homeMobileView = isPhone
    ? effectiveMobileTab === "standings" ? "ranks" : "fixtures"
    : undefined;
  const userName = me.name || me.email;

  function selectMobileTab(tab: MobileTab) {
    if (tab === "admin" && !isAdmin) return;
    setMobileTab(tab);
    setView(tab === "admin" ? "admin" : "predictions");
  }

  return (
    <>
      {/* Backdrop layers (app-bg + thunderstorm) live in index.html, outside #root */}

      {/* App shell sits above the backdrop (z-index: 1) */}
      <div className="app">
        {/* ── Topbar: logo (left) · profile dropdown (right). No pill nav —
              Admin / Predictions / How to play / Log out live in the menu. ── */}
        <header className="topbar" role="banner">
          {/* LEFT: brand wordmark (click → predictions) */}
          <button
            type="button"
            className="logo logo--btn"
            aria-label="SayScore — predictions"
            onClick={() => setView("predictions")}
          >
            <img className="logo-slot" src={sayoneLogo} alt="SayScore" />
          </button>

          <div className="topbar-spacer" />

          {/* RIGHT: profile dropdown */}
          <div className="topbar-r">
            <div className="topbar-profile" ref={profileRef}>
              <button
                type="button"
                className="profile-trigger"
                aria-label={`Profile menu for ${userName}`}
                aria-haspopup="menu"
                aria-expanded={profileOpen}
                aria-controls="profile-menu"
                onClick={() => {
                  setProfileOpen((open) => !open);
                  setLogoutConfirm(false);
                }}
              >
                <Avatar name={userName} avatarUrl={me.avatar_url || undefined} size={32} isMe />
                <span className="profile-trigger__text">
                  <span className="profile-trigger__name">{userName}</span>
                  <span className="profile-trigger__role">{isAdmin ? "Admin" : "Player"}</span>
                </span>
                <span className="profile-trigger__chev" aria-hidden="true">
                  <ChevronDownIcon />
                </span>
              </button>

              {profileOpen && (
                <div id="profile-menu" className="profile-menu" role="menu">
                  <div className="profile-menu__identity" aria-label={`Signed in as ${userName}`}>
                    <span className="profile-menu__name">{userName}</span>
                    <span className="profile-menu__email">{me.email}</span>
                  </div>

                  {/* Navigation lives in the menu (no header nav bar) */}
                  {isAdmin && activeView === "admin" && (
                    <button
                      type="button"
                      className="profile-menu__item"
                      role="menuitem"
                      onClick={() => {
                        setView("predictions");
                        setProfileOpen(false);
                      }}
                    >
                      <SparkIcon />
                      <span>Predictions</span>
                    </button>
                  )}
                  {isAdmin && (
                    <button
                      type="button"
                      className={`profile-menu__item${activeView === "admin" ? " is-active" : ""}`}
                      role="menuitem"
                      aria-current={activeView === "admin" ? "page" : undefined}
                      onClick={() => {
                        setView("admin");
                        setProfileOpen(false);
                      }}
                    >
                      <ShieldTabIcon />
                      <span>Admin</span>
                    </button>
                  )}

                  <button
                    type="button"
                    className="profile-menu__item"
                    role="menuitem"
                    onClick={() => {
                      setProfileOpen(false);
                      setLogoutConfirm(false);
                      setHelpOpen(true);
                    }}
                  >
                    <HelpIcon />
                    <span>How to play</span>
                  </button>

                  {!logoutConfirm ? (
                    <button
                      type="button"
                      className="profile-menu__item profile-menu__item--danger"
                      role="menuitem"
                      onClick={() => setLogoutConfirm(true)}
                    >
                      <LogOutIcon />
                      <span>Log out</span>
                    </button>
                  ) : (
                    <div className="profile-menu__confirm" role="group" aria-label="Confirm log out">
                      <p className="profile-menu__confirm-text">Log out of SayScore?</p>
                      <div className="profile-menu__confirm-row">
                        <button
                          type="button"
                          className="profile-menu__cancel"
                          onClick={() => setLogoutConfirm(false)}
                          disabled={logout.isPending}
                          autoFocus
                        >
                          Cancel
                        </button>
                        <button
                          type="button"
                          className="profile-menu__confirm-danger"
                          onClick={() => logout.mutate()}
                          disabled={logout.isPending}
                        >
                          {logout.isPending ? "Logging out…" : "Log out"}
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </header>

        {/* ── Main content ── */}
        {activeView === "predictions" ? (
          <Home mobileView={homeMobileView} />
        ) : (
          <Admin />
        )}

        <nav className="mobile-tabbar" aria-label="Primary mobile navigation">
          <button
            type="button"
            className={`mobile-tab${effectiveMobileTab === "predict" ? " on" : ""}`}
            aria-current={effectiveMobileTab === "predict" ? "page" : undefined}
            onClick={() => selectMobileTab("predict")}
          >
            <span className="mobile-tab__icon"><SparkIcon /></span>
            <span>Predict</span>
          </button>

          <button
            type="button"
            className={`mobile-tab${effectiveMobileTab === "standings" ? " on" : ""}`}
            aria-current={effectiveMobileTab === "standings" ? "page" : undefined}
            onClick={() => selectMobileTab("standings")}
          >
            <span className="mobile-tab__icon"><StandingsIcon /></span>
            <span>Standings</span>
          </button>

          {isAdmin && (
            <button
              type="button"
              className={`mobile-tab${effectiveMobileTab === "admin" ? " on" : ""}`}
              aria-current={effectiveMobileTab === "admin" ? "page" : undefined}
              onClick={() => selectMobileTab("admin")}
            >
              <span className="mobile-tab__icon"><ShieldTabIcon /></span>
              <span>Admin</span>
            </button>
          )}
        </nav>
      </div>

      {/* ── How to Play modal ── */}
      {helpOpen && <HowToPlayModal onClose={() => setHelpOpen(false)} />}

      {activeCelebration && (
        <VictoryCelebration celebration={activeCelebration} onDone={handleCelebrationDone} />
      )}
      {/* ChatWidget hidden per request — launcher unmounted so no one can chat.
          Component and API code are intentionally kept; re-add <ChatWidget /> to restore. */}
      {/* <ChatWidget /> */}
    </>
  );
}
