package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sayonetech/worldcup-predictor/backend/internal/store"
)

// TestGetBonus_SettingsErrorFailsSafe + TestPutBonus_SettingsErrorFailsSafe lock
// the most security-critical branch: if the settings store can't be read, the
// bonus lock must NOT fall open — the handler returns 500 and writes nothing.
func TestGetBonus_SettingsErrorFailsSafe(t *testing.T) {
	d := &Deps{Bonus: &fakeBonusStore{}, Players: &fakePlayerStore{}, Settings: &fakeSettings{lockErr: errors.New("settings down")}}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus", nil), 1)
	rec := httptest.NewRecorder()
	d.GetBonus(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (must not fall open on a settings error)", rec.Code)
	}
}

func TestPutBonus_SettingsErrorFailsSafe(t *testing.T) {
	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st, Players: &fakePlayerStore{}, Settings: &fakeSettings{lockErr: errors.New("settings down")}}
	body := `{"picks":[{"category":"winner","ref_id":9}]}`
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(body)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (must not write when lock state is unknown)", rec.Code)
	}
	if len(st.upserts) != 0 {
		t.Errorf("must not write picks when settings unreadable; got %d upserts", len(st.upserts))
	}
}

// fakeBonusStore implements store.BonusStore for handler tests.
type fakeBonusStore struct {
	picks   []store.BonusPick
	upserts []struct {
		cat string
		ref int64
	}
	teamOK       bool
	playerOK     bool
	resultsSaved int
	results      []store.BonusResult
	allPicks     []store.BonusUserPickRow
}

func (f *fakeBonusStore) UpsertBonusPrediction(_ context.Context, _ int64, cat string, ref int64) error {
	f.upserts = append(f.upserts, struct {
		cat string
		ref int64
	}{cat, ref})
	return nil
}
func (f *fakeBonusStore) UpsertBonusPredictions(_ context.Context, _ int64, picks []store.BonusPickWrite) error {
	for _, p := range picks {
		f.upserts = append(f.upserts, struct {
			cat string
			ref int64
		}{p.Category, p.RefID})
	}
	return nil
}
func (f *fakeBonusStore) ListBonusPredictionsForUser(context.Context, int64) ([]store.BonusPick, error) {
	return f.picks, nil
}
func (f *fakeBonusStore) ListBonusPredictionsWithUsers(context.Context) ([]store.BonusUserPickRow, error) {
	return f.allPicks, nil
}
func (f *fakeBonusStore) UpsertBonusResult(context.Context, string, int64) error {
	f.resultsSaved++
	return nil
}
func (f *fakeBonusStore) ListBonusResults(context.Context) ([]store.BonusResult, error) {
	return f.results, nil
}
func (f *fakeBonusStore) ListAllBonusPredictions(context.Context) ([]store.BonusPredictionRow, error) {
	return nil, nil
}
func (f *fakeBonusStore) SetBonusPredictionPoints(context.Context, int64, int64) error { return nil }
func (f *fakeBonusStore) TeamExists(context.Context, int64) (bool, error)              { return f.teamOK, nil }
func (f *fakeBonusStore) PlayerExists(context.Context, int64) (bool, error)            { return f.playerOK, nil }

// fakePlayerStore implements store.PlayerStore for picker tests.
type fakePlayerStore struct {
	teams       []store.TeamOption
	players     []store.PlayerOption
	teamNames   map[int64]string
	playerNames map[int64]string
}

func (f *fakePlayerStore) ListTeamsForPicker(context.Context) ([]store.TeamOption, error) {
	return f.teams, nil
}
func (f *fakePlayerStore) SearchPlayers(_ context.Context, _ string) ([]store.PlayerOption, error) {
	return f.players, nil
}
func (f *fakePlayerStore) TeamNameByID(_ context.Context, id int64) (string, error) {
	if f.teamNames != nil {
		return f.teamNames[id], nil
	}
	return "", nil
}
func (f *fakePlayerStore) PlayerNameByID(_ context.Context, id int64) (string, error) {
	if f.playerNames != nil {
		return f.playerNames[id], nil
	}
	return "", nil
}

