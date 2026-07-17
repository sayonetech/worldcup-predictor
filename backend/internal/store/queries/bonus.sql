-- name: UpsertBonusPrediction :exec
INSERT INTO bonus_predictions (user_id, category, ref_id)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE ref_id = VALUES(ref_id);

-- name: ListBonusPredictionsForUser :many
SELECT category, ref_id, points FROM bonus_predictions WHERE user_id = ?;

-- name: UpsertBonusResult :exec
INSERT INTO bonus_results (category, ref_id)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE ref_id = VALUES(ref_id);

-- name: ListBonusResults :many
SELECT category, ref_id FROM bonus_results;

-- name: ListAllBonusPredictions :many
SELECT id, category, ref_id FROM bonus_predictions;

-- name: SetBonusPredictionPoints :exec
UPDATE bonus_predictions SET points = ? WHERE id = ?;

-- name: ListBonusPredictionsWithUsers :many
-- Every user's bonus picks with the player's name/avatar. Used to reveal others'
-- picks once bonus locks at BONUS_LOCK_AT (privacy, spec §4). The INNER JOIN means
-- only users who actually set picks appear. Alphabetical by player, then category.
SELECT
    u.id AS user_id, u.name, u.avatar_url,
    bp.category, bp.ref_id, bp.points
FROM bonus_predictions bp
JOIN users u ON u.id = bp.user_id
ORDER BY u.name, bp.category;
