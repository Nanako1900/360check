-- =============================================================================
-- C5 — 360相机巡查标注系统 (360-camera inspection & annotation system)
-- Shared foundation schema. PostgreSQL 16 + PostGIS 3.4.
-- Single migration, idempotent, runs top-to-bottom on a fresh DB.
-- VALIDATED against postgis/postgis:16-3.4 (PostGIS 3.4.3): up x2 (idempotent),
-- down, up-again all exit 0; SRID/mileage/idempotency/unique constraints tested.
--
-- Conventions:
--   * All geometry is WGS84 / SRID 4326. GCJ-02 is NEVER persisted.
--   * All timestamps are timestamptz (stored UTC, rendered Asia/Shanghai at edge).
--   * Audit columns: created_by / updated_by / created_at / updated_at.
--   * Soft delete via deleted_at where sensible.
--   * client_uuid UNIQUE on offline-originated entities for idempotent sync.
--   * Status modeled as native enum (capture/job/processing lifecycles) or as a
--     soft FK to dict_item (admin-editable problem types/statuses/categories).
--   * GiST index on every geometry column; btree on FKs + time/status columns.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()

-- -----------------------------------------------------------------------------
-- 0. Shared helper: updated_at auto-touch trigger function
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- -----------------------------------------------------------------------------
-- 1. Enumerated lifecycles (native enums — stable, code-driven state machines)
-- -----------------------------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE inspection_status AS ENUM ('IN_PROGRESS', 'FINISHED', 'ABANDONED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE task_status AS ENUM ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'ARCHIVED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE project_status AS ENUM ('ACTIVE', 'PAUSED', 'ARCHIVED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- media capture state machine (per confirmed constraints)
DO $$ BEGIN
    CREATE TYPE capture_state AS ENUM (
        'CAPTURED_RAW', 'STITCHED', 'QUEUED', 'UPLOADING', 'UPLOADED', 'CONFIRMED'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- COS storage tier
DO $$ BEGIN
    CREATE TYPE media_tier AS ENUM ('original', 'web', 'thumb');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- polymorphic media owner
DO $$ BEGIN
    CREATE TYPE media_owner_type AS ENUM ('problem', 'inspection', 'project', 'user');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- async export job lifecycle
DO $$ BEGIN
    CREATE TYPE job_status AS ENUM ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE export_type AS ENUM ('INSPECTION_RECORDS', 'PROBLEM_LIST', 'PROJECT_STATS');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- dictionary scope: which admin-configurable catalog a dict_type belongs to
DO $$ BEGIN
    CREATE TYPE dict_scope AS ENUM (
        'problem_type', 'problem_status', 'problem_category',
        'project_field', 'capture_preset', 'misc'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- =============================================================================
-- 2. Identity & RBAC
-- =============================================================================

-- 2.1 users
CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT        NOT NULL,
    password_hash   TEXT        NOT NULL,           -- bcrypt/argon2; never plaintext
    display_name    TEXT        NOT NULL DEFAULT '',
    phone           TEXT,
    email           TEXT,
    avatar_media_id BIGINT,                          -- soft FK -> media_assets.id (deferred)
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    created_by      BIGINT,
    updated_by      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username_active
    ON users (lower(username)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_users_is_active   ON users (is_active);
CREATE INDEX IF NOT EXISTS ix_users_deleted_at  ON users (deleted_at);
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2.2 roles catalog (UI labels for casbin role 'sub')
CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,        -- e.g. 'admin','inspector' == casbin sub
    name        TEXT        NOT NULL,               -- UI label, e.g. '系统管理员'
    description TEXT        NOT NULL DEFAULT '',
    is_system   BOOLEAN     NOT NULL DEFAULT FALSE, -- system roles cannot be deleted
    sort_order  INT         NOT NULL DEFAULT 0,
    created_by  BIGINT,
    updated_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_roles_deleted_at ON roles (deleted_at);
DROP TRIGGER IF EXISTS trg_roles_updated_at ON roles;
CREATE TRIGGER trg_roles_updated_at BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2.3 permissions catalog (UI labels for casbin object/action pairs)
CREATE TABLE IF NOT EXISTS permissions (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,        -- e.g. 'project:read' == casbin obj:act
    name        TEXT        NOT NULL,               -- UI label, e.g. '查看项目'
    object      TEXT        NOT NULL,               -- casbin obj (resource), e.g. '/api/v1/projects'
    action      TEXT        NOT NULL,               -- casbin act, e.g. 'GET'
    group_name  TEXT        NOT NULL DEFAULT '',    -- UI grouping, e.g. '项目管理'
    description TEXT        NOT NULL DEFAULT '',
    sort_order  INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_permissions_group ON permissions (group_name);
DROP TRIGGER IF EXISTS trg_permissions_updated_at ON permissions;
CREATE TRIGGER trg_permissions_updated_at BEFORE UPDATE ON permissions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2.4 user_roles (a user may hold multiple roles)
CREATE TABLE IF NOT EXISTS user_roles (
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id    BIGINT      NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);
CREATE INDEX IF NOT EXISTS ix_user_roles_role_id ON user_roles (role_id);

-- 2.5 casbin_rule (DB-backed policy store; admin-editable, redis-watcher-synced)
--     Standard go-casbin gorm/sqlx adapter layout.
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL DEFAULT '',   -- 'p' (policy) | 'g' (grouping)
    v0    VARCHAR(100) NOT NULL DEFAULT '',
    v1    VARCHAR(100) NOT NULL DEFAULT '',
    v2    VARCHAR(100) NOT NULL DEFAULT '',
    v3    VARCHAR(100) NOT NULL DEFAULT '',
    v4    VARCHAR(100) NOT NULL DEFAULT '',
    v5    VARCHAR(100) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_casbin_rule
    ON casbin_rule (ptype, v0, v1, v2, v3, v4, v5);
CREATE INDEX IF NOT EXISTS ix_casbin_rule_ptype ON casbin_rule (ptype);

-- =============================================================================
-- 3. Dictionary / config (admin-configurable + versioning + history)
-- =============================================================================

-- 3.1 dict_type — a named catalog (e.g. "问题类型", "巡查问题状态")
CREATE TABLE IF NOT EXISTS dict_type (
    id           BIGSERIAL PRIMARY KEY,
    code         TEXT        NOT NULL UNIQUE,       -- machine key, e.g. 'problem_type'
    name         TEXT        NOT NULL,              -- UI label, e.g. '问题类型'
    scope        dict_scope  NOT NULL DEFAULT 'misc',
    description  TEXT        NOT NULL DEFAULT '',
    -- versioning of the WHOLE type's item set; bumps when any item changes
    version      INT         NOT NULL DEFAULT 1,
    content_hash TEXT        NOT NULL DEFAULT '',   -- ETag = hash of item payload
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_by   BIGINT,
    updated_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_dict_type_scope     ON dict_type (scope);
CREATE INDEX IF NOT EXISTS ix_dict_type_is_active ON dict_type (is_active);
DROP TRIGGER IF EXISTS trg_dict_type_updated_at ON dict_type;
CREATE TRIGGER trg_dict_type_updated_at BEFORE UPDATE ON dict_type
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3.2 dict_item — a value within a dict_type.
--     is_active=false => RETIRED but historical references remain valid (ingest
--     MUST accept historical dictionary versions).
CREATE TABLE IF NOT EXISTS dict_item (
    id           BIGSERIAL PRIMARY KEY,
    dict_type_id BIGINT      NOT NULL REFERENCES dict_type (id) ON DELETE CASCADE,
    code         TEXT        NOT NULL,              -- machine key, unique within type
    label        TEXT        NOT NULL,              -- UI label
    color        TEXT,                              -- optional hex for status/type chips
    extra        JSONB       NOT NULL DEFAULT '{}', -- e.g. capture preset params
    sort_order   INT         NOT NULL DEFAULT 0,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE, -- false == retired (kept for history)
    created_by   BIGINT,
    updated_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_dict_item_type_code
    ON dict_item (dict_type_id, code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_dict_item_type      ON dict_item (dict_type_id);
CREATE INDEX IF NOT EXISTS ix_dict_item_is_active ON dict_item (is_active);
DROP TRIGGER IF EXISTS trg_dict_item_updated_at ON dict_item;
CREATE TRIGGER trg_dict_item_updated_at BEFORE UPDATE ON dict_item
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3.3 app_config — versioned key/value config blocks with history.
--     Each (config_key) has many rows; exactly one is_active=true row = current.
CREATE TABLE IF NOT EXISTS app_config (
    id             BIGSERIAL PRIMARY KEY,
    config_key     TEXT        NOT NULL,            -- e.g. 'export.image', 'capture.default'
    value          JSONB       NOT NULL DEFAULT '{}',
    version        INT         NOT NULL DEFAULT 1,
    content_hash   TEXT        NOT NULL DEFAULT '', -- ETag = hash(value)
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
    effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_to   TIMESTAMPTZ,                     -- NULL => still effective
    description    TEXT        NOT NULL DEFAULT '',
    created_by     BIGINT,
    updated_by     BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_app_config_active
    ON app_config (config_key) WHERE is_active;
CREATE UNIQUE INDEX IF NOT EXISTS uq_app_config_key_version
    ON app_config (config_key, version);
CREATE INDEX IF NOT EXISTS ix_app_config_key ON app_config (config_key);
DROP TRIGGER IF EXISTS trg_app_config_updated_at ON app_config;
CREATE TRIGGER trg_app_config_updated_at BEFORE UPDATE ON app_config
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 4. Projects & inspection tasks
-- =============================================================================

-- 4.1 projects
CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT        NOT NULL,               -- human project code
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    status      project_status NOT NULL DEFAULT 'ACTIVE',
    custom_fields JSONB     NOT NULL DEFAULT '{}',  -- admin-defined (dict_scope='project_field')
    area_geom   geometry(MultiPolygon, 4326),       -- optional coverage area, WGS84
    start_date  DATE,
    end_date    DATE,
    created_by  BIGINT,
    updated_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT chk_projects_area_srid
        CHECK (area_geom IS NULL OR ST_SRID(area_geom) = 4326)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_projects_code_active
    ON projects (code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_projects_status     ON projects (status);
CREATE INDEX IF NOT EXISTS ix_projects_deleted_at ON projects (deleted_at);
CREATE INDEX IF NOT EXISTS gix_projects_area_geom ON projects USING GIST (area_geom);
DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
CREATE TRIGGER trg_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 4.2 inspection_tasks (a unit of work within a project, assignable to inspectors)
CREATE TABLE IF NOT EXISTS inspection_tasks (
    id            BIGSERIAL PRIMARY KEY,
    project_id    BIGINT      NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    title         TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    status        task_status NOT NULL DEFAULT 'PENDING',
    assignee_id   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    planned_start TIMESTAMPTZ,
    planned_end   TIMESTAMPTZ,
    plan_geom     geometry(MultiLineString, 4326),  -- optional planned route
    created_by    BIGINT,
    updated_by    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    CONSTRAINT chk_tasks_plan_srid
        CHECK (plan_geom IS NULL OR ST_SRID(plan_geom) = 4326)
);
CREATE INDEX IF NOT EXISTS ix_tasks_project    ON inspection_tasks (project_id);
CREATE INDEX IF NOT EXISTS ix_tasks_assignee   ON inspection_tasks (assignee_id);
CREATE INDEX IF NOT EXISTS ix_tasks_status     ON inspection_tasks (status);
CREATE INDEX IF NOT EXISTS ix_tasks_deleted_at ON inspection_tasks (deleted_at);
CREATE INDEX IF NOT EXISTS gix_tasks_plan_geom ON inspection_tasks USING GIST (plan_geom);
DROP TRIGGER IF EXISTS trg_tasks_updated_at ON inspection_tasks;
CREATE TRIGGER trg_tasks_updated_at BEFORE UPDATE ON inspection_tasks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 5. Inspections (sessions) & trajectory
-- =============================================================================

-- 5.1 inspections — one "开始巡查 -> 结束巡查" session.
--     route_geom: finalized LineString built from trajectory_points on session end.
--     mileage_meters MUST be computed with ST_Length(route::geography) (meters).
CREATE TABLE IF NOT EXISTS inspections (
    id              BIGSERIAL PRIMARY KEY,
    client_uuid     UUID        NOT NULL DEFAULT gen_random_uuid(), -- offline idempotency
    project_id      BIGINT      NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    task_id         BIGINT      REFERENCES inspection_tasks (id) ON DELETE SET NULL,
    inspector_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status          inspection_status NOT NULL DEFAULT 'IN_PROGRESS',
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    duration_seconds BIGINT     NOT NULL DEFAULT 0,  -- derived, persisted on end
    mileage_meters  DOUBLE PRECISION NOT NULL DEFAULT 0, -- ST_Length(route::geography)
    point_count     INT         NOT NULL DEFAULT 0,  -- # trajectory points captured
    route_geom      geometry(LineString, 4326),      -- finalized route (NULL until end)
    device_info     JSONB       NOT NULL DEFAULT '{}',
    note            TEXT        NOT NULL DEFAULT '',
    created_by      BIGINT,
    updated_by      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT chk_inspections_route_srid
        CHECK (route_geom IS NULL OR ST_SRID(route_geom) = 4326),
    CONSTRAINT chk_inspections_time
        CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_inspections_client_uuid ON inspections (client_uuid);
CREATE INDEX IF NOT EXISTS ix_inspections_project     ON inspections (project_id);
CREATE INDEX IF NOT EXISTS ix_inspections_task        ON inspections (task_id);
CREATE INDEX IF NOT EXISTS ix_inspections_inspector   ON inspections (inspector_id);
CREATE INDEX IF NOT EXISTS ix_inspections_status      ON inspections (status);
CREATE INDEX IF NOT EXISTS ix_inspections_started_at  ON inspections (started_at);
CREATE INDEX IF NOT EXISTS ix_inspections_deleted_at  ON inspections (deleted_at);
CREATE INDEX IF NOT EXISTS gix_inspections_route_geom ON inspections USING GIST (route_geom);
DROP TRIGGER IF EXISTS trg_inspections_updated_at ON inspections;
CREATE TRIGGER trg_inspections_updated_at BEFORE UPDATE ON inspections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 5.2 trajectory_points — raw GPS samples for an inspection (high volume).
--     client_uuid present for idempotent batch upsert of offline samples.
CREATE TABLE IF NOT EXISTS trajectory_points (
    id            BIGSERIAL PRIMARY KEY,
    client_uuid   UUID        NOT NULL DEFAULT gen_random_uuid(),
    inspection_id BIGINT      NOT NULL REFERENCES inspections (id) ON DELETE CASCADE,
    seq           INT         NOT NULL,             -- monotonic order within inspection
    geom          geometry(Point, 4326) NOT NULL,
    recorded_at   TIMESTAMPTZ NOT NULL,             -- device GPS fix time
    speed         REAL,                             -- m/s
    bearing       REAL,                             -- degrees 0..360
    altitude      REAL,                             -- meters
    accuracy      REAL,                             -- horizontal accuracy, meters
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_traj_srid CHECK (ST_SRID(geom) = 4326)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_traj_client_uuid ON trajectory_points (client_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS uq_traj_inspection_seq
    ON trajectory_points (inspection_id, seq);
CREATE INDEX IF NOT EXISTS ix_traj_inspection   ON trajectory_points (inspection_id);
CREATE INDEX IF NOT EXISTS ix_traj_recorded_at  ON trajectory_points (recorded_at);
CREATE INDEX IF NOT EXISTS gix_traj_geom        ON trajectory_points USING GIST (geom);

-- =============================================================================
-- 6. Problems (annotations) & processing log
-- =============================================================================

-- 6.1 problems — a 360-annotated issue discovered during inspection.
--     type/status/category are SOFT FKs to dict_item (admin-editable catalogs);
--     ON DELETE RESTRICT prevents deletion of a referenced item, while retiring
--     (is_active=false) keeps it referencable for history.
--     dict_version_used pins the problem_type dict version used at capture time.
CREATE TABLE IF NOT EXISTS problems (
    id                BIGSERIAL PRIMARY KEY,
    client_uuid       UUID        NOT NULL DEFAULT gen_random_uuid(),
    project_id        BIGINT      NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    inspection_id     BIGINT      REFERENCES inspections (id) ON DELETE SET NULL,
    inspector_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    geom              geometry(Point, 4326) NOT NULL,   -- auto-located capture point
    type_item_id      BIGINT      REFERENCES dict_item (id) ON DELETE RESTRICT,
    status_item_id    BIGINT      REFERENCES dict_item (id) ON DELETE RESTRICT,
    category_item_id  BIGINT      REFERENCES dict_item (id) ON DELETE RESTRICT,
    dict_version_used INT         NOT NULL DEFAULT 1,    -- problem_type dict version pin
    title             TEXT        NOT NULL DEFAULT '',
    description       TEXT        NOT NULL DEFAULT '',
    note              TEXT        NOT NULL DEFAULT '',
    captured_at       TIMESTAMPTZ NOT NULL,             -- when the photo was taken
    cover_media_id    BIGINT,                            -- denormalized cover (deferred FK)
    created_by        BIGINT,
    updated_by        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    CONSTRAINT chk_problems_srid CHECK (ST_SRID(geom) = 4326)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_problems_client_uuid ON problems (client_uuid);
CREATE INDEX IF NOT EXISTS ix_problems_project     ON problems (project_id);
CREATE INDEX IF NOT EXISTS ix_problems_inspection  ON problems (inspection_id);
CREATE INDEX IF NOT EXISTS ix_problems_inspector   ON problems (inspector_id);
CREATE INDEX IF NOT EXISTS ix_problems_type        ON problems (type_item_id);
CREATE INDEX IF NOT EXISTS ix_problems_status      ON problems (status_item_id);
CREATE INDEX IF NOT EXISTS ix_problems_category    ON problems (category_item_id);
CREATE INDEX IF NOT EXISTS ix_problems_captured_at ON problems (captured_at);
CREATE INDEX IF NOT EXISTS ix_problems_deleted_at  ON problems (deleted_at);
CREATE INDEX IF NOT EXISTS gix_problems_geom       ON problems USING GIST (geom);
DROP TRIGGER IF EXISTS trg_problems_updated_at ON problems;
CREATE TRIGGER trg_problems_updated_at BEFORE UPDATE ON problems
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 6.2 problem_processing_log — append-only audit/event trail of status changes.
CREATE TABLE IF NOT EXISTS problem_processing_log (
    id          BIGSERIAL PRIMARY KEY,
    problem_id  BIGINT      NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    action      TEXT        NOT NULL,               -- 'STATUS_CHANGE','COMMENT','REASSIGN',...
    from_status_item_id BIGINT REFERENCES dict_item (id) ON DELETE SET NULL,
    to_status_item_id   BIGINT REFERENCES dict_item (id) ON DELETE SET NULL,
    note        TEXT        NOT NULL DEFAULT '',
    operator_id BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_pplog_problem    ON problem_processing_log (problem_id);
CREATE INDEX IF NOT EXISTS ix_pplog_operator   ON problem_processing_log (operator_id);
CREATE INDEX IF NOT EXISTS ix_pplog_created_at ON problem_processing_log (created_at);

-- =============================================================================
-- 7. Media assets (COS-backed; 3 tiers + capture state machine)
-- =============================================================================

-- media_assets — polymorphic owner; multipart-uploaded to COS via STS, then
-- HeadObject-verified before capture_state advances to CONFIRMED (verified_at set).
CREATE TABLE IF NOT EXISTS media_assets (
    id            BIGSERIAL PRIMARY KEY,
    client_uuid   UUID        NOT NULL DEFAULT gen_random_uuid(),
    owner_type    media_owner_type NOT NULL,
    owner_id      BIGINT      NOT NULL,             -- soft FK by (owner_type, owner_id)
    tier          media_tier  NOT NULL,            -- original | web | thumb
    cos_bucket    TEXT        NOT NULL,
    cos_key       TEXT        NOT NULL,             -- per-upload prefix scoped key
    cos_region    TEXT        NOT NULL DEFAULT 'ap-guangzhou',
    content_type  TEXT        NOT NULL DEFAULT 'image/jpeg',
    byte_size     BIGINT,
    width         INT,                              -- equirectangular e.g. 4096
    height        INT,                              -- equirectangular e.g. 2048
    etag          TEXT,                             -- COS object ETag (verify match)
    capture_state capture_state NOT NULL DEFAULT 'CAPTURED_RAW',
    verified_at   TIMESTAMPTZ,                      -- set when HeadObject verified
    media_group   UUID,                             -- groups original/web/thumb siblings
    meta          JSONB       NOT NULL DEFAULT '{}',
    created_by    BIGINT,
    updated_by    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_media_client_uuid ON media_assets (client_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS uq_media_bucket_key  ON media_assets (cos_bucket, cos_key);
CREATE INDEX IF NOT EXISTS ix_media_owner       ON media_assets (owner_type, owner_id);
CREATE INDEX IF NOT EXISTS ix_media_state       ON media_assets (capture_state);
CREATE INDEX IF NOT EXISTS ix_media_tier        ON media_assets (tier);
CREATE INDEX IF NOT EXISTS ix_media_group       ON media_assets (media_group);
CREATE INDEX IF NOT EXISTS ix_media_verified_at ON media_assets (verified_at);
CREATE INDEX IF NOT EXISTS ix_media_deleted_at  ON media_assets (deleted_at);
DROP TRIGGER IF EXISTS trg_media_updated_at ON media_assets;
CREATE TRIGGER trg_media_updated_at BEFORE UPDATE ON media_assets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 8. Export jobs (async Excel exports via asynq)
-- =============================================================================

CREATE TABLE IF NOT EXISTS export_jobs (
    id             BIGSERIAL PRIMARY KEY,
    job_uuid       UUID        NOT NULL DEFAULT gen_random_uuid(), -- public job handle
    type           export_type NOT NULL,
    params         JSONB       NOT NULL DEFAULT '{}', -- filters: project/time/inspector
    status         job_status  NOT NULL DEFAULT 'PENDING',
    progress       INT         NOT NULL DEFAULT 0,    -- 0..100
    total_rows     INT,
    processed_rows INT         NOT NULL DEFAULT 0,
    result_cos_key TEXT,                              -- COS key of produced .xlsx
    result_bucket  TEXT,
    error          TEXT,
    requested_by   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_export_progress CHECK (progress BETWEEN 0 AND 100)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_export_job_uuid ON export_jobs (job_uuid);
CREATE INDEX IF NOT EXISTS ix_export_status       ON export_jobs (status);
CREATE INDEX IF NOT EXISTS ix_export_requested_by ON export_jobs (requested_by);
CREATE INDEX IF NOT EXISTS ix_export_created_at   ON export_jobs (created_at);
DROP TRIGGER IF EXISTS trg_export_updated_at ON export_jobs;
CREATE TRIGGER trg_export_updated_at BEFORE UPDATE ON export_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 9. Deferred FKs that reference media_assets (added after media_assets exists)
-- =============================================================================
DO $$ BEGIN
    ALTER TABLE users
        ADD CONSTRAINT fk_users_avatar_media
        FOREIGN KEY (avatar_media_id) REFERENCES media_assets (id) ON DELETE SET NULL;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE problems
        ADD CONSTRAINT fk_problems_cover_media
        FOREIGN KEY (cover_media_id) REFERENCES media_assets (id) ON DELETE SET NULL;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;