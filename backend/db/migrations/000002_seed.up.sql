-- =============================================================================
-- C5 — 000002_seed (UP)  —  Day-1 BLOCKING seed (data only; schema lives in 000001)
-- 360相机巡查标注系统 baseline seed: RBAC + casbin + dictionaries + app_config.
--
-- SINGLE AUTHORITATIVE SOURCE for: roles, permissions, the bootstrap admin user,
-- user_roles, casbin_rule policies, the three problem dict_types
-- (problem_status / problem_type / problem_category) and their baseline items,
-- and app_config (export.image, capture.default).
--
-- CANONICAL enum / dict literals are FROZEN and must match 00-数据模型与API契约.md
-- and the on-disk 000001_init.up.sql verbatim.
--
-- IDEMPOTENCY: every INSERT uses ON CONFLICT DO NOTHING against the REAL unique
-- indexes declared in 000001. Safe to re-run top-to-bottom on a fresh or seeded DB.
--
-- FK ORDERING (top→bottom, FK-safe):
--   roles → permissions → users → user_roles → casbin_rule
--   → dict_type → dict_item → app_config
-- All cross-references use subselects by stable natural keys (code/username/key)
-- so it is FK-correct regardless of BIGSERIAL id assignment.
--
-- pgcrypto (gen_random_uuid, crypt, gen_salt) is enabled by 000001.
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. roles  (UI catalog for casbin role subjects; code == casbin sub)
--    Unique key: roles.code (UNIQUE).  Columns: code,name,description,is_system,sort_order
-- -----------------------------------------------------------------------------
INSERT INTO roles (code, name, description, is_system, sort_order) VALUES
    ('admin',     '系统管理员', '系统内置超级管理员，拥有全部权限，不可删除', TRUE, 10),
    ('inspector', '巡查员',     'APP 端巡查员，可执行巡查任务、采集轨迹/问题/媒体并同步', TRUE, 20)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 2. permissions  (UI catalog mirroring casbin object/action pairs)
--    Unique key: permissions.code (UNIQUE).
--    Columns: code,name,object,action,group_name(,description,sort_order)
--    object = /api/v1/<resource>; action = HTTP method. Mirrors web 权限树.
-- -----------------------------------------------------------------------------
INSERT INTO permissions (code, name, object, action, group_name, sort_order) VALUES
    -- 用户管理
    ('user:read',        '查看用户',   '/api/v1/users',                'GET',    '用户管理', 100),
    ('user:create',      '创建用户',   '/api/v1/users',                'POST',   '用户管理', 101),
    ('user:update',      '编辑用户',   '/api/v1/users/*',              'PUT',    '用户管理', 102),
    ('user:delete',      '删除用户',   '/api/v1/users/*',              'DELETE', '用户管理', 103),
    ('user:roles:read',  '查看用户角色', '/api/v1/users/*/roles',       'GET',    '用户管理', 104),
    ('user:roles:write', '分配用户角色', '/api/v1/users/*/roles',       'PUT',    '用户管理', 105),
    -- 角色管理
    ('role:read',        '查看角色',   '/api/v1/roles',                'GET',    '角色管理', 110),
    ('role:create',      '创建角色',   '/api/v1/roles',                'POST',   '角色管理', 111),
    ('role:update',      '编辑角色',   '/api/v1/roles/*',              'PUT',    '角色管理', 112),
    ('role:delete',      '删除角色',   '/api/v1/roles/*',              'DELETE', '角色管理', 113),
    ('role:perms:read',  '查看角色权限', '/api/v1/roles/*/permissions', 'GET',    '角色管理', 114),
    ('role:perms:write', '分配角色权限', '/api/v1/roles/*/permissions', 'PUT',    '角色管理', 115),
    -- 权限目录（只读 UI 树）
    ('permission:read',  '查看权限目录', '/api/v1/permissions',         'GET',    '权限管理', 120),
    -- 字典管理
    ('dict:read',        '查看字典',   '/api/v1/dict/types',           'GET',    '字典管理', 130),
    ('dict:create',      '创建字典',   '/api/v1/dict/types',           'POST',   '字典管理', 131),
    ('dict:update',      '编辑字典',   '/api/v1/dict/types/*',         'PUT',    '字典管理', 132),
    ('dict:delete',      '删除字典',   '/api/v1/dict/types/*',         'DELETE', '字典管理', 133),
    -- 配置管理
    ('config:read',      '查看配置',   '/api/v1/config/*',             'GET',    '配置管理', 140),
    ('config:update',    '编辑配置',   '/api/v1/config/*',             'PUT',    '配置管理', 141),
    -- 项目管理
    ('project:read',     '查看项目',   '/api/v1/projects',             'GET',    '项目管理', 150),
    ('project:create',   '创建项目',   '/api/v1/projects',             'POST',   '项目管理', 151),
    ('project:update',   '编辑项目',   '/api/v1/projects/*',           'PUT',    '项目管理', 152),
    ('project:delete',   '删除项目',   '/api/v1/projects/*',           'DELETE', '项目管理', 153),
    -- 任务管理
    ('task:read',        '查看任务',   '/api/v1/tasks',                'GET',    '任务管理', 160),
    ('task:create',      '创建任务',   '/api/v1/tasks',                'POST',   '任务管理', 161),
    ('task:update',      '编辑任务',   '/api/v1/tasks/*',              'PUT',    '任务管理', 162),
    ('task:delete',      '删除任务',   '/api/v1/tasks/*',              'DELETE', '任务管理', 163),
    -- 巡查记录（只读管理）
    ('inspection:read',  '查看巡查记录', '/api/v1/inspections',         'GET',    '巡查管理', 170),
    -- 问题管理
    ('problem:read',     '查看问题',   '/api/v1/problems',             'GET',    '问题管理', 180),
    ('problem:create',   '创建问题',   '/api/v1/problems',             'POST',   '问题管理', 181),
    ('problem:update',   '编辑问题',   '/api/v1/problems/*',           'PUT',    '问题管理', 182),
    ('problem:delete',   '删除问题',   '/api/v1/problems/*',           'DELETE', '问题管理', 183),
    ('problem_log:write','追加问题处理记录', '/api/v1/problems/*/logs', 'POST',   '问题管理', 184),
    -- 媒体
    ('media:read',       '查看媒体',   '/api/v1/media/*',              'GET',    '媒体管理', 190),
    -- 统计
    ('stats:read',       '查看统计',   '/api/v1/stats/overview',       'GET',    '统计分析', 200),
    -- 导出
    ('export:create',    '创建导出任务', '/api/v1/exports',            'POST',   '数据导出', 210),
    ('export:read',      '查看导出结果', '/api/v1/exports/*',          'GET',    '数据导出', 211)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 3. users — bootstrap admin (password LOCKED, no default credential)
