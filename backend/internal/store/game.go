package store

import (
	"context"
	"fmt"

	"github.com/sayonetech/worldcup-predictor/backend/internal/store/sqlc"
)

type GameDistanceRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Email     string
	Distance  int64
}

type GameCoinRow struct {
	UserID    int64
	Name      string
	AvatarURL string
	Email     string
	Coins     int64
}

type GameMe struct {
	BestDistance int64
	CoinPool     int64
}

// GameStore backs the GOAT mini-game boards (§3.10). Append-only writes;
// reads are pure aggregates (never recompute on the prize path).
type GameStore interface {
	InsertGameRun(ctx context.Context, userID int64, distance, coins int32) error
	GameDistanceBoard(ctx context.Context) ([]GameDistanceRow, error)
	GameCoinBoard(ctx context.Context) ([]GameCoinRow, error)
	GameMeStanding(ctx context.Context, userID int64) (GameMe, error)
}

var _ GameStore = (*SQLStore)(nil)

func (s *SQLStore) InsertGameRun(ctx context.Context, userID int64, distance, coins int32) error {
	if err := s.q.InsertGameRun(ctx, sqlc.InsertGameRunParams{
		UserID:   userID,
		Distance: uint32(distance),
		Coins:    uint32(coins),
	}); err != nil {
		return fmt.Errorf("store: insert game run: %w", err)
	}
	return nil
}

func (s *SQLStore) GameDistanceBoard(ctx context.Context) ([]GameDistanceRow, error) {
	rows, err := s.q.GameDistanceBoard(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: game distance board: %w", err)
	}
	out := make([]GameDistanceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameDistanceRow{
			UserID:    r.UserID,
			Name:      r.Name,
			AvatarURL: r.AvatarUrl,
			Email:     r.Email,
			Distance:  r.BestDistance,
		})
	}
	return out, nil
}

func (s *SQLStore) GameCoinBoard(ctx context.Context) ([]GameCoinRow, error) {
	rows, err := s.q.GameCoinBoard(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: game coin board: %w", err)
	}
	out := make([]GameCoinRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameCoinRow{
			UserID:    r.UserID,
			Name:      r.Name,
			AvatarURL: r.AvatarUrl,
			Email:     r.Email,
			Coins:     r.CoinPool,
		})
	}
	return out, nil
}

func (s *SQLStore) GameMeStanding(ctx context.Context, userID int64) (GameMe, error) {
	row, err := s.q.GameMeStanding(ctx, userID)
	if err != nil {
		return GameMe{}, fmt.Errorf("store: game me standing: %w", err)
	}
	return GameMe{BestDistance: row.BestDistance, CoinPool: row.CoinPool}, nil
}
