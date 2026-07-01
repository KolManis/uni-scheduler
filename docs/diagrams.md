# Диаграммы системы автоматического составления расписания занятий

**Тема дипломной работы:** Проектирование и разработка моделей и методов системы автоматического составления расписания занятий  
**Язык реализации:** Go 1.22+  
**СУБД:** PostgreSQL 16

---

## 1. Диаграмма вариантов использования (Use Case)

```mermaid
graph TD
    Actor(["Пользователь\n(составитель расписания)"])

    subgraph System ["Система составления расписания"]
        UC1["Загрузить данные из Excel"]
        UC2["Просмотреть справочники\n(преподаватели, группы, аудитории)"]
        UC3["Сгенерировать расписание"]
        UC4["Просмотреть список расписаний"]
        UC5["Просмотреть расписание"]
        UC6["Редактировать назначение вручную"]
        UC7["Экспортировать расписание в Excel"]
        UC8["Удалить расписание"]

        UC3 -->|include| UC3a["Проверить жёсткие ограничения HC1–HC9"]
        UC3 -->|include| UC3b["Вычислить штрафную функцию SC1–SC7"]
        UC3 -->|extend| UC3c["Запустить Local Search 2-opt"]
        UC6 -->|include| UC6a["Проверить конфликты HC1–HC9"]
    end

    Actor --> UC1
    Actor --> UC2
    Actor --> UC3
    Actor --> UC4
    Actor --> UC5
    Actor --> UC6
    Actor --> UC7
    Actor --> UC8
```

---

## 2. ER-диаграмма (Концептуальная модель данных)

```mermaid
erDiagram
    BUILDING {
        text id PK
        text name
        text address
    }

    DEPARTMENT {
        text id PK
        text name
    }

    GROUP {
        text id PK
        text name
        int student_count
        jsonb building_ids
    }

    TEACHER {
        text id PK
        text name
        text department_id FK
        int max_weekly_hours
        jsonb unavailable_slots
        jsonb preferred_buildings
    }

    ROOM {
        text id PK
        text number
        text building_id FK
        int capacity
        text type
    }

    SUBJECT_PLAN {
        text id PK
        text name
        text department_id FK
        text teacher_id FK
        jsonb group_ids
        int lecture_hours
        int practice_hours
        int lab_hours
        text requires_room_type
        text parity
    }

    SCHEDULE {
        bigserial id PK
        text name
        jsonb assignments
        int score
        timestamptz created_at
    }

    DEPARTMENT ||--o{ TEACHER : "относится к"
    DEPARTMENT ||--o{ SUBJECT_PLAN : "относится к"
    BUILDING ||--o{ ROOM : "содержит"
    TEACHER ||--o{ SUBJECT_PLAN : "ведёт"
    SCHEDULE ||--o{ SUBJECT_PLAN : "включает (через assignments)"
```

> **Примечание.** Поля типа `jsonb` используются для хранения массивов идентификаторов (`group_ids`, `building_ids`) и вложенных объектов (`assignments`, `unavailable_slots`). Это позволяет избежать дополнительных связующих таблиц и обеспечивает гибкость при изменении структуры данных.

---

## 3. Компонентная диаграмма (Архитектура системы)

