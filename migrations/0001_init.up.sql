-- ============================================================
-- УНИВЕРСИТЕТСКИЙ ПЛАНИРОВЩИК РАСПИСАНИЯ
-- Реалистичные тестовые данные
-- ============================================================

CREATE TABLE IF NOT EXISTS buildings (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS departments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    student_count INTEGER NOT NULL DEFAULT 0,
    building_ids JSONB DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS teachers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    department_id TEXT REFERENCES departments(id),
    max_weekly_hours INTEGER NOT NULL DEFAULT 36,
    unavailable_slots JSONB DEFAULT '[]',
    preferred_buildings JSONB DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS rooms (
    id TEXT PRIMARY KEY,
    number TEXT NOT NULL,
    building_id TEXT REFERENCES buildings(id),
    capacity INTEGER NOT NULL,
    type TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subject_plans (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    department_id TEXT REFERENCES departments(id),
    lecture_hours INTEGER NOT NULL DEFAULT 0,
    practice_hours INTEGER NOT NULL DEFAULT 0,
    lab_hours INTEGER NOT NULL DEFAULT 0,
    requires_room_type TEXT NOT NULL,
    teacher_id TEXT NOT NULL REFERENCES teachers(id),
    group_ids JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS schedules (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    assignments JSONB NOT NULL DEFAULT '[]',
    score INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ КОРПУСА ============
INSERT INTO buildings (id, name, address) VALUES
    ('B1', 'Главный корпус', 'ул. Ленина, 1'),
    ('B2', 'Лабораторный корпус', 'ул. Пушкина, 10');

-- ============ КАФЕДРЫ ============
INSERT INTO departments (id, name) VALUES
    ('DEP-ETF', 'Электротехнический факультет'),
    ('DEP-IT', 'Информационных технологий'),
    ('DEP-MATH', 'Высшей математики'),
    ('DEP-HUM', 'Гуманитарных дисциплин');

-- ============ ГРУППЫ (18 групп) ============
INSERT INTO groups (id, name, student_count, building_ids) VALUES
    ('ATP-101', 'АТП-101', 25, '["B1", "B2"]'),
    ('ATP-102', 'АТП-102', 22, '["B1", "B2"]'),
    ('IKT-101', 'ИКТ-101', 28, '["B1"]'),
    ('IKT-102', 'ИКТ-102', 26, '["B1"]'),
    ('IVT-101', 'ИВТ-101', 30, '["B1"]'),
    ('IVT-102', 'ИВТ-102', 24, '["B1"]'),
    ('IB-101', 'ИБ-101', 27, '["B1"]'),
    ('IB-102', 'ИБ-102', 23, '["B1"]'),
    ('IBA-101', 'ИБА-101', 20, '["B1"]'),
    ('IBA-102', 'ИБА-102', 22, '["B1"]'),
    ('MR-101', 'МР-101', 18, '["B1", "B2"]'),
    ('MR-102', 'МР-102', 20, '["B1", "B2"]'),
    ('PI-101', 'ПИ-101', 28, '["B1"]'),
    ('PI-102', 'ПИ-102', 26, '["B1"]'),
    ('UTS-101', 'УТС-101', 22, '["B1", "B2"]'),
    ('UTS-102', 'УТС-102', 24, '["B1", "B2"]'),
    ('EE-101', 'ЭЭ-101', 20, '["B1", "B2"]'),
    ('EE-102', 'ЭЭ-102', 22, '["B1", "B2"]');

-- ============ ПРЕПОДАВАТЕЛИ ============
INSERT INTO teachers (id, name, department_id, max_weekly_hours, unavailable_slots, preferred_buildings) VALUES
    ('T1', 'Архипов М.С.', 'DEP-MATH', 24, '[]', '["B1"]'),
    ('T2', 'Белова Е.Н.', 'DEP-MATH', 20, '[]', '["B1"]'),
    ('T3', 'Володин А.К.', 'DEP-IT', 26, '[]', '["B1"]'),
    ('T4', 'Громова Л.П.', 'DEP-IT', 22, '[]', '["B1"]'),
    ('T5', 'Дмитриев С.В.', 'DEP-ETF', 24, '[]', '["B1", "B2"]'),
    ('T6', 'Ершова Т.А.', 'DEP-ETF', 20, '[]', '["B1"]'),
    ('T7', 'Жуков П.Н.', 'DEP-ETF', 24, '[]', '["B2"]'),
    ('T8', 'Зимина О.В.', 'DEP-HUM', 20, '[]', '["B1"]'),
    ('T9', 'Ильин Р.Д.', 'DEP-HUM', 16, '[]', '["B1"]'),
    ('T10', 'Крылова А.С.', 'DEP-HUM', 40, '[]', '["B1"]'),
    ('T11', 'Лебедев В.М.', 'DEP-IT', 22, '[]', '["B1"]'),
    ('T12', 'Морозов Д.А.', 'DEP-ETF', 20, '[]', '["B1", "B2"]'),
    ('T13', 'Новикова Е.А.', 'DEP-HUM', 20, '[]', '["B1"]');

-- ============ АУДИТОРИИ ============
INSERT INTO rooms (id, number, building_id, capacity, type) VALUES
    -- Главный корпус B1
    ('R001', 'Актовый зал', 'B1', 500, 'lecture'),
    ('R101', '101', 'B1', 300, 'lecture'),
    ('R102', '102', 'B1', 200, 'lecture'),
    ('R103', '103', 'B1', 80, 'lecture'),
    ('R201', '201', 'B1', 30, 'computer'),
    ('R202', '202', 'B1', 30, 'computer'),
    ('R203', '203', 'B1', 25, 'computer'),
    ('R301', '301', 'B1', 60, 'lecture'),
    ('R302', '302', 'B1', 50, 'lecture'),
    -- Лабораторный корпус B2
    ('R401', '401', 'B2', 30, 'lab'),
    ('R402', '402', 'B2', 30, 'lab'),
    ('R403', '403', 'B2', 25, 'lab'),
    ('R404', '404', 'B2', 20, 'lab'),
    ('R501', '501', 'B2', 120, 'lecture');

-- ============ ПРЕДМЕТЫ (ЛОГИЧНЫЕ ПОТОКИ ПО НАПРАВЛЕНИЯМ) ============

-- Английский язык — КАЖДАЯ ГРУППА ОТДЕЛЬНО (практика)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('ENG-ATP101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["ATP-101"]'),
    ('ENG-ATP102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["ATP-102"]'),
    ('ENG-IKT101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IKT-101"]'),
    ('ENG-IKT102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IKT-102"]'),
    ('ENG-IVT101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IVT-101"]'),
    ('ENG-IVT102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IVT-102"]'),
    ('ENG-IB101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IB-101"]'),
    ('ENG-IB102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["IB-102"]'),
    ('ENG-IBA101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["IBA-101"]'),
    ('ENG-IBA102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["IBA-102"]'),
    ('ENG-MR101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["MR-101"]'),
    ('ENG-MR102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["MR-102"]'),
    ('ENG-PI101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["PI-101"]'),
    ('ENG-PI102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T13', '["PI-102"]'),
    ('ENG-UTS101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["UTS-101"]'),
    ('ENG-UTS102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["UTS-102"]'),
    ('ENG-EE101', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["EE-101"]'),
    ('ENG-EE102', 'Английский язык', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10', '["EE-102"]');

-- АНГЛИЙСКИЙ: ещё группы (второй поток для IT, т.к. 8 групп много для одной аудитории)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('ENG-4', 'Английский язык (IT-2)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10',
     '["IVT-101","IVT-102","IB-101","IB-102"]'),
    ('ENG-5', 'Английский язык (IT-3)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T10',
     '["IBA-101","IBA-102","PI-101","PI-102"]');

-- Удаляем старые ENG, если остались (пропускаем)

-- ФИЗКУЛЬТУРА (6 потоков по 3 группы)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('PHYS-1', 'Физическая культура (1)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["ATP-101","ATP-102","IKT-101"]'),
    ('PHYS-2', 'Физическая культура (2)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["IKT-102","IVT-101","IVT-102"]'),
    ('PHYS-3', 'Физическая культура (3)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["IB-101","IB-102","IBA-101"]'),
    ('PHYS-4', 'Физическая культура (4)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["IBA-102","MR-101","MR-102"]'),
    ('PHYS-5', 'Физическая культура (5)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["PI-101","PI-102","UTS-101"]'),
    ('PHYS-6', 'Физическая культура (6)', 'DEP-HUM', 0, 2, 0, 'lecture', 'T9',
     '["UTS-102","EE-101","EE-102"]');

-- ФИЛОСОФИЯ (3 потока)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('PHIL-1', 'Философия (IT)', 'DEP-HUM', 2, 0, 0, 'lecture', 'T8',
     '["IVT-101","IVT-102","IB-101","IB-102","IBA-101","IBA-102","PI-101","PI-102"]'),
    ('PHIL-2', 'Философия (автоматика/связь)', 'DEP-HUM', 2, 0, 0, 'lecture', 'T8',
     '["ATP-101","ATP-102","IKT-101","IKT-102","UTS-101","UTS-102"]'),
    ('PHIL-3', 'Философия (мехатроника/энергетика)', 'DEP-HUM', 2, 0, 0, 'lecture', 'T8',
     '["MR-101","MR-102","EE-101","EE-102"]');

-- ВЫСШАЯ МАТЕМАТИКА (лекции потоками по факультетам)
-- Поток 1: IT-направления (ИВТ, ИБ, ИБА, ПИ = 8 групп)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('MATH-L1', 'Высшая математика (IT-поток)', 'DEP-MATH', 2, 0, 0, 'lecture', 'T1',
     '["IVT-101","IVT-102","IB-101","IB-102","IBA-101","IBA-102","PI-101","PI-102"]');

-- Поток 2: Автоматика + Связь + Управление + Мехатроника + Энергетика = 10 групп
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('MATH-L2', 'Высшая математика (технический поток)', 'DEP-MATH', 2, 0, 0, 'lecture', 'T1',
     '["ATP-101","ATP-102","IKT-101","IKT-102","MR-101","MR-102","UTS-101","UTS-102","EE-101","EE-102"]');

-- Практики по математике (каждая группа отдельно)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('MATH-PR-ATP101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["ATP-101"]'),
    ('MATH-PR-ATP102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["ATP-102"]'),
    ('MATH-PR-IKT101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IKT-101"]'),
    ('MATH-PR-IKT102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IKT-102"]'),
    ('MATH-PR-IVT101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IVT-101"]'),
    ('MATH-PR-IVT102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IVT-102"]'),
    ('MATH-PR-IB101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IB-101"]'),
    ('MATH-PR-IB102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IB-102"]'),
    ('MATH-PR-IBA101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IBA-101"]'),
    ('MATH-PR-IBA102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["IBA-102"]'),
    ('MATH-PR-MR101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["MR-101"]'),
    ('MATH-PR-MR102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["MR-102"]'),
    ('MATH-PR-PI101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["PI-101"]'),
    ('MATH-PR-PI102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["PI-102"]'),
    ('MATH-PR-UTS101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["UTS-101"]'),
    ('MATH-PR-UTS102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["UTS-102"]'),
    ('MATH-PR-EE101', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["EE-101"]'),
    ('MATH-PR-EE102', 'Высшая математика (пр)', 'DEP-MATH', 0, 2, 0, 'lecture', 'T2', '["EE-102"]');

-- БАЗЫ ДАННЫХ (лекция для IT-направлений)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('DB-L', 'Базы данных (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T3',
     '["IVT-101","IVT-102","IB-101","IB-102","IBA-101","IBA-102","PI-101","PI-102"]');

-- Базы данных (лабы по 2 группы)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('DB-LAB-1', 'Базы данных (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T3', '["IVT-101","IVT-102"]'),
    ('DB-LAB-2', 'Базы данных (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T3', '["IB-101","IB-102"]'),
    ('DB-LAB-3', 'Базы данных (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T3', '["IBA-101","IBA-102"]'),
    ('DB-LAB-4', 'Базы данных (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T3', '["PI-101","PI-102"]');

-- ПРОГРАММИРОВАНИЕ (лекция для IT)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('PROG-L', 'Программирование (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T4',
     '["IVT-101","IVT-102","IB-101","IB-102","PI-101","PI-102"]');

-- Программирование (лабы)
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('PROG-LAB-1', 'Программирование (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T11', '["IVT-101","IVT-102"]'),
    ('PROG-LAB-2', 'Программирование (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T11', '["IB-101","IB-102"]'),
    ('PROG-LAB-3', 'Программирование (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T11', '["PI-101","PI-102"]');

-- ============ СПЕЦИАЛЬНЫЕ ПРЕДМЕТЫ ============
-- АТП
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('TAU-L', 'Теория авт. управления (лекция)', 'DEP-ETF', 2, 0, 0, 'lecture', 'T5', '["ATP-101","ATP-102"]'),
    ('TAU-PR', 'Теория авт. управления (пр)', 'DEP-ETF', 0, 2, 0, 'lecture', 'T5', '["ATP-101","ATP-102"]'),
    ('TAU-LAB', 'Теория авт. управления (лаб)', 'DEP-ETF', 0, 0, 2, 'lab', 'T5', '["ATP-101","ATP-102"]');

-- ИКТ
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('NET-L', 'Сети и системы связи (лекция)', 'DEP-ETF', 2, 0, 0, 'lecture', 'T6', '["IKT-101","IKT-102"]'),
    ('NET-LAB', 'Сети и системы связи (лаб)', 'DEP-ETF', 0, 0, 2, 'lab', 'T6', '["IKT-101","IKT-102"]');

-- ИВТ
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('ARCH-L', 'Архитектура ЭВМ (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T11', '["IVT-101","IVT-102"]'),
    ('ARCH-LAB', 'Архитектура ЭВМ (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T11', '["IVT-101","IVT-102"]');

-- ИБ
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('CRYPTO-L', 'Криптография (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T3', '["IB-101","IB-102"]'),
    ('CRYPTO-LAB', 'Криптография (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T3', '["IB-101","IB-102"]');

-- ИБА
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('SEC-L', 'Защита АС (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T4', '["IBA-101","IBA-102"]'),
    ('SEC-PR', 'Защита АС (пр)', 'DEP-IT', 0, 2, 0, 'lecture', 'T4', '["IBA-101","IBA-102"]');

-- МР
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('ROBO-L', 'Робототехника (лекция)', 'DEP-ETF', 2, 0, 0, 'lecture', 'T7', '["MR-101","MR-102"]'),
    ('ROBO-LAB', 'Робототехника (лаб)', 'DEP-ETF', 0, 0, 2, 'lab', 'T7', '["MR-101","MR-102"]');

-- ПИ
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('SE-L', 'Технологии разработки ПО (лекция)', 'DEP-IT', 2, 0, 0, 'lecture', 'T11', '["PI-101","PI-102"]'),
    ('SE-LAB', 'Технологии разработки ПО (лаб)', 'DEP-IT', 0, 0, 2, 'computer', 'T11', '["PI-101","PI-102"]');

-- УТС
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('CTRL-L', 'Теория управления (лекция)', 'DEP-ETF', 2, 0, 0, 'lecture', 'T5', '["UTS-101","UTS-102"]'),
    ('CTRL-PR', 'Теория управления (пр)', 'DEP-ETF', 0, 2, 0, 'lecture', 'T5', '["UTS-101","UTS-102"]');

-- ЭЭ
INSERT INTO subject_plans (id, name, department_id, lecture_hours, practice_hours, lab_hours, requires_room_type, teacher_id, group_ids) VALUES
    ('ELMACH-L', 'Электрические машины (лекция)', 'DEP-ETF', 2, 0, 0, 'lecture', 'T12', '["EE-101","EE-102"]'),
    ('ELMACH-LAB', 'Электрические машины (лаб)', 'DEP-ETF', 0, 0, 2, 'lab', 'T12', '["EE-101","EE-102"]');