-- =============================================================================
-- C5 — Reverse migration. Drops everything created by 000001_init_schema.up.sql
-- in reverse dependency order. Idempotent (IF EXISTS everywhere).
-- VALIDATED: executes top-to-bottom on the migrated DB with exit 0; leaves only
-- PostGIS system objects (spatial_ref_sys + geometry/geography_columns views).
-- =============================================================================

-- 9. Deferred FKs first (so dependent tables can drop cleanly)
ALTER TABLE IF EXISTS problems DROP CONSTRAINT IF EXISTS fk_problems_cover_media;
ALTER TABLE IF EXISTS users    DROP CONSTRAINT IF EXISTS fk_users_avatar_media;

-- 8. Export jobs
DROP TABLE IF EXISTS export_jobs CASCADE;

-- 7. Media
DROP TABLE IF EXISTS media_assets CASCADE;

-- 6. Problems & processing log
DROP TABLE IF EXISTS problem_processing_log CASCADE;
DROP TABLE IF EXISTS problems CASCADE;

-- 5. Inspections & trajectory
DROP TABLE IF EXISTS trajectory_points CASCADE;
DROP TABLE IF EXISTS inspections CASCADE;

-- 4. Projects & tasks
DROP TABLE IF EXISTS inspection_tasks CASCADE;
DROP TABLE IF EXISTS projects CASCADE;

-- 3. Dictionary / config
DROP TABLE IF EXISTS app_config CASCADE;
DROP TABLE IF EXISTS dict_item CASCADE;
DROP TABLE IF EXISTS dict_type CASCADE;

-- 2. Identity & RBAC
DROP TABLE IF EXISTS casbin_rule CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- 1. Enums
DROP TYPE IF EXISTS dict_scope;
DROP TYPE IF EXISTS export_type;
DROP TYPE IF EXISTS job_status;
DROP TYPE IF EXISTS media_owner_type;
DROP TYPE IF EXISTS media_tier;
DROP TYPE IF EXISTS capture_state;
DROP TYPE IF EXISTS project_status;
DROP TYPE IF EXISTS task_status;
DROP TYPE IF EXISTS inspection_status;

-- 0. Helper function (drop last; nothing depends on it after triggers are gone)
DROP FUNCTION IF EXISTS set_updated_at();

-- Extensions are intentionally NOT dropped (may be shared by other schemas).
-- DROP EXTENSION IF EXISTS postgis;
-- DROP EXTENSION IF EXISTS pgcrypto;