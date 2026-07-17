package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/bonus"
	"github.com/sayonetech/worldcup-predictor/backend/internal/store"
)

// bonusLockAt fetches the live bonus lock time from the settings service.
// On error it returns zero time + false — callers must treat this as LOCKED
// (fail safe: never silently treat an error as unlocked).
func (d *Deps) bonusLockAt(r *http.Request) (time.Time, bool) {
	t, err := d.Settings.BonusLockAt(r.Context())
	if err != nil {
		slog.Error("bonus: read bonus_lock_at from settings", "err", err)
		return time.Time{}, false
	}
	return t, true
}

type bonusPickDTO struct {
	Category string `json:"category"`
	RefType  string `json:"ref_type"`
	RefID    int64  `json:"ref_id"`
	Label    string `json:"label"`
	Points   *int64 `json:"points,omitempty"`
}

type bonusResponse struct {
	LockAt string         `json:"lock_at"`
	Locked bool           `json:"locked"`
	Picks  []bonusPickDTO `json:"picks"`
}

type bonusUserPicksDTO struct {
	UserID    int64          `json:"user_id"`
	Name      string         `json:"name"`
	AvatarURL string         `json:"avatar_url"`
	IsMe      bool           `json:"is_me"`
	Picks     []bonusPickDTO `json:"picks"`
}

// GetAllBonusPredictions reveals every player's tournament-bonus picks — but only
// once picks lock at BONUS_LOCK_AT. Before that boundary it returns 403, mirroring
// the kickoff gate on match predictions (privacy, spec §4). Read-only.
func (d *Deps) GetAllBonusPredictions(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	lock, ok := d.bonusLockAt(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "settings unavailable")
		return
	}
	// Privacy gate: hidden until picks lock — same boundary as the write lock.
	if now().Before(lock) {
		writeError(w, http.StatusForbidden, "bonus picks are hidden until lock")
		return
	}

	rows, err := d.Bonus.ListBonusPredictionsWithUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bonus picks")
		return
	}

	out := make([]bonusUserPicksDTO, 0)
	index := make(map[int64]int, len(rows))
	for _, row := range rows {
		i, seen := index[row.UserID]
		if !seen {
			out = append(out, bonusUserPicksDTO{
				UserID:    row.UserID,
				Name:      row.Name,
				AvatarURL: row.AvatarURL,
				IsMe:      row.UserID == u.ID,
				Picks:     []bonusPickDTO{},
			})
			i = len(out) - 1
			index[row.UserID] = i
		}
		cat := bonus.Category(row.Category)
		out[i].Picks = append(out[i].Picks, bonusPickDTO{
			Category: row.Category,
			RefType:  string(bonus.RefTypeOf(cat)),
			RefID:    row.RefID,
			Label:    d.resolveRefLabel(r, cat, row.RefID),
			Points:   row.Points,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetBonus returns the caller's bonus picks and the current lock state.
func (d *Deps) GetBonus(w http.ResponseWriter, r *http.Request) {
	lock, ok := d.bonusLockAt(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "settings unavailable")
		return
	}
	u, _ := userFromContext(r.Context())
	picks, err := d.Bonus.ListBonusPredictionsForUser(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bonus picks")
		return
	}
	out := make([]bonusPickDTO, 0, len(picks))
	for _, p := range picks {
		cat := bonus.Category(p.Category)
		refType := bonus.RefTypeOf(cat)
		out = append(out, bonusPickDTO{
			Category: p.Category,
			RefType:  string(refType),
			RefID:    p.RefID,
			Label:    d.resolveRefLabel(r, cat, p.RefID),
			Points:   p.Points,
		})
	}
	writeJSON(w, http.StatusOK, bonusResponse{
		LockAt: lock.Format(time.RFC3339),
		Locked: !now().Before(lock),
		Picks:  out,
	})
}

type putBonusRequest struct {
	Picks []struct {
		Category string `json:"category"`
		RefID    int64  `json:"ref_id"`
	} `json:"picks"`
}

// PutBonus upserts the caller's bonus picks. Server-authoritative lock: rejects
// all writes when now >= BonusLockAt. Validates all picks before writing any.
func (d *Deps) PutBonus(w http.ResponseWriter, r *http.Request) {
	lock, ok := d.bonusLockAt(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "settings unavailable")
		return
	}
	if !now().Before(lock) { // server-authoritative lock
		writeError(w, http.StatusForbidden, "bonus predictions are locked")
		return
	}
	u, _ := userFromContext(r.Context())
	var req putBonusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// validate all before writing any
	picks := make([]store.BonusPickWrite, 0, len(req.Picks))
	for _, p := range req.Picks {
		c := bonus.Category(p.Category)
		if !bonus.Valid(c) {
			writeError(w, http.StatusBadRequest, "invalid category")
			return
		}
		ok, err := d.refExists(r, c, p.RefID)
		if err != nil {
			slog.Error("bonus ref validation failed", "err", err)
			writeError(w, http.StatusInternalServerError, "validation failed")
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid pick for category")
			return
		}
		picks = append(picks, store.BonusPickWrite{Category: p.Category, RefID: p.RefID})
	}
	if err := d.Bonus.UpsertBonusPredictions(r.Context(), u.ID, picks); err != nil {
		slog.Error("could not save bonus picks", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save picks")
		return
	}
	d.GetBonus(w, r) // return the updated set + lock state
}

// refExists checks that refID exists in the correct table for the category's ref-type.
func (d *Deps) refExists(r *http.Request, c bonus.Category, refID int64) (bool, error) {
	if bonus.RefTypeOf(c) == bonus.RefTeam {
		return d.Bonus.TeamExists(r.Context(), refID)
	}
	return d.Bonus.PlayerExists(r.Context(), refID)
}

// GetTeams returns all non-placeholder teams for the bonus team-award dropdowns.
func (d *Deps) GetTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := d.Players.ListTeamsForPicker(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load teams")
		return
	}
	type teamDTO struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Code string `json:"code"`
	}
	out := make([]teamDTO, 0, len(teams))
	for _, x := range teams {
		out = append(out, teamDTO{x.ID, x.Name, x.Code})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetPlayers searches players by name for the bonus player-award searchbox.
func (d *Deps) GetPlayers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	players, err := d.Players.SearchPlayers(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not search players")
		return
	}
	type playerDTO struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		TeamCode string `json:"team_code"`
		Position string `json:"position"`
	}
	out := make([]playerDTO, 0, len(players))
	for _, x := range players {
		out = append(out, playerDTO{x.ID, x.Name, x.TeamCode, x.Position})
	}
	writeJSON(w, http.StatusOK, out)
}
