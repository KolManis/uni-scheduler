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
  "parallel_starts": 1,
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
| `parallel_starts` | 1 | N > 1 — многостарт, берётся лучший |
| `semester_half` | `""` | `second` исключает планы только первой половины |
| `solver_type` | `""` | построение: `""` или `teacher` — по преподавателям, `dsatur` — самая трудная пара первой (ADR-0015); иначе 400 |

Ответы: **201** — расписание целиком; **400** — неверные параметры; **500** — ошибка генерации
или таймаут.

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
| 409 | конфликт: `{"type": "teacher_busy \| group_busy \| room_busy \| teacher_unavailable", "resource_id": "…", "conflict_with": N}`; у `teacher_unavailable` `conflict_with` = −1 |
| 404 | нет расписания |

## Справочники
`GET`, `POST`, `PUT /{id}`, `DELETE /{id}` для `/buildings`, `/departments`, `/rooms`, `/groups`,
`/teachers`, `/subject-plans`. `POST /import/excel` — импорт справочников из xlsx.