// ctxUser injects a store.User into the request context (simulates RequireAuth).
func ctxUser(req *http.Request, id int64) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), userCtxKey, store.User{ID: id}))
}

func TestPutBonus_BeforeLockUpserts(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}} // 23:59 IST
	body := `{"picks":[{"category":"winner","ref_id":9},{"category":"golden_boot","ref_id":42}]}`
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(body)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(st.upserts) != 2 {
		t.Fatalf("upserts = %d, want 2", len(st.upserts))
	}
}

func TestPutBonus_AfterLockRejected(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(`{"picks":[{"category":"winner","ref_id":9}]}`)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(st.upserts) != 0 {
		t.Errorf("must not write after lock; got %d upserts", len(st.upserts))
	}
}

func TestPutBonus_AtExactLockBoundaryRejected(t *testing.T) {
	old := now
	// Exactly at lock time: now == BonusLockAt → !now().Before(lockAt) → rejected
	lockAt := time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)
	now = func() time.Time { return lockAt }
	t.Cleanup(func() { now = old })

	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st, Settings: &fakeSettings{lockAt: lockAt}}
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(`{"picks":[{"category":"winner","ref_id":9}]}`)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 at exact boundary", rec.Code)
	}
}

func TestPutBonus_WrongRefType(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
	// team category but the team doesn't exist (teamOK=false) -> 400
	st := &fakeBonusStore{teamOK: false, playerOK: true}
	d := &Deps{Bonus: st, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(`{"picks":[{"category":"winner","ref_id":999}]}`)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPutBonus_UnknownCategory(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
	d := &Deps{Bonus: &fakeBonusStore{teamOK: true, playerOK: true}, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/bonus", strings.NewReader(`{"picks":[{"category":"most_assists","ref_id":1}]}`)), 1)
	rec := httptest.NewRecorder()
	d.PutBonus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetBonus_ReturnsLockState(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
	st := &fakeBonusStore{picks: []store.BonusPick{{Category: "winner", RefID: 9}}}
	fp := &fakePlayerStore{teamNames: map[int64]string{9: "Brazil"}}
	d := &Deps{Bonus: st, Players: fp, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus", nil), 1)
	rec := httptest.NewRecorder()
	d.GetBonus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Locked bool `json:"locked"`
		Picks  []struct {
			Category string `json:"category"`
			RefID    int64  `json:"ref_id"`
		} `json:"picks"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got.Locked {
		t.Error("should not be locked on 20 Jun")
	}
	if len(got.Picks) != 1 || got.Picks[0].Category != "winner" {
		t.Errorf("picks = %+v", got.Picks)
	}
}

// TestGetBonus_LabelResolvedPerRefType asserts that each pick in the response
// carries a resolved label — team name for team-award categories, player name
// for player-award categories (spec §11 label field).
func TestGetBonus_LabelResolvedPerRefType(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	picks := []store.BonusPick{
		{Category: "winner", RefID: 9},       // team award → TeamNameByID
		{Category: "golden_ball", RefID: 42}, // player award → PlayerNameByID
	}
	st := &fakeBonusStore{picks: picks}
	fp := &fakePlayerStore{
		teamNames:   map[int64]string{9: "Brazil"},
		playerNames: map[int64]string{42: "Messi"},
	}
	d := &Deps{Bonus: st, Players: fp, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}

	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus", nil), 1)
	rec := httptest.NewRecorder()
	d.GetBonus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got struct {
		Picks []struct {
			Category string `json:"category"`
			RefID    int64  `json:"ref_id"`
			Label    string `json:"label"`
		} `json:"picks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Picks) != 2 {
		t.Fatalf("picks count = %d, want 2", len(got.Picks))
	}

	byCategory := make(map[string]string, len(got.Picks))
	for _, p := range got.Picks {
		byCategory[p.Category] = p.Label
	}

	if got := byCategory["winner"]; got != "Brazil" {
		t.Errorf("winner label = %q, want %q", got, "Brazil")
	}
	if got := byCategory["golden_ball"]; got != "Messi" {
		t.Errorf("golden_ball label = %q, want %q", got, "Messi")
	}
}

// TestGetBonus_StaleRefIDDegradesToEmptyLabel asserts that an unknown ref_id
// produces an empty label string rather than failing the whole request.
func TestGetBonus_StaleRefIDDegradesToEmptyLabel(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })

	picks := []store.BonusPick{{Category: "winner", RefID: 999}} // unknown team
	st := &fakeBonusStore{picks: picks}
	fp := &fakePlayerStore{teamNames: map[int64]string{}} // id 999 not present → ""
	d := &Deps{Bonus: st, Players: fp, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}

	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus", nil), 1)
	rec := httptest.NewRecorder()
	d.GetBonus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even with unknown ref_id, got %d", rec.Code, rec.Code)
	}
	var got struct {
		Picks []struct {
			Label string `json:"label"`
		} `json:"picks"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Picks) != 1 {
		t.Fatalf("picks count = %d, want 1", len(got.Picks))
	}
	if got.Picks[0].Label != "" {
		t.Errorf("label = %q, want empty string for unknown ref_id", got.Picks[0].Label)
	}
}

func TestGetBonus_LockedAfterLockAt(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
	st := &fakeBonusStore{picks: []store.BonusPick{{Category: "runner_up", RefID: 5}}}
	fp := &fakePlayerStore{teamNames: map[int64]string{5: "France"}}
	d := &Deps{Bonus: st, Players: fp, Settings: &fakeSettings{lockAt: time.Date(2026, 6, 28, 18, 29, 0, 0, time.UTC)}}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/bonus", nil), 1)
	rec := httptest.NewRecorder()
	d.GetBonus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Locked bool `json:"locked"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if !got.Locked {
		t.Error("should be locked on 29 Jun")
	}
}

func TestPutBonusResults_ValidSaves(t *testing.T) {
	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st}
	body := `{"results":[{"category":"winner","ref_id":9},{"category":"golden_boot","ref_id":42}]}`
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/admin/bonus/results", strings.NewReader(body)), 1)
	rec := httptest.NewRecorder()
	d.PutBonusResults(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if st.resultsSaved != 2 {
		t.Errorf("resultsSaved = %d, want 2", st.resultsSaved)
	}
}

func TestPutBonusResults_BadSecondEntryWritesNothing(t *testing.T) {
	// validate-all-then-write: the 2nd entry is an unknown category, so the whole
	// batch is rejected (400) and NO outcome is written.
	st := &fakeBonusStore{teamOK: true, playerOK: true}
	d := &Deps{Bonus: st}
	body := `{"results":[{"category":"winner","ref_id":9},{"category":"most_assists","ref_id":1}]}`
	req := ctxUser(httptest.NewRequest(http.MethodPut, "/api/admin/bonus/results", strings.NewReader(body)), 1)
	rec := httptest.NewRecorder()
	d.PutBonusResults(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if st.resultsSaved != 0 {
		t.Errorf("must write nothing on a rejected batch; got %d", st.resultsSaved)
	}
}

func TestGetTeams_Returns200(t *testing.T) {
	fp := &fakePlayerStore{
		teams: []store.TeamOption{
			{ID: 1, Name: "Brazil", Code: "BRA"},
			{ID: 2, Name: "France", Code: "FRA"},
		},
	}
	d := &Deps{Players: fp}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/teams", nil), 1)
	rec := httptest.NewRecorder()
	d.GetTeams(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got) != 2 {
		t.Fatalf("teams count = %d, want 2", len(got))
	}
}

func TestGetPlayers_Returns200(t *testing.T) {
	fp := &fakePlayerStore{
		players: []store.PlayerOption{
			{ID: 10, Name: "Messi", Position: "Forward", TeamCode: "ARG"},
		},
	}
	d := &Deps{Players: fp}
	req := ctxUser(httptest.NewRequest(http.MethodGet, "/api/players?q=mes", nil), 1)
	rec := httptest.NewRecorder()
	d.GetPlayers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got) != 1 {
		t.Fatalf("players count = %d, want 1", len(got))
	}
}