--    Unique key: uq_users_username_active = UNIQUE(lower(username)) WHERE deleted_at IS NULL.
--
--    SECURITY: the admin row ships with a LOCKED password_hash sentinel ('!'),
--    which is not a valid bcrypt hash — golang.org/x/crypto/bcrypt
--    CompareHashAndPassword always errors against it, so Login() returns
--    ErrInvalidCredentials and NO password can authenticate. There is no live
--    default credential. Ops sets the real admin password out-of-band, once,
--    from a deploy secret:  `c5-api create-admin`  reads C5_BOOTSTRAP_ADMIN_PASSWORD
--    (and optional C5_BOOTSTRAP_ADMIN_USERNAME, default 'admin') and bcrypt-hashes
--    it at cost 12. Run it as a one-shot Job after the migrate Job. Until then the
--    admin account exists (with its admin role binding) but is un-loginable —
--    fail-safe, not fail-open.
--    NOTE: ON CONFLICT targets the partial unique index (lower(username)) WHERE
--    deleted_at IS NULL, so re-runs never overwrite an existing admin row.
-- -----------------------------------------------------------------------------
INSERT INTO users (username, password_hash, display_name, is_active)
VALUES (
    'admin',
    '!', -- locked sentinel: invalid bcrypt hash → no password authenticates (see above)
    '系统管理员',
    TRUE
)
ON CONFLICT (lower(username)) WHERE deleted_at IS NULL DO NOTHING;

-- -----------------------------------------------------------------------------
-- 4. user_roles — bind admin user → admin role (PK (user_id, role_id))
--    Subselects keep it FK-correct regardless of serial ids.
-- -----------------------------------------------------------------------------
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
WHERE u.username = 'admin' AND u.deleted_at IS NULL
  AND r.code = 'admin'
ON CONFLICT (user_id, role_id) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 5. casbin_rule — enforced policy source of truth (go-casbin sqlx/gorm adapter).
--    Unique key: uq_casbin_rule(ptype, v0..v5).
--
--    MODEL ASSUMPTION (must match backend casbin model.conf):
--      [request_definition]  r = sub, obj, act
--      [policy_definition]   p = sub, obj, act
--      [role_definition]     g = _, _
--      [matchers] m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) \
--                     && (r.act == p.act || p.act == "*")
--    keyMatch2 lets '/api/v1/*' and '/api/v1/users/:id' style patterns match.
--    p rows:  p, <role>, <obj pattern>, <act>
--    g rows:  g, <user-subject>, <role>     (user→role grouping)
--
--    'admin' role = full access. 'inspector' role = App-needed subset.
-- -----------------------------------------------------------------------------

-- 5.1 p policies
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
    -- admin: full access to everything under /api/v1
    ('p', 'admin', '/api/v1/*', '*'),

    -- inspector: App-needed subset only (read identity, dicts/config, projects/
    -- tasks/inspections, problems CRUD + logs, media read, offline sync).
    ('p', 'inspector', '/api/v1/auth/me',          'GET'),
    ('p', 'inspector', '/api/v1/auth/password',    'PUT'),
    ('p', 'inspector', '/api/v1/auth/logout',      'POST'),
    ('p', 'inspector', '/api/v1/dict/types',       'GET'),
    ('p', 'inspector', '/api/v1/dict/types/*',     'GET'),
    ('p', 'inspector', '/api/v1/config/*',         'GET'),
    ('p', 'inspector', '/api/v1/projects',         'GET'),
    ('p', 'inspector', '/api/v1/projects/*',       'GET'),
    ('p', 'inspector', '/api/v1/tasks',            'GET'),
    ('p', 'inspector', '/api/v1/tasks/*',          'GET'),
    ('p', 'inspector', '/api/v1/tasks/*',          'PUT'),    -- update task status from field
    ('p', 'inspector', '/api/v1/inspections',      'GET'),
    ('p', 'inspector', '/api/v1/inspections/*',    'GET'),
    ('p', 'inspector', '/api/v1/inspections/start','POST'),
    ('p', 'inspector', '/api/v1/inspections/*/finish', 'POST'),
    ('p', 'inspector', '/api/v1/problems',         'GET'),
    ('p', 'inspector', '/api/v1/problems',         'POST'),
    ('p', 'inspector', '/api/v1/problems/*',       'GET'),
    ('p', 'inspector', '/api/v1/problems/*',       'PUT'),
    ('p', 'inspector', '/api/v1/problems/*/logs',  'GET'),
    ('p', 'inspector', '/api/v1/problems/*/logs',  'POST'),   -- COMMENT/REASSIGN only (D3)
    ('p', 'inspector', '/api/v1/media/*',          'GET'),
    ('p', 'inspector', '/api/v1/media/upload-credentials', 'POST'),
    ('p', 'inspector', '/api/v1/media/confirm',    'POST'),
    ('p', 'inspector', '/api/v1/sync/batch',       'POST')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- 5.2 g grouping — map the bootstrap admin subject to the admin role.
--     The service uses the user's username as the casbin subject key for the
--     bootstrap admin (sub == 'admin'); g binds that subject to the 'admin' role.
INSERT INTO casbin_rule (ptype, v0, v1) VALUES
    ('g', 'admin', 'admin')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- =============================================================================
-- 6. dict_type + dict_item — admin-configurable catalogs.
--    Unique keys: dict_type.code (UNIQUE);
--                 uq_dict_item_type_code = UNIQUE(dict_type_id, code) WHERE deleted_at IS NULL.
--    version=1 baseline; content_hash = md5(...) of the item-set payload string
--    (deterministic, recomputed identically on re-run → stable ETag).
-- =============================================================================

