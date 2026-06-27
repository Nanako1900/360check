-- name: ListPermissions :many
SELECT id, code, name, object, action, group_name, description, sort_order
FROM permissions
ORDER BY sort_order, id;

-- name: PermissionsByIDs :many
SELECT id, code, name, object, action, group_name, description, sort_order
FROM permissions WHERE id = ANY($1::bigint[])
ORDER BY sort_order, id;

-- name: PermissionsByObjectAction :many
SELECT id, code, name, object, action, group_name, description, sort_order
FROM permissions
ORDER BY sort_order, id;
