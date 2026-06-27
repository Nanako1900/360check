-- Scalar/relational CRUD via sqlc. Geometry I/O is handled by the hand-written
-- pgx layer (internal/platform/geo + ST_AsEWKB/ST_GeomFromEWKB casts), never by
-- sqlc, because sqlc's analyzer cannot resolve PostGIS functions. sqlc queries
-- therefore never select a raw geometry column or call a PostGIS function.

-- name: GetUserByUsername :one
SELECT id, username, password_hash, display_name, phone, email,
       is_active, last_login_at, created_at, updated_at
FROM users
WHERE lower(username) = lower(sqlc.arg(username)) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, username, display_name, phone, email, avatar_media_id, is_active,
       last_login_at, created_at, updated_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, display_name, phone, email, is_active, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, username, display_name, is_active, created_at;

-- name: TouchUserLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: SoftDeleteUser :execrows
UPDATE users SET deleted_at = now(), updated_by = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT id, username, display_name, phone, email, avatar_media_id,
       is_active, last_login_at, created_at, updated_at
FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR username ILIKE '%' || sqlc.narg(q) || '%'
       OR display_name ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active))
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: CountUsers :one
SELECT count(*) FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR username ILIKE '%' || sqlc.narg(q) || '%'
       OR display_name ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active));

-- name: UpdateUser :one
UPDATE users SET
  display_name    = COALESCE(sqlc.narg(display_name), display_name),
  phone           = COALESCE(sqlc.narg(phone), phone),
  email           = COALESCE(sqlc.narg(email), email),
  avatar_media_id = COALESCE(sqlc.narg(avatar_media_id), avatar_media_id),
  is_active       = COALESCE(sqlc.narg(is_active), is_active),
  updated_by      = sqlc.narg(updated_by)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, username, display_name, phone, email, avatar_media_id,
          is_active, last_login_at, created_at, updated_at;

-- name: UpdateUserPassword :execrows
UPDATE users SET password_hash = $2, updated_by = $3 WHERE id = $1 AND deleted_at IS NULL;

-- name: UsernameExists :one
SELECT EXISTS(SELECT 1 FROM users WHERE lower(username) = lower($1) AND deleted_at IS NULL);

-- name: GetUserPasswordHash :one
SELECT password_hash FROM users WHERE id = $1 AND deleted_at IS NULL;
