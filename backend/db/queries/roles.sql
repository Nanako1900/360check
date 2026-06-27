-- Roles + permissions: scalar/relational CRUD. The roles/permissions tables are
-- human-readable UI mirrors of casbin_rule; casbin remains the enforcement source
-- of truth (see internal/rbac).

-- name: ListRoles :many
SELECT id, code, name, description, is_system, sort_order, created_at, updated_at
FROM roles
WHERE deleted_at IS NULL
ORDER BY sort_order, id;

-- name: GetRole :one
SELECT id, code, name, description, is_system, sort_order, created_at, updated_at
FROM roles WHERE id = $1 AND deleted_at IS NULL;

-- name: GetRoleByCode :one
SELECT id, code, name, description, is_system, sort_order, created_at, updated_at
FROM roles WHERE code = $1 AND deleted_at IS NULL;

-- name: RolesByIDs :many
SELECT id, code, name, description, is_system, sort_order, created_at, updated_at
FROM roles WHERE id = ANY($1::bigint[]) AND deleted_at IS NULL
ORDER BY sort_order, id;

-- name: CreateRole :one
INSERT INTO roles (code, name, description, sort_order, is_system, created_by)
VALUES ($1, $2, COALESCE($3, ''), COALESCE($4, 0), FALSE, $5)
RETURNING id, code, name, description, is_system, sort_order, created_at, updated_at;

-- name: UpdateRole :one
UPDATE roles SET
  name        = COALESCE(sqlc.narg(name), name),
  description = COALESCE(sqlc.narg(description), description),
  sort_order  = COALESCE(sqlc.narg(sort_order), sort_order),
  updated_by  = sqlc.narg(updated_by)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, code, name, description, is_system, sort_order, created_at, updated_at;

-- name: SoftDeleteRole :execrows
UPDATE roles SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL AND is_system = FALSE;

-- name: RoleCodeExists :one
SELECT EXISTS(SELECT 1 FROM roles WHERE code = $1 AND deleted_at IS NULL);
