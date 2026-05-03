-- Корпуса
CREATE TABLE IF NOT EXISTS buildings (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT ''
);

-- Кафедры
CREATE TABLE IF NOT EXISTS departments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

-- Обновлённые таблицы
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS building_id TEXT REFERENCES buildings(id);
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS department_id TEXT REFERENCES departments(id);
ALTER TABLE subject_plans ADD COLUMN IF NOT EXISTS department_id TEXT REFERENCES departments(id);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS building_ids JSONB DEFAULT '[]';

-- Данные
INSERT INTO buildings VALUES
    ('B1', 'Главный корпус', 'ул. Ленина, 15'),
    ('B2', 'Корпус Б', 'ул. Пушкина, 10');

INSERT INTO departments VALUES
    ('D1', 'Кафедра высшей математики'),
    ('D2', 'Кафедра информатики');