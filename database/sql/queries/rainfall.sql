-- name: CreateRainfall :one
INSERT INTO rainfall (value)
VALUES ($1)
RETURNING id, value, received_at;

-- name: ListRainfall :many
SELECT id, value, received_at
FROM rainfall
ORDER BY id DESC;
