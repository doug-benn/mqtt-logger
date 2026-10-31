-- name: CreateHumidities :exec
INSERT INTO humidity (value, received_at)
SELECT unnest(@values::float8[]), unnest(@received_at::timestamptz[]);
