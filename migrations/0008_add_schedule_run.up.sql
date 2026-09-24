-- +goose Up
-- Как получено расписание (domain.RunInfo): алгоритмы, сиды и число раундов улучшения.
-- По ним запуск можно повторить и получить то же расписание (ADR-0023). У старых
-- расписаний — '{}': повторить их нельзя.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS run JSONB NOT NULL DEFAULT '{}'::jsonb;