```mermaid
graph TB
    subgraph Client["Клиент (HTTP)"]
        HTTP["HTTP-запросы\nPOST / GET / PATCH / DELETE"]
    end

    subgraph Transport["Транспортный слой — internal/transport/http"]
        R["router.go\ngorilla/mux"]
        H1["schedule_handler.go\nGenerate · List · GetByID · Delete · Patch"]
        H2["excel_handler.go\nExport xlsx"]
        H3["import_handler.go\nImport xlsx"]
        H4["reference_handler.go\nTeachers · Groups · Rooms"]
        D["dto.go\nRequest / Response типы"]
    end

    subgraph Usecase["Слой бизнес-логики — internal/usecase/generator"]
        P["ports.go\nInputRepository · OutputRepository · Usecase"]
        S["service.go\nGenerate · GetByID · List · Delete · Patch"]
    end

    subgraph Solver["Слой алгоритмов — internal/solver"]
        TS["teacher_solver.go\nSolveTeacher — жадный алгоритм"]
        BT["backtrack.go\nSolveParallel — возврат с параллелизмом"]
        LS["local_search.go\nLocalSearch — локальный поиск 2-opt"]
        CN["constraints.go\nПроверка HC1–HC9"]
        FT["fitness.go\ncalculateFitness — SC1–SC7"]
        HE["heuristics.go\nselectMostConstrained · generateCandidates"]
    end

    subgraph Domain["Слой домена — internal/domain/schedule"]
        M["models.go\nBuilding · Department · Group · Teacher\nRoom · SubjectPlan · Assignment · Schedule"]
        E["errors.go\nErrConflict · ErrNotFound · ErrNoSolution"]
    end

    subgraph Repository["Слой репозитория — internal/repository/postgres"]
        IR["input_repo.go\nLoadInput — загрузка 6 таблиц"]
        SR["schedule_repo.go\nSave · Get · List · Delete · Update"]
        IMR["import_repo.go\nUPSERT сущностей из Excel"]
    end

    subgraph Importer["Модуль импорта — internal/importer"]
        EP["excel_parser.go\nПарсинг листов xlsx"]
        CP["cell_parser.go\nРазбор ячейки по regex"]
        IM["models.go\nImportedData · ImportedLesson"]
    end

    subgraph Infra["Инфраструктура"]
        PG["PostgreSQL 16\n7 таблиц · JSONB · индексы"]
        MIG["migrations/\n001_init.sql · 002_seed_etf.sql"]
        DC["Docker Compose\napp + db"]
        POOL["pool.go\npgxpool с retry"]
    end

    HTTP --> R
    R --> H1 & H2 & H3 & H4
    H1 & H2 & H3 & H4 --> S
    S --> P
    S --> TS
    TS --> BT
    TS & BT --> LS
    LS --> CN & FT
    TS & BT --> HE
    CN & FT --> M
    S --> IR & SR & IMR
    H3 --> EP
    EP --> CP
    EP --> IM
    IR & SR & IMR & POOL --> PG
    MIG --> PG
    DC --> PG
```

---

## 4. Блок-схема алгоритма генерации расписания

### 4.1 Основной алгоритм (Teacher-Driven Greedy)

```mermaid
flowchart TD
    A([Начало]) --> B[Загрузить InputData из БД\nLoadInput]
    B --> C[Отсортировать преподавателей\nASC по max_weekly_hours\nDESC по кол-ву предметов]
    C --> D{Есть необработанные\nпреподаватели?}
    D -- Нет --> K
    D -- Да --> E[Получить предметы преподавателя\nОтсортировать по кол-ву групп DESC,\nсуммарным часам DESC]
    E --> F{Есть предмет\nс оставшимися часами?}
    F -- Нет --> D
    F -- Да --> G[Для типа занятия lec / prac / lab\nвызвать findBestSlot]
    G --> H{Слот найден?}
    H -- Нет --> I[Записать предупреждение\nПропустить занятие]
    I --> F
    H -- Да --> J[assign — зафиксировать назначение\nОбновить occupied maps]
    J --> F
    K{Все предметы\nразмещены?}
    K -- Нет --> L[Fallback: SolveParallel\nBacktracking + MRV\n4 параллельных горутины]
    K -- Да --> M[LocalSearch — 2-opt swap\nwhile score улучшается]
    L --> M
    M --> N[calculateFitness\nПодсчитать штраф SC1–SC7]
    N --> O[SaveSchedule в БД]
    O --> P([Конец — вернуть Schedule])
```

### 4.2 Функция findBestSlot (выбор слота)

```mermaid
flowchart TD
    A([Начало findBestSlot]) --> B[Перебрать дни в порядке:\nПн → Вт → Ср → Чт → Пт → Сб]
    B --> C[Перебрать пары 1 → 6]
    C --> D{Преподаватель\nдоступен?\nHC7}
    D -- Нет --> C
    D -- Да --> E{Слот свободен\nдля преподавателя?\nHC1 + HC9}
    E -- Нет --> C
    E -- Да --> F{Слот свободен\nдля всех групп?\nHC2 + HC9}
    F -- Нет --> C
    F -- Да --> G[Day-aware бонус:\nпредпочесть слот вплотную\nк уже стоящим парам группы]
    G --> H[Перебрать аудитории\nпо вместимости DESC]
    H --> I{Тип аудитории\nподходит?\nHC5}
    I -- Нет --> H
    I -- Да --> J{Аудитория свободна?\nHC3 + HC9}
    J -- Нет --> H
    J -- Да --> K{Вместимость\nдостаточна?\nHC6}
    K -- Нет --> H
    K -- Да --> L{Корпус допустим\nдля групп и\nпреподавателя?\nHC8}
    L -- Нет --> H
    L -- Да --> M([Вернуть slot + room])
    H -- Аудитории\nкончились --> C
    C -- Пары\nкончились --> B
    B -- Дни\nкончились --> N([Вернуть: не найдено])
```

