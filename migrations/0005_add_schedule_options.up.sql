-- Необязательные правила, с которыми сгенерировано расписание
-- (domain.SolverPreferences). Нужны, чтобы разбивка штрафа на странице
-- расписания совпадала с сохранённым score. У старых расписаний — '{}':
-- все правила выключены, как и было при их генерации.
ALTER TABLE schedules ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '{}'::jsonb;
