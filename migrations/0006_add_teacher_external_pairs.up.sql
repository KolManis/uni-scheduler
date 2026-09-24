-- +goose Up
-- Пары преподавателя на других факультетах (domain.ExternalPair): время задано
-- извне и не двигается, алгоритм строит расписание кафедры вокруг них.
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS external_pairs JSONB NOT NULL DEFAULT '[]'::jsonb;