### 4.3 Local Search 2-opt

```mermaid
flowchart TD
    A([Начало LocalSearch]) --> B[improvement = true]
    B --> C{improvement == true?}
    C -- Нет --> Z([Конец — вернуть улучшенное расписание])
    C -- Да --> D[improvement = false]
    D --> E[Перебрать все пары назначений a1, a2]
    E --> F[Попробовать swap a1.TimeSlot ↔ a2.TimeSlot]
    F --> G{Нарушаются\nHC1–HC9?}
    G -- Да --> E
    G -- Нет --> H[Вычислить newScore]
    H --> I{newScore < currentScore?}
    I -- Нет --> E
    I -- Да --> J[Применить swap\ncurrentScore = newScore\nimprovement = true]
    J --> E
    E -- Все пары\nперебраны --> C
```

---

## 5. Диаграммы последовательностей

### 5.1 Генерация расписания

```mermaid
sequenceDiagram
    participant C as Client
    participant H as ScheduleHandler
    participant S as Service
    participant IR as InputRepo
    participant SL as Solver
    participant SR as ScheduleRepo

    C->>H: POST /api/v1/schedules/generate\n{name, solver_type, timeout_sec}
    H->>S: Generate(ctx, req)
    S->>IR: LoadInput()
    IR-->>S: InputData {teachers, groups, rooms, plans}
    S->>SL: SolveTeacher(input, maxIter)
    Note over SL: Greedy → Local Search 2-opt
    alt Не все предметы размещены
        SL->>SL: SolveParallel(input, 100000, 4)
        Note over SL: Backtracking + MRV, 4 горутины
    end
    SL-->>S: Schedule {assignments, score}
    S->>SR: SaveSchedule(schedule)
    SR-->>S: id
    S-->>H: Schedule
    H-->>C: 201 Created {id, name, score, assignments}
```

### 5.2 Импорт данных из Excel

```mermaid
sequenceDiagram
    participant C as Client
    participant H as ImportHandler
    participant EP as ExcelParser
    participant CP as CellParser
    participant IR as ImportRepo
    participant DB as PostgreSQL

    C->>H: POST /api/v1/import/excel\n(multipart/form-data, xlsx файл)
    H->>EP: ParseExcel(file)
    loop Для каждого листа (преподаватель)
        EP->>EP: Извлечь имя преподавателя из строки 1
        loop Для каждой строки данных
            EP->>CP: ParseCell(rawText)
            Note over CP: regex: ^(1н|2н)?\s*\((\w+)\)\s*(.+?)\s+гр\.(.+)\s+(\w+)\s+к\.(\w+)
            CP-->>EP: {parity, type, subject, groups[], room, building}
        end
    end
    EP-->>H: ImportedData
    H->>IR: UpsertAll(ImportedData)
    IR->>DB: UPSERT buildings, departments
    IR->>DB: UPSERT teachers
    IR->>DB: UPSERT groups, rooms
    IR->>DB: UPSERT subject_plans
    DB-->>IR: ok
    IR-->>H: ImportResult
    H-->>C: 200 OK {teachers_created, groups_created,\nsubjects_created, rooms_created, warnings[]}
```

### 5.3 Ручное редактирование назначения

```mermaid
sequenceDiagram
    participant C as Client
    participant H as ScheduleHandler
    participant S as Service
    participant SR as ScheduleRepo
    participant CN as Constraints
    participant FT as Fitness

    C->>H: PATCH /api/v1/schedules/{id}/assignments/{idx}\n{time_slot, room_id, parity}
    H->>S: PatchAssignment(id, idx, req)
    S->>SR: GetSchedule(id)
    SR-->>S: Schedule
    S->>S: Проверить idx < len(assignments)
    S->>S: Применить изменение к assignments[idx]
    S->>CN: CheckHardConstraints(modified assignment, all assignments)
    alt Конфликт найден
        CN-->>S: ConflictError {type, resource_id, conflict_with_idx}
        S-->>H: ConflictError
        H-->>C: 409 Conflict {type, resource_id, conflict_with}
    else Конфликтов нет
        CN-->>S: ok
        S->>FT: calculateFitness(assignments, input)
        FT-->>S: новый score
        S->>SR: UpdateSchedule(schedule)
        SR-->>S: ok
        S-->>H: обновлённый Schedule
        H-->>C: 200 OK {schedule с новым score}
    end
```

### 5.4 Экспорт расписания в Excel

