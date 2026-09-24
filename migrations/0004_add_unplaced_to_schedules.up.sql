-- +goose Up
-- Отчёт о занятиях, которые солвер не смог разместить (внутри schedules, вместе с assignments).
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS unplaced JSONB NOT NULL DEFAULT '[]';
