package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sayonetech/worldcup-predictor/backend/internal/store/sqlc"
)

// BonusPick is one user's stored pick for a category (Points is nil until scored).
type BonusPick struct {
	Category string
	RefID    int64
	Points   *int64
}

// BonusResult is one stored award outcome.
type BonusResult struct {
	Category string
	RefID    int64
}

// BonusPredictionRow is a flat bonus_predictions row used by the scoring job.
type BonusPredictionRow struct {
	ID       int64
	Category string
	RefID    int64
}

// BonusPickWrite is one validated pick to write atomically in UpsertBonusPredictions.
type BonusPickWrite struct {
	Category string
	RefID    int64
}

// BonusUserPickRow is one user's single bonus pick alongside that player's
// identity. Used to reveal all users' bonus picks after BONUS_LOCK_AT (spec §4).
type BonusUserPickRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Category  string
	RefID     int64
	Points    *int64
}

// BonusStore is the read/write surface for tournament-bonus picks + outcomes.
type BonusStore interface {
	UpsertBonusPrediction(ctx context.Context, userID int64, category string, refID int64) error
	UpsertBonusPredictions(ctx context.Context, userID int64, picks []BonusPickWrite) error
	ListBonusPredictionsForUser(ctx context.Context, userID int64) ([]BonusPick, error)
	ListBonusPredictionsWithUsers(ctx context.Context) ([]BonusUserPickRow, error)
	UpsertBonusResult(ctx context.Context, category string, refID int64) error
	ListBonusResults(ctx context.Context) ([]BonusResult, error)
	ListAllBonusPredictions(ctx context.Context) ([]BonusPredictionRow, error)
	SetBonusPredictionPoints(ctx context.Context, id int64, points int64) error
	TeamExists(ctx context.Context, id int64) (bool, error)
	PlayerExists(ctx context.Context, id int64) (bool, error)
}

var _ BonusStore = (*SQLStore)(nil)

func (s *SQLStore) UpsertBonusPrediction(ctx context.Context, userID int64, category string, refID int64) error {
	if err := s.q.UpsertBonusPrediction(ctx, sqlc.UpsertBonusPredictionParams{
		UserID: userID, Category: sqlc.BonusPredictionsCategory(category), RefID: refID,
	}); err != nil {
		return fmt.Errorf("store: upsert bonus prediction: %w", err)
	}
	return nil
}

// UpsertBonusPredictions writes all picks for a user atomically in a single
// transaction, mirroring the UpsertWeeklyResults pattern. A mid-loop error
// rolls back the entire batch so no partial write is committed.
func (s *SQLStore) UpsertBonusPredictions(ctx context.Context, userID int64, picks []BonusPickWrite) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin bonus picks tx: %w", err)
	}
	q := s.q.WithTx(tx)
	for _, p := range picks {
		if err := q.UpsertBonusPrediction(ctx, sqlc.UpsertBonusPredictionParams{
			UserID: userID, Category: sqlc.BonusPredictionsCategory(p.Category), RefID: p.RefID,
		}); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: upsert bonus predictions: %w", err)
		}
	}
	return tx.Commit()
}

func (s *SQLStore) ListBonusPredictionsForUser(ctx context.Context, userID int64) ([]BonusPick, error) {
	rows, err := s.q.ListBonusPredictionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list bonus for user: %w", err)
	}
	out := make([]BonusPick, 0, len(rows))
	for _, r := range rows {
		bp := BonusPick{Category: string(r.Category), RefID: r.RefID}
		if r.Points.Valid {
			v := int64(r.Points.Int32)
			bp.Points = &v
		}
		out = append(out, bp)
	}
	return out, nil
}

func (s *SQLStore) ListBonusPredictionsWithUsers(ctx context.Context) ([]BonusUserPickRow, error) {
	rows, err := s.q.ListBonusPredictionsWithUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list bonus predictions with users: %w", err)
	}
	out := make([]BonusUserPickRow, 0, len(rows))
	for _, r := range rows {
		row := BonusUserPickRow{
			UserID:    r.UserID,
			Name:      r.Name,
			AvatarURL: r.AvatarUrl,
			Category:  string(r.Category),
			RefID:     r.RefID,
		}
		if r.Points.Valid {
			v := int64(r.Points.Int32)
			row.Points = &v
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *SQLStore) UpsertBonusResult(ctx context.Context, category string, refID int64) error {
	if err := s.q.UpsertBonusResult(ctx, sqlc.UpsertBonusResultParams{
		Category: sqlc.BonusResultsCategory(category), RefID: refID,
	}); err != nil {
		return fmt.Errorf("store: upsert bonus result: %w", err)
	}
	return nil
}

func (s *SQLStore) ListBonusResults(ctx context.Context) ([]BonusResult, error) {
	rows, err := s.q.ListBonusResults(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list bonus results: %w", err)
	}
	out := make([]BonusResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, BonusResult{Category: string(r.Category), RefID: r.RefID})
	}
	return out, nil
}

func (s *SQLStore) ListAllBonusPredictions(ctx context.Context) ([]BonusPredictionRow, error) {
	rows, err := s.q.ListAllBonusPredictions(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list all bonus predictions: %w", err)
	}
	out := make([]BonusPredictionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BonusPredictionRow{ID: r.ID, Category: string(r.Category), RefID: r.RefID})
	}
	return out, nil
}

func (s *SQLStore) SetBonusPredictionPoints(ctx context.Context, id int64, points int64) error {
	if err := s.q.SetBonusPredictionPoints(ctx, sqlc.SetBonusPredictionPointsParams{
		Points: sql.NullInt32{Int32: int32(points), Valid: true},
		ID:     id,
	}); err != nil {
		return fmt.Errorf("store: set bonus points: %w", err)
	}
	return nil
}