```mermaid
sequenceDiagram
    participant C as Client
    participant H as ExcelHandler
    participant S as Service
    participant SR as ScheduleRepo

    C->>H: GET /api/v1/schedules/{id}/excel\n?type=group&id=G1&week=both
    H->>S: GetByID(id)
    S->>SR: GetSchedule(id)
    SR-->>S: Schedule {assignments}
    S-->>H: Schedule
    H->>H: Отфильтровать assignments по group/teacher
    H->>H: Сформировать xlsx через excelize\n(строки: дни × пары, столбцы: чётность)
    H-->>C: 200 OK\nContent-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\n(файл .xlsx)
```

---

## 6. Диаграмма состояний расписания

```mermaid
stateDiagram-v2
    [*] --> Черновик : POST /schedules/generate (запрос)

    Черновик --> Генерация : Solver запущен

    Генерация --> Сгенерировано : Все HC выполнены\nScore вычислен

    Генерация --> Ошибка : Не удалось разместить\nвсе предметы (422)

    Ошибка --> [*]

    Сгенерировано --> Редактируется : PATCH /assignments/{idx}

    Редактируется --> Сгенерировано : HC нарушены → откат (409)

    Редактируется --> Отредактировано : HC соблюдены\nScore пересчитан (200)

    Отредактировано --> Редактируется : PATCH /assignments/{idx}

    Сгенерировано --> Экспортировано : GET /excel

    Отредактировано --> Экспортировано : GET /excel

    Экспортировано --> Редактируется : PATCH /assignments/{idx}

    Сгенерировано --> [*] : DELETE /schedules/{id}
    Отредактировано --> [*] : DELETE /schedules/{id}
    Экспортировано --> [*] : DELETE /schedules/{id}
```

---

## 7. Схема базы данных (DDL)

```sql
-- Корпуса
CREATE TABLE buildings (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    address TEXT
);

-- Кафедры
CREATE TABLE departments (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

-- Учебные группы
CREATE TABLE groups (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    student_count INT  NOT NULL DEFAULT 25,
    building_ids  JSONB NOT NULL DEFAULT '[]'
);

-- Преподаватели
CREATE TABLE teachers (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    department_id       TEXT REFERENCES departments(id),
    max_weekly_hours    INT  NOT NULL DEFAULT 18,
    unavailable_slots   JSONB NOT NULL DEFAULT '[]',
    preferred_buildings JSONB NOT NULL DEFAULT '[]'
);

-- Аудитории
CREATE TABLE rooms (
    id          TEXT PRIMARY KEY,
    number      TEXT NOT NULL,
    building_id TEXT NOT NULL REFERENCES buildings(id),
    capacity    INT  NOT NULL DEFAULT 30,
    type        TEXT NOT NULL CHECK (type IN ('lecture', 'lab', 'pc'))
);

-- Учебные планы
CREATE TABLE subject_plans (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    department_id      TEXT REFERENCES departments(id),
    teacher_id         TEXT NOT NULL REFERENCES teachers(id),
    group_ids          JSONB NOT NULL DEFAULT '[]',
    lecture_hours      INT  NOT NULL DEFAULT 0,
    practice_hours     INT  NOT NULL DEFAULT 0,
    lab_hours          INT  NOT NULL DEFAULT 0,
    requires_room_type TEXT NOT NULL DEFAULT 'lecture'
                           CHECK (requires_room_type IN ('lecture', 'lab', 'pc')),
    parity             TEXT NOT NULL DEFAULT 'always'
                           CHECK (parity IN ('always', 'even', 'odd'))
);

-- Готовые расписания
CREATE TABLE schedules (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    assignments JSONB NOT NULL DEFAULT '[]',
    score       INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Индексы
CREATE INDEX idx_subject_plans_teacher ON subject_plans(teacher_id);
CREATE INDEX idx_schedules_score       ON schedules(score ASC);
```

---

## 8. Таблица жёстких ограничений

| № | Ограничение | Проверяемый ресурс | Функция |
|---|---|---|---|
| HC1 | Преподаватель не ведёт два занятия в один слот × чётность | Teacher | `isSlotFree(teacher)` |
| HC2 | Группа не имеет двух занятий в один слот × чётность | Group | `isSlotFree(group)` |
| HC3 | Аудитория не используется двумя занятиями одновременно | Room | `isSlotFree(room)` |
| HC4 | Кол-во пар каждого типа не превышает лимит SubjectPlan | SubjectPlan | `getRemainingHours()` |
| HC5 | Тип аудитории соответствует требованию предмета | Room | `isRoomSuitable()` |
| HC6 | Вместимость аудитории ≥ суммарное число студентов групп | Room | `isRoomBigEnough()` |
| HC7 | Преподаватель не назначается в слот из UnavailableSlots | Teacher | `isTeacherAvailable()` |
| HC8 | Аудитория находится в корпусе, допустимом для всех групп | Building | `isBuildingAllowed()` |
| HC9 | Чётность занятия (even/odd/always) учитывается при проверке конфликтов | Parity | `parityConflicts()` |

