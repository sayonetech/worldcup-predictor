CREATE TABLE game_runs (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id    BIGINT       NOT NULL,
    distance   INT UNSIGNED NOT NULL,
    coins      INT UNSIGNED NOT NULL DEFAULT 0,
    played_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_game_runs_user (user_id),
    KEY idx_game_runs_distance (distance),
    CONSTRAINT fk_game_runs_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
