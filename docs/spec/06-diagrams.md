# Диаграммы

Все диаграммы описывают текущий код. ER-диаграмма — в [02-data-model.md](02-data-model.md),
схема алгоритма — в [03-algorithm.md](03-algorithm.md).

## Варианты использования

```mermaid
flowchart LR
  U((Составитель<br/>расписания))
  X((Внешняя<br/>система))
  U --> R[Вести справочники<br/>и учебные планы]
  U --> I[Импортировать<br/>справочники из Excel]
  U --> G[Сгенерировать расписание]
  U --> C[Сравнить методы<br/>улучшения]
  U --> V[Посмотреть расписание<br/>и его качество]
  U --> M[Перенести пару вручную]
  U --> E[Выгрузить в Excel]
  U --> H[Открыть справку]
  X --> G
  X --> V
  X --> M
  X --> E
  C -.->|включает| G
  M -.->|проверяет| K[Жёсткие ограничения]
  G -.->|проверяет| K
```

## Компоненты

Гексагональная архитектура: ядро (`internal/core`) не импортирует адаптеры (ADR-0001).

```mermaid
flowchart TB
  subgraph in["Входящие адаптеры — internal/adapters/in"]
    REST["http/rest<br/>JSON API /api/v1"]
    WEB["http/web<br/>интерфейс /ui, шаблоны"]
    XL["excel<br/>разбор xlsx"]
  end
  subgraph core["Ядро — internal/core"]
    APP["application<br/>команды и запросы:<br/>генерация, перенос, импорт…"]
    SOLVER["solver<br/>построение, поиск, оценка"]
    DOMAIN["domain<br/>типы"]
    PORTS["ports<br/>интерфейсы хранилищ"]
  end
  subgraph out["Исходящие адаптеры — internal/adapters/out"]
    PG["postgres<br/>хранилище"]
  end
  DB[(PostgreSQL)]

  REST --> APP
  WEB --> APP
  REST --> XL
  APP --> SOLVER
  APP --> PORTS
  APP --> DOMAIN
  SOLVER --> DOMAIN
  PG -.->|реализует| PORTS
  PG --> DB
```

## Генерация расписания

```mermaid
sequenceDiagram
  participant C as Клиент
  participant H as REST или веб
  participant S as generateschedule.Handler
  participant IR as InputRepository
  participant SL as solver
  participant OR as OutputRepository

  C->>H: POST generate {name, timeout_sec, solver_type, improve_algo, правила}
  H->>H: generateschedule.NewCommand — проверить solver_type, значения по умолчанию
  H->>S: Handle(команда)
  S->>IR: LoadInput()
  IR-->>S: справочники и планы
  S->>SL: SolveMultiStart(данные + правила, построение, бюджет = таймаут − 15 с)
  Note over SL: нормализация порядка → построение →<br/>вставка непоставленных → сходимость → метаэвристика
  SL-->>S: пары, score
  S->>SL: ComputeUnplaced(пары, данные) + причины неразмещения
  S->>OR: SaveSchedule(пары, score, unplaced, options)
  Note over S,OR: генерация и сохранение не зависят от запроса:<br/>закрытая вкладка их не прерывает
  OR-->>S: сохранённое расписание
  S-->>H: расписание
  H-->>C: 201 / страница списка
```

«Все методы — сравнить» запускает пять таких генераций параллельно, по одной на метод,
и сохраняет пять расписаний.

## Ручной перенос пары

```mermaid
sequenceDiagram
  participant C as Клиент
  participant S as moveassignment.Handler
  participant OR as OutputRepository
  participant IR as InputRepository
  participant SL as solver

  C->>S: Handle(moveassignment.Command{id, idx, слот, аудитория, чётность})
  S->>OR: GetSchedule(id)
  S->>IR: LoadInput()
  alt слот недоступен преподавателю
    S-->>C: 409 teacher_unavailable
  else занят преподаватель, группа или аудитория
    S-->>C: 409 teacher_busy / group_busy / room_busy
  else конфликтов нет
    S->>SL: CalculateFitness(пары, данные + правила расписания)
    S->>OR: UpdateSchedule
    S-->>C: 200, новое расписание
  end
```

## Импорт справочников из Excel

```mermaid
sequenceDiagram
  participant C as Клиент
  participant H as rest.ImportHandler
  participant P as excel
  participant S as importexcel.Handler
  participant R as ImportRepository

  C->>H: POST /import/excel (xlsx)
  H->>P: разобрать лист
  P-->>H: преподаватели, группы, аудитории, планы
  H->>S: Handle(importexcel.Command{данные})
  S->>R: UpsertAll
  R-->>C: 200 {создано/обновлено по сущностям, предупреждения}
```

## Состояния расписания

```mermaid
stateDiagram-v2
  [*] --> Генерируется: «Сгенерировать»
  Генерируется --> Сохранено: солвер завершил работу
  Генерируется --> Ошибка: таймаут / неверные параметры
  Сохранено --> Сохранено: ручной перенос пары
  Сохранено --> [*]: «Удалить»
```