---

## 9. Таблица мягких ограничений и штрафной функции

| № | Ограничение | Штраф | Обоснование веса |
|---|---|---|---|
| SC1 | Пара поставлена в субботу | +200 за пару | Нежелательно, но допустимо |
| SC2 | День группы длиннее 4 пар подряд | +300–500 | Высокая учебная нагрузка |
| SC3 | Концентрация нагрузки (мало дней) | +200–600 | Неравномерность по неделе |
| SC4 | Окна у групп (gap между парами в день) | **+10 000 × N** | Критично: студенты ждут впустую |
| SC5 | Окна у преподавателей | +60 × N | Нежелательно, меньший приоритет |
| SC6 | Переход между корпусами в смежных парах | +150 | Физически затруднён за 10 мин |
| SC7 | Одна пара в день у группы («форточка») | +25 | Минимальный дискомфорт |

**Целевое значение:** `score < 10 000` при `placement_rate = 100%`

---

## 10. Описание формата входного файла Excel

### Структура листа (один лист = один преподаватель)

| Строка | Содержимое |
|---|---|
| 0 | Заголовок: «Расписание занятий на 2024/25 уч. год (кафедра ИТАС)» |
| 1 | «Преподаватель Доц. Петренко А.А.» |
| 2 | Заголовки столбцов: Дни / Часы / Дисциплина, группа, аудитория |
| 3+ | Данные (forward-fill для столбца «Дни») |

### Формат ячейки занятия

```
[чётность] (тип) название_предмета гр.ГРУППА [, ГРУППА2] аудитория к.КОРПУС (кафедра)
```

**Примеры:**

```
(лаб) Дискретная математика гр.РИС -23-2б 226 к.А (ЭТФ)
1н (лаб) Объектно-ориентированное программирование гр.РИС -23-1б 128 к.А (ЭТФ)
(лек) Сети и телекоммуникации гр.АСУ -22-1б, ИКС -22-1б, РИС -22-1б 402 к.А (ЭТФ)
```

Если в одном слоте две записи (чётная/нечётная неделя) — разделены символом `\n`.

### Регулярное выражение для разбора ячейки

```
^(1н|2н)?\s*\((\w+\.?)\)\s*(.+?)\s+гр\.([\w,\s\-\/]+)\s+(\w+)\s+к\.(\w+)
```

| Группа | Значение | Пример |
|---|---|---|
| `$1` | Чётность (1н / 2н / пусто) | `1н` |
| `$2` | Тип занятия | `лаб` |
| `$3` | Название предмета | `Дискретная математика` |
| `$4` | Группы через запятую | `РИС -23-1б, РИС -23-2б` |
| `$5` | Номер аудитории | `226` |
| `$6` | Корпус | `А` |

### Маппинги

```
Дни:      ПОНЕДЕЛЬНИК→monday, ВТОРНИК→tuesday, СРЕДА→wednesday,
          ЧЕТВЕРГ→thursday, ПЯТНИЦА→friday, СУББОТА→saturday

Часы:     8:00→1, 9:40→2, 11:30→3, 13:20→4, 15:00→5, 16:40→6

Типы:     лек→lecture, лаб→lab, пр→practice, кср→practice, у.л.→lecture

Чётность: 1н→odd, 2н→even, (пусто)→always
```

---

## 11. Нефункциональные требования и метрики качества

| Метрика | Описание | Целевое значение |
|---|---|---|
| `score` | Суммарный штраф по SC1–SC7 | < 10 000 |
| `gap_ratio` | Доля пар групп с окнами / всего пар | < 5% |
| `saturday_ratio` | Доля пар в субботу | < 2% |
| `placement_rate` | Доля размещённых предметов | 100% |
| `solve_time_ms` | Время работы алгоритма | < 30 000 мс |

| Требование | Значение |
|---|---|
| Язык реализации | Go 1.22+ |
| СУБД | PostgreSQL 16 |
| Контейнеризация | Docker Compose |
| HTTP-фреймворк | gorilla/mux v1.8.1 |
| Драйвер БД | jackc/pgx v5 |
| Работа с Excel | xuri/excelize v2 |
| Покрытие unit-тестами | > 70% (solver, parser) |
