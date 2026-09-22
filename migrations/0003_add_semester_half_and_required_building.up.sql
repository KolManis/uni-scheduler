-- Код (internal/repository/postgres/input_repo.go, ref_write_repo.go) уже читает и пишет
-- subject_plans.required_building_id и subject_plans.semester_half, но эти колонки
-- никогда не добавлялись в схему — миграция была пропущена при разработке ветки semester-half.
ALTER TABLE subject_plans ADD COLUMN IF NOT EXISTS required_building_id TEXT REFERENCES buildings(id);
ALTER TABLE subject_plans ADD COLUMN IF NOT EXISTS semester_half TEXT NOT NULL DEFAULT 'full';
