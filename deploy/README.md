# uni-scheduler — установка на сервер

Автоматическое составление расписания. Веб-интерфейс и REST API, данные в PostgreSQL.
Всё работает в Docker, доступ в интернет на сервере не нужен.

## Что нужно на сервере

- Docker 20.10+ с плагином `docker compose`
- Один свободный TCP-порт (по умолчанию 18080)
- ~500 МБ на диске

## Состав пакета

| Файл | Назначение |
|---|---|
| `images.tar` | Образы приложения и PostgreSQL |
| `docker-compose.yml` | Описание сервисов |
| `.env.example` | Настройки: порт, пароль БД |
| `migrations/` | Схема базы данных |
| `seed.sql` | Справочники: корпуса, аудитории, группы, преподаватели, учебные планы |
| `scheduler-linux-amd64`, `scheduler-windows-amd64.exe` | Бинарники приложения — для запуска без Docker (см. ниже) |

Образы собраны под x86-64 (amd64): работают на Linux-сервере с Docker, а также на
Windows/macOS с Docker Desktop. На ARM-сервере (aarch64) образ не запустится —
нужна отдельная сборка.

## Запуск

```bash
docker load -i images.tar
cp .env.example .env
# отредактировать .env: APP_PORT и DB_PASSWORD
docker compose up -d --wait
```

Интерфейс: `http://<адрес-сервера>:<APP_PORT>/ui/schedules`

При первом запуске создаётся база и загружаются справочники из `seed.sql`.
При последующих запусках данные сохраняются.

## Управление

```bash
docker compose ps          # состояние
docker compose logs -f app # логи приложения
docker compose down        # остановить (данные сохраняются)
docker compose up -d       # запустить снова
```

**Внимание:** `docker compose down -v` удаляет том с базой — все созданные расписания
будут потеряны. При следующем запуске база создастся заново из `seed.sql`.

## Порт занят

Поменять `APP_PORT` в `.env` и выполнить `docker compose up -d`.
PostgreSQL наружу не публикуется и с портами на сервере не конфликтует.

## Запуск без Docker

Нужен свой PostgreSQL 16 с применёнными `migrations/*.sql` и `seed.sql`.

```bash
chmod +x scheduler-linux-amd64   # архив собран на Windows, бит исполнения не сохраняется
DATABASE_DSN="postgres://user:pass@host:5432/scheduler?sslmode=disable" \
HTTP_ADDR=":18080" \
./scheduler-linux-amd64
```

На Windows (PowerShell):

```powershell
$env:DATABASE_DSN = "postgres://user:pass@host:5432/scheduler?sslmode=disable"
$env:HTTP_ADDR = ":18080"
.\scheduler-windows-amd64.exe
```
