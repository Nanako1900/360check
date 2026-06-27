-- =============================================================================
-- C5 — 000002_seed (DOWN)  —  reverse of 000002_seed.up.sql
-- Removes EXACTLY what the seed UP added, by known codes/keys/usernames, in
-- reverse FK order. Leaves the 000001 schema (tables/enums/indexes) intact.
--
-- Reverse FK order:
--   app_config → dict_item → dict_type → casbin_rule → user_roles
--   → users → permissions → roles
--
-- Each DELETE is keyed on the same stable natural keys the UP inserted, so this
-- is safe to run even if some rows were already removed (no-op on missing rows).
-- =============================================================================

-- 7. app_config (delete the two seeded keys at version=1)
DELETE FROM app_config
WHERE config_key IN ('export.image', 'capture.default') AND version = 1;

-- 6. dict_item then dict_type (items first; CASCADE would also drop them, but be explicit)
DELETE FROM dict_item
WHERE dict_type_id IN (
    SELECT id FROM dict_type
    WHERE code IN ('problem_status', 'problem_type', 'problem_category')
);

DELETE FROM dict_type
WHERE code IN ('problem_status', 'problem_type', 'problem_category');

-- 5. casbin_rule (remove seeded p policies + g grouping)
--    p: admin full-access + the inspector App subset; g: admin→admin.
DELETE FROM casbin_rule
WHERE (ptype = 'p' AND v0 IN ('admin', 'inspector'))
   OR (ptype = 'g' AND v0 = 'admin' AND v1 = 'admin');

-- 4. user_roles (unbind admin user ↔ admin role)
DELETE FROM user_roles
WHERE user_id IN (SELECT id FROM users WHERE username = 'admin')
  AND role_id IN (SELECT id FROM roles WHERE code = 'admin');

-- 3. users (remove bootstrap admin)
DELETE FROM users
WHERE username = 'admin';

-- 2. permissions (remove the seeded catalog by code)
DELETE FROM permissions
WHERE code IN (
    'user:read','user:create','user:update','user:delete','user:roles:read','user:roles:write',
    'role:read','role:create','role:update','role:delete','role:perms:read','role:perms:write',
    'permission:read',
    'dict:read','dict:create','dict:update','dict:delete',
    'config:read','config:update',
    'project:read','project:create','project:update','project:delete',
    'task:read','task:create','task:update','task:delete',
    'inspection:read',
    'problem:read','problem:create','problem:update','problem:delete','problem_log:write',
    'media:read',
    'stats:read',
    'export:create','export:read'
);

-- 1. roles (remove the two seeded system roles)
DELETE FROM roles
WHERE code IN ('admin', 'inspector');

-- =============================================================================
-- END 000002_seed (DOWN)
-- =============================================================================
