# Контракты API

База: `/api/v1`, JSON в UTF-8. Ошибки: `{"error": "текст"}`.

## Генерация

`POST /schedules/generate`

```json
{
  "name": "Осень 2026",
  "timeout_sec": 120,
  "solver_type": "dsatur",
  "improve_algo": "hillclimb",
  "parallel_starts": 4,
  "semester_half": "",
  "lecture_practice_same_day": false,
  "lecture_before_practice": false,
  "same_subject_same_day": false
}
```

| Поле | По умолчанию | Значения |
|---|---|---|
| `timeout_sec` | 120 | секунды; бюджет солвера — на 15 с меньше |
| `improve_algo` | `hillclimb` | `hillclimb`, `sa`, `tabu`, `ga`, `lns` |
| `parallel_starts` | 4 | сколько раз составить параллельно, берётся лучшее; 1 — один запуск |
| `semester_half` | `""` | `second` исключает планы только первой половины |
| `solver_type` | `""` | построение: `""` или `dsatur` — самая трудная пара первой, `teacher` — по преподавателям (ADR-0015, ADR-0019); иначе 400 |

Ответы: **201** — расписание целиком; **400** — неверные параметры; **500** — ошибка генерации
или таймаут.

У непоставленных пар в `unplaced` есть поле `reason` — причина человеческими словами: нет
подходящей аудитории, преподавателю не хватает свободного времени, нет общего свободного времени
у преподавателя и групп.

## Проверка данных

`GET /input/check` — ошибки в справочниках и планах до генерации, `[]` — ошибок нет.

```json
[{ "severity": "error", "object": "План «Физика»", "message": "нет аудитории типа «lab» на 50 мест в допустимых корпусах" }]
```

`severity`: `error` — пару точно не поставить; `warning` — поставить можно, но результат будет не
таким, как ждут (нечётные часы, нагрузка больше указанного максимума).

## Расписания

| Запрос | Ответ |
|---|---|
| `GET /schedules` | 200, список без пар: `id, name, score, total_pairs, unplaced_count, created_at`, свежие сверху |
| `GET /schedules/{id}` | 200 — расписание; 404 |
| `DELETE /schedules/{id}` | 204; 404 |
| `GET /schedules/{id}/excel?type=all_teachers\|all_groups` | 200, файл xlsx |

## Ручной перенос пары

`PATCH /schedules/{id}/assignments/{idx}`

```json
{ "time_slot": {"day": "tuesday", "pair_num": 3}, "room_id": "…", "parity": "always" }
```

| Ответ | Когда |
|---|---|
| 200 | перенос сохранён, score пересчитан |
| 409 | конфликт: `{"type": "teacher_busy \| group_busy \| room_busy \| teacher_unavailable \| teacher_external_pair \| room_unsuitable", "resource_id": "…", "conflict_with": N}`; у `teacher_unavailable`, `teacher_external_pair` и `room_unsuitable` `conflict_with` = −1; у `teacher_external_pair` ещё `detail` (пометка пары на другом факультете) и `parity` (её неделя); у `room_unsuitable` `detail` — чем аудитория не подходит: тип, вместимость или корпус (проверяется, только если аудитория меняется) |
| 404 | нет расписания |

При переносе в другую аудиторию корпус пары (`building_id`) берётся из аудитории.

`POST /schedules/{id}/replay` — повторить запуск, которым получено расписание: те же сиды и
то же число раундов улучшения (ADR-0023). 201 и новое расписание; 400 — у расписания нет
записанного запуска. Параметры запуска — в поле `run` любого расписания:

```json
"run": { "construction": "dsatur", "seed": 0, "improve_seed": 1790254518834369665,
         "rounds": 131, "exact": true }
```

`GET /schedules/{id}/violations` — из чего складывается score: пересчитанный score (сумма
чётной и нечётной недели), разбивка по категориям и каждое нарушение от дорогих к дешёвым.

```json
{ "score": 24000, "breakdown": { "SingleClassDay": 24000, "...": 0 },
  "violations": [ { "category": "SingleClassDay", "rule": "День с одной парой",
    "group_ids": ["G1"], "week": "even", "day": "wednesday", "detail": "только 4-я пара",
    "penalty": 12000 } ] }
```

`GET /schedules/{id}/assignments/{idx}/options` — куда можно перенести пару с её чётностью:
36 записей, по одной на слот сетки.

```json
{ "time_slot": {"day": "monday", "pair_num": 3}, "room_id": "R102", "current": false,
  "conflict": null, "score_delta": -4000, "gaps_delta": 0, "long_gaps_delta": 0,
  "single_days_delta": -1, "saturday_delta": 0 }
```

`conflict` — как в ответе 409; `null` — перенос возможен в аудиторию `room_id` (своя, если свободна,
иначе другая подходящая по типу, вместимости и корпусам). Изменения — сумма по чётной и нечётной
неделе, отрицательные — лучше.

## Закрепление и перегенерация

`PUT /schedules/{id}/assignments/{idx}/pinned` — `{"pinned": true}` закрепляет пару, `false` снимает
закрепление; 200 — расписание целиком.

`POST /schedules/generate` с `"base_schedule_id": N` — перегенерация: закреплённые пары расписания N
остаются на местах и в своих аудиториях, остальные пары строятся заново вокруг них. Результат —
новое расписание, исходное не меняется.

## Справочники
`GET`, `POST`, `PUT /{id}`, `DELETE /{id}` для `/buildings`, `/departments`, `/rooms`, `/groups`,
`/teachers`, `/subject-plans`. `POST /import/excel` — импорт справочников из xlsx.

Пары преподавателя на других факультетах — поле `external_pairs` в теле `/teachers`:

```json
"external_pairs": [
  {"time_slot": {"day": "monday", "pair_num": 2}, "parity": "odd", "note": "ФИТ, 305"}
]
```

`parity`: `always` | `even` | `odd`. Время этих пар не меняется; алгоритм и ручной перенос не
ставят пары преподавателя на это время в те же недели (HC7, ADR-0017).
