-- =============================================================
-- УЧЕБНЫЕ ПЛАНЫ (subject_plans)
-- id                  — уникальный идентификатор
-- name                — название дисциплины
-- department_id       — кафедра ('itas')
-- teacher_id          — id преподавателя из таблицы teachers
-- group_ids           — группы для этого занятия, JSONB-массив
--                       лекции: можно несколько групп  '["itas-21-1","itas-21-2"]'
--                       практики/лабы: ОДНА группа     '["itas-21-1"]'
-- lecture_hours       — кол-во ПАРЫ лекций в неделю (не часов!)
-- practice_hours      — кол-во пар практик в неделю
-- lab_hours           — кол-во пар лабораторных в неделю
-- requires_room_type  — '' (обычная) или 'lab' (нужна лаборатория)
-- parity              — 'always' (каждую нед.) / 'odd' (нечётные) / 'even' (чётные)
-- semester_half       — 'full' (весь семестр) / 'first' / 'second'
-- required_building_id— '' (корпус группы) или конкретный корпус ('D')
--                       используй для математики/философии в чужом корпусе
-- =============================================================

INSERT INTO subject_plans
  (id, name, department_id, teacher_id, group_ids,
   lecture_hours, practice_hours, lab_hours,
   requires_room_type, parity, semester_half, required_building_id)
VALUES

-- Лекция для двух групп сразу (лекционный поток):
('prog_lec_21',   'Программирование', 'itas', 'ivanov_ii',
 '["itas-21-1","itas-21-2"]', 1, 0, 0, '', 'always', 'full', ''),

-- Практика отдельно для каждой группы (по одной строке на группу!):
('prog_prac_21_1','Программирование', 'itas', 'ivanov_ii',
 '["itas-21-1"]', 0, 1, 0, '', 'always', 'full', ''),
('prog_prac_21_2','Программирование', 'itas', 'ivanov_ii',
 '["itas-21-2"]', 0, 1, 0, '', 'always', 'full', ''),

-- Лабораторная (нужна лаб. аудитория):
('prog_lab_21_1', 'Программирование', 'itas', 'ivanov_ii',
 '["itas-21-1"]', 0, 0, 1, 'lab', 'odd', 'full', ''),
('prog_lab_21_2', 'Программирование', 'itas', 'ivanov_ii',
 '["itas-21-2"]', 0, 0, 1, 'lab', 'even', 'full', ''),

-- Математика в корпусе Д (required_building_id = 'D'):
('math_lec_21',   'Математика', 'itas', 'petrov_pa',
 '["itas-21-1","itas-21-2"]', 1, 0, 0, '', 'always', 'full', 'D'),
('math_prac_21_1','Математика', 'itas', 'petrov_pa',
 '["itas-21-1"]', 0, 1, 0, '', 'odd', 'full', 'D'),
('math_prac_21_2','Математика', 'itas', 'petrov_pa',
 '["itas-21-2"]', 0, 1, 0, '', 'even', 'full', 'D'),

-- Предмет только в первой половине семестра:
('intro_21',      'Введение в специальность', 'itas', 'kozlov_vv',
 '["itas-21-1","itas-21-2"]', 1, 0, 0, '', 'always', 'first', '')

-- добавь строки по образцу ↑
ON CONFLICT (id) DO NOTHING;
