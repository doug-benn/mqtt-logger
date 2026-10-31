-- name: CreateTemperatures :exec
INSERT INTO temperature (value, received_at)
SELECT unnest(@values::float8[]), unnest(@received_at::timestamptz[]);
