-- =============================================================================
-- C5 — 000003_seed_project_field (DOWN) — reverse of 000003_seed_project_field.up.sql
-- Removes the seeded project_field dict type and ANY items configured under it
-- (items first to respect the FK, then the type), keyed on the stable natural
-- code. Safe to run even if the rows were already removed (no-op on missing).
-- =============================================================================

DELETE FROM dict_item
WHERE dict_type_id IN (SELECT id FROM dict_type WHERE code = 'project_field');

DELETE FROM dict_type
WHERE code = 'project_field';