-- 6.1 problem_status  (scope=problem_status) — baseline OPEN/PROCESSING/RESOLVED/CLOSED
INSERT INTO dict_type (code, name, scope, description, version, content_hash, is_active) VALUES
    ('problem_status', '问题状态', 'problem_status', '巡查问题处理状态（基线，可配置；退役用 is_active=false 永不硬删）',
     1,
     md5('problem_status|v1|'
         || 'OPEN:待处理:#9CA3AF:10;'
         || 'PROCESSING:处理中:#F59E0B:20;'
         || 'RESOLVED:已解决:#10B981:30;'
         || 'CLOSED:已关闭:#6B7280:40'),
     TRUE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO dict_item (dict_type_id, code, label, color, sort_order, is_active)
SELECT dt.id, x.code, x.label, x.color, x.sort_order, TRUE
FROM dict_type dt
CROSS JOIN (VALUES
    ('OPEN',       '待处理', '#9CA3AF', 10),
    ('PROCESSING', '处理中', '#F59E0B', 20),
    ('RESOLVED',   '已解决', '#10B981', 30),
    ('CLOSED',     '已关闭', '#6B7280', 40)
) AS x(code, label, color, sort_order)
WHERE dt.code = 'problem_status'
ON CONFLICT (dict_type_id, code) WHERE deleted_at IS NULL DO NOTHING;

-- 6.2 problem_type  (scope=problem_type) — baseline 路面/设施/卫生/安全/其他
INSERT INTO dict_type (code, name, scope, description, version, content_hash, is_active) VALUES
    ('problem_type', '问题类型', 'problem_type', '巡查问题类型（基线，可配置）',
     1,
     md5('problem_type|v1|'
         || 'ROAD:路面:#EF4444:10;'
         || 'FACILITY:设施:#3B82F6:20;'
         || 'SANITATION:卫生:#22C55E:30;'
         || 'SAFETY:安全:#F97316:40;'
         || 'OTHER:其他:#6B7280:50'),
     TRUE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO dict_item (dict_type_id, code, label, color, sort_order, is_active)
SELECT dt.id, x.code, x.label, x.color, x.sort_order, TRUE
FROM dict_type dt
CROSS JOIN (VALUES
    ('ROAD',       '路面', '#EF4444', 10),
    ('FACILITY',   '设施', '#3B82F6', 20),
    ('SANITATION', '卫生', '#22C55E', 30),
    ('SAFETY',     '安全', '#F97316', 40),
    ('OTHER',      '其他', '#6B7280', 50)
) AS x(code, label, color, sort_order)
WHERE dt.code = 'problem_type'
ON CONFLICT (dict_type_id, code) WHERE deleted_at IS NULL DO NOTHING;

-- 6.3 problem_category  (scope=problem_category) — small baseline
INSERT INTO dict_type (code, name, scope, description, version, content_hash, is_active) VALUES
    ('problem_category', '问题分类', 'problem_category', '巡查问题分类（基线，可配置）',
     1,
     md5('problem_category|v1|'
         || 'URGENT:紧急:#DC2626:10;'
         || 'ROUTINE:常规:#2563EB:20;'
         || 'MINOR:轻微:#64748B:30'),
     TRUE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO dict_item (dict_type_id, code, label, color, sort_order, is_active)
SELECT dt.id, x.code, x.label, x.color, x.sort_order, TRUE
FROM dict_type dt
CROSS JOIN (VALUES
    ('URGENT',  '紧急', '#DC2626', 10),
    ('ROUTINE', '常规', '#2563EB', 20),
    ('MINOR',   '轻微', '#64748B', 30)
) AS x(code, label, color, sort_order)
WHERE dt.code = 'problem_category'
ON CONFLICT (dict_type_id, code) WHERE deleted_at IS NULL DO NOTHING;

-- =============================================================================
-- 7. app_config — versioned key/value config (one is_active=true row per key).
--    Unique keys: uq_app_config_active = UNIQUE(config_key) WHERE is_active;
--                 uq_app_config_key_version = UNIQUE(config_key, version).
--    version=1, is_active=true; content_hash = md5(value::text).
-- =============================================================================

-- 7.1 export.image — equirectangular export size/quality (drives APP ExportUtils + worker)
INSERT INTO app_config (config_key, value, version, content_hash, is_active, description)
VALUES (
    'export.image',
    '{"width":4096,"height":2048,"quality":85}'::jsonb,
    1,
    md5('{"width":4096,"height":2048,"quality":85}'::jsonb::text),
    TRUE,
    '全景导出图像尺寸与质量（2:1 equirectangular）'
)
ON CONFLICT (config_key) WHERE is_active DO NOTHING;

-- 7.2 capture.default — default capture mode (APP 拍照默认参数)
INSERT INTO app_config (config_key, value, version, content_hash, is_active, description)
VALUES (
    'capture.default',
    '{"mode":"NORMAL","hdr":false}'::jsonb,
    1,
    md5('{"mode":"NORMAL","hdr":false}'::jsonb::text),
    TRUE,
    'APP 默认拍照参数（NORMAL 普通拍照，关闭 HDR）'
)
ON CONFLICT (config_key) WHERE is_active DO NOTHING;

-- =============================================================================
-- END 000002_seed (UP)
-- =============================================================================
