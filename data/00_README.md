# Шаблоны для заполнения базы данных

## Порядок заполнения

Файлы нужно выполнять строго по номерам (есть зависимости по внешним ключам):

```
01_buildings.sql      → корпуса
02_rooms.sql          → аудитории (ссылаются на buildings)
03_departments.sql    → кафедры
04_teachers.sql       → преподаватели (ссылаются на departments)
05_groups.sql         → группы
06_subject_plans.sql  → учебные планы (ссылаются на teachers и groups)
```

## Как запустить

```bash
# Один файл:
psql postgres://postgres:postgres@127.0.0.1:5432/scheduler -f data/01_buildings.sql

# Все сразу:
psql postgres://postgres:postgres@127.0.0.1:5432/scheduler \
  -f data/01_buildings.sql \
  -f data/02_rooms.sql \
  -f data/03_departments.sql \
  -f data/04_teachers.sql \
  -f data/05_groups.sql \
  -f data/06_subject_plans.sql
```

## Ключевые правила

### Аудитории (rooms)
- `type = 'lecture'` — обычная аудитория
- `type = 'lab'`     — лаборатория (нужна для занятий с `requires_room_type = 'lab'`)

### Преподаватели (teachers)
- `max_weekly_hours` — в академических часах (1 пара = 2 часа). 8 пар/нед = 16 часов
- `preferred_buildings` — `'[]'` означает "любой корпус"
- `unavailable_slots` — `'[]'` означает "всегда доступен"

### Учебные планы (subject_plans)
- Лекции для потока: `group_ids` = несколько групп
- Практики/лабы: ОДНА запись НА КАЖДУЮ группу отдельно
- `lecture_hours`, `practice_hours`, `lab_hours` — это **пары в неделю**, не часы
- `required_building_id = 'D'` — занятие всегда в корпусе Д (математика, философия и т.п.)
- `semester_half = 'full'` — весь семестр (по умолчанию)

### Чётность (parity)
- `always` — каждую неделю
- `odd`    — только нечётные недели
- `even`   — только чётные недели
