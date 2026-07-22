-- =============================================================
-- ПРЕПОДАВАТЕЛИ (teachers)
-- id                  — уникальный идентификатор (латиница, без пробелов)
-- name                — ФИО как в расписании
-- department_id       — id кафедры из таблицы departments
-- max_weekly_hours    — макс. нагрузка в академических часах в неделю
--                       (обычно 6–10 пар = 12–20 часов)
-- unavailable_slots   — слоты когда препод недоступен, JSONB-массив
--                       пример: '[{"day":"monday","pair_num":1}]'
--                       пустой: '[]'
-- preferred_buildings — список корпусов где работает, JSONB-массив
--                       пример: '["A","B"]'
--                       пустой (любой корпус): '[]'
-- =============================================================

INSERT INTO teachers (id, name, department_id, max_weekly_hours, unavailable_slots, preferred_buildings) VALUES
  ('ivanov_ii',   'Иванов И.И.',   'itas', 16, '[]', '["A"]'),
  ('petrov_pa',   'Петров П.А.',   'itas', 14, '[]', '["A","B"]'),
  ('sidorova_nn', 'Сидорова Н.Н.', 'itas', 18, '[{"day":"friday","pair_num":5},{"day":"friday","pair_num":6}]', '["A"]'),
  ('kozlov_vv',   'Козлов В.В.',   'itas', 12, '[]', '[]')
-- добавь строки по образцу ↑
ON CONFLICT (id) DO NOTHING;
