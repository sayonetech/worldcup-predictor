-- name: InsertGameRun :exec
INSERT INTO game_runs (user_id, distance, coins) VALUES (?, ?, ?);

-- name: GameDistanceBoard :many
SELECT u.id AS user_id, u.name, u.avatar_url, u.email,
       CAST(MAX(r.distance) AS SIGNED) AS best_distance
FROM game_runs r JOIN users u ON u.id = r.user_id
GROUP BY u.id, u.name, u.avatar_url, u.email
ORDER BY best_distance DESC, u.id ASC
LIMIT 20;

-- name: GameCoinBoard :many
SELECT u.id AS user_id, u.name, u.avatar_url, u.email,
       CAST(SUM(r.coins) AS SIGNED) AS coin_pool
FROM game_runs r JOIN users u ON u.id = r.user_id
GROUP BY u.id, u.name, u.avatar_url, u.email
ORDER BY coin_pool DESC, u.id ASC
LIMIT 20;

-- name: GameMeStanding :one
SELECT CAST(COALESCE(MAX(distance),0) AS SIGNED) AS best_distance,
       CAST(COALESCE(SUM(coins),0)   AS SIGNED) AS coin_pool
FROM game_runs WHERE user_id = ?;
