-- Нежелательное время преподавателя (domain.Teacher.UndesiredSlots): мягкое пожелание,
-- в отличие от unavailable_slots — пары туда ставятся, если иначе расписание хуже.
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS undesired_slots JSONB NOT NULL DEFAULT '[]'::jsonb;
