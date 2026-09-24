# uni-scheduler — команды для запуска и разработки.
#
#   make help    список команд
#   make fresh   всё с нуля
#
# Рецепты написаны так, чтобы работать и через sh (Git Bash, Linux), и через
# cmd.exe — GNU make на Windows берёт cmd.exe, если sh.exe нет в PATH (а он там
# есть только внутри Git Bash). Поэтому: без кавычек в echo, без POSIX-префиксов
# переменных окружения, переменные пробрасываются через export.
#
# Тот же набор команд для PowerShell:  .\make.ps1 <команда>

.DEFAULT_GOAL := help
.PHONY: help up demo down fresh seed dev db db-reset db-migrate logs ps test build clean package

DSN ?= postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable

# Заливка справочных данных: на Windows — PowerShell-версия, иначе — Python.
ifeq ($(OS),Windows_NT)
SEED_CMD = powershell -NoProfile -ExecutionPolicy Bypass -File scripts/migrate.ps1
else
SEED_CMD = python3 scripts/migrate.py
endif

# ── справка ───────────────────────────────────────────────────────────────────
# На Windows печатаем через make.ps1: cmd.exe выводит UTF-8 из Makefile в кодировке
# консоли и ломает кириллицу, а PowerShell-скрипт сам выставляет UTF-8.

ifeq ($(OS),Windows_NT)
help:
	@powershell -NoProfile -ExecutionPolicy Bypass -File make.ps1 help make
else
help:
	@echo ""
	@echo "uni-scheduler — доступные команды:"
	@echo ""
	@echo "  make fresh     Всё с нуля: пересоздать БД, поднять сервисы, залить данные"
	@echo "  make up        Поднять postgres + приложение в Docker и дождаться готовности"
	@echo "  make demo      Запуск без пересборки — работает без интернета (для защиты)"
	@echo "  make down      Остановить сервисы, данные в БД сохраняются"
	@echo "  make seed      Залить справочники из data/snapshots — ВНИМАНИЕ: TRUNCATE"
	@echo ""
	@echo "  make dev       Postgres в Docker, приложение локально через go run"
	@echo "  make db        Поднять только postgres"
	@echo "  make db-reset  Пересоздать БД с нуля, миграции применяются заново"
	@echo "  make db-migrate Накатить новые миграции на существующую БД, данные сохраняются"
	@echo ""
	@echo "  make test      go build + go vet + go test — Definition of Done"
	@echo "  make build     Собрать бинарник в bin/scheduler"
	@echo "  make logs      Логи приложения"
	@echo "  make ps        Статус контейнеров"
	@echo "  make clean     Остановить всё и удалить том с данными БД"
	@echo ""
	@echo "После make up или make fresh: http://localhost:8080/ui/schedules"
	@echo ""
endif

# ── запуск ────────────────────────────────────────────────────────────────────

up:
	docker compose up -d --build --wait
	@echo Ready: http://localhost:8080/ui/schedules
	@echo No reference data? Load it with: make seed

# demo — запуск БЕЗ пересборки образа. Нужен там, где нет интернета (защита, аудитория):
# сборка тянет базовые образы golang:1.25-alpine и alpine:3.21 из реестра, плюс go mod
# download и apk add — всё это требует сети. Готовый uni-scheduler-app уже лежит локально,
# поэтому старт из него сети не требует вообще. Код при этом не пересобирается.
demo:
	docker compose up -d --wait
	@echo Ready offline: http://localhost:8080/ui/schedules

fresh: clean up seed
	@echo Ready from scratch, real data loaded: http://localhost:8080/ui/schedules

down:
	docker compose down

clean:
	docker compose down -v

# ── база данных ───────────────────────────────────────────────────────────────

db:
	docker compose up -d --wait postgres

# Миграции подключены как docker-entrypoint-initdb.d и применяются только при
# создании тома. Поэтому «накатить» новую миграцию = пересоздать том.
db-reset:
	docker compose down -v
	docker compose up -d --wait postgres
	@echo DB recreated, migrations 0001/0003/0004/0005/0006 applied.

# Миграции 0003+ идемпотентны (IF NOT EXISTS) — их можно накатить на рабочую базу
# без пересоздания тома, данные и расписания сохраняются.
db-migrate: db
	for f in migrations/0003_*.sql migrations/0004_*.sql migrations/0005_*.sql migrations/0006_*.sql; do \
		docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d scheduler < $$f || exit 1; \
	done
	@echo Migrations 0003-0006 applied.

seed:
	$(SEED_CMD)

# ── разработка ────────────────────────────────────────────────────────────────

# export вместо POSIX-префикса VAR=value — работает и в cmd.exe, и в sh.
dev: export DATABASE_DSN = $(DSN)
dev: db
	go run ./cmd

build:
	go build -o bin/scheduler ./cmd

test:
	go build ./...
	go vet ./...
	go test ./...

# Пакет для установки на сервер: dist/uni-scheduler.tar.gz (образы, compose, дамп справочников).
package:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package.ps1

# ── диагностика ───────────────────────────────────────────────────────────────

logs:
	docker compose logs -f app

ps:
	docker compose ps
