-- =============================================================================
-- C5 — 000003_seed_project_field (UP)
-- Seeds an EMPTY 'project_field' dict type so an admin can configure project
-- custom fields from the Dict Config UI without first hand-creating the type.
-- Unlike the problem_* baselines (000002), project_field ships with NO items:
-- a project has no custom fields until an admin adds dict_item rows under it.
--
-- Why a separate migration (not edited into 000002): 000002 is already applied
-- on deployed databases, and golang-migrate never re-runs an applied version —
-- so the new seed must land as its own forward migration. Idempotent via
-- ON CONFLICT (code) DO NOTHING (safe to re-run / on a fresh DB).
--
-- content_hash mirrors the 000002 convention — md5('<code>|v<version>|' || items)
-- — here with an empty item-set: md5('project_field|v1|').
-- =============================================================================

INSERT INTO dict_type (code, name, scope, description, version, content_hash, is_active) VALUES
    ('project_field', '项目字段', 'project_field',
     '项目自定义字段（管理员配置；dict_item.code=字段键，label=字段标签，extra.type=number 时为数值字段）',
     1,
     md5('project_field|v1|'),
     TRUE)
ON CONFLICT (code) DO NOTHING;
