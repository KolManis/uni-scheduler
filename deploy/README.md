# uni-scheduler — установка на сервер

Автоматическое составление расписания. Веб-интерфейс и REST API, данные в PostgreSQL.
Всё работает в Docker. Два способа установки:

- **без интернета на сервере** — из пакета `uni-scheduler.tar.gz` (`make package`): образы, база со
  справочниками; разделы ниже до «Установки с интернетом»;
- **с интернетом на сервере** — образ собирается на сервере из репозитория, см.
  [Установка с интернетом](#установка-с-интернетом-сборка-на-сервере).

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
| `initdb/seed.sql` | База на первый запуск: схема и справочники — корпуса, аудитории, группы, преподаватели, учебные планы |
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

При первом запуске создаётся база и загружаются справочники из `initdb/seed.sql`.
При последующих запусках данные сохраняются.

## Управление

```bash
docker compose ps          # состояние
docker compose logs -f app # логи приложения
docker compose down        # остановить (данные сохраняются)
docker compose up -d       # запустить снова
```

**Внимание:** `docker compose down -v` удаляет том с базой — все созданные расписания
будут потеряны. При следующем запуске база создастся заново из `initdb/seed.sql`.

## Обновление без потери данных

Схему базы обновляет само приложение при запуске: миграции встроены в образ, и недостающие
применяются автоматически, расписания сохраняются. Для обновления достаточно загрузить новые
образы и перезапустить:

```bash
docker load -i images.tar
docker compose up -d
docker compose logs app | grep migrations   # «migrations applied from=… to=…»
```

База, созданная прежней версией (до перехода на goose), при первом запуске новой версии
принимается как есть: в журнале появится «existing schema adopted as version 1».

## Установка с интернетом (сборка на сервере)

Нужны Docker с `docker compose` и `git`; пакет не нужен. Образ собирается на сервере: первая
сборка скачивает Go и зависимости (~300 МБ, несколько минут).

```bash
git clone https://github.com/KolManis/uni-scheduler.git
cd uni-scheduler
docker build -t uni-scheduler-app:latest .
cd deploy
cp .env.example .env
# отредактировать .env: APP_PORT и DB_PASSWORD
docker compose up -d --wait
docker compose exec app /scheduler seed   # справочники кафедры вместо демо-данных
```

При первом запуске приложение создаёт пустую схему с демо-справочниками (миграция 0001).
`scheduler seed` заменяет их данными кафедры, встроенными в образ (`data/snapshots`). Если в
базе уже есть расписания, команда откажется: их пары ссылаются на справочники и перестали бы
открываться (`-force` заменит всё равно).

Интерфейс: `http://<адрес-сервера>:<APP_PORT>/ui/schedules`.

**Обновление** — расписания сохраняются, новые миграции приложение применит само:

```bash
cd uni-scheduler
git pull
docker build -t uni-scheduler-app:latest .
cd deploy
docker compose up -d --wait
```

Файл `docker-compose.yml` из корня репозитория на сервере не использовать: это настройка для
разработки, в ней база открыта наружу на порту 5432 с паролем `postgres`.

## Проверка установки на своём компьютере

Любой способ можно проверить локально, не задев рабочую установку: в `.env` задать другой
порт и **отдельное имя проекта** — иначе Docker примет папку за тот же проект и пересоздаст
рабочие контейнеры:

```
APP_PORT=18094
COMPOSE_PROJECT_NAME=uscheck
```

Удалить проверочную установку вместе с её базой — `docker compose down -v` в той же папке.
В Git Bash на Windows путь `/scheduler` в `docker compose exec` подменяется путём Windows —
запускать с `MSYS_NO_PATHCONV=1` или из PowerShell.

## Порт занят

Поменять `APP_PORT` в `.env` и выполнить `docker compose up -d`.
PostgreSQL наружу не публикуется и с портами на сервере не конфликтует.

## Запуск без Docker

Нужен свой PostgreSQL 16 и пустая база с загруженным `initdb/seed.sql` (`psql -d scheduler -f initdb/seed.sql`).
Схему дальше обновляет само приложение при запуске.

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
