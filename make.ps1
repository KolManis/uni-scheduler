# PowerShell-аналог Makefile для Windows, где make не установлен.
# Использование:  .\make.ps1 <команда>     например:  .\make.ps1 fresh
# Список команд:  .\make.ps1 help

# $Prefix — как показывать команды в справке. Makefile передаёт сюда "make",
# чтобы подсказка соответствовала способу вызова.
param([string]$Target = "help", [string]$Prefix = ".\make.ps1")

$ErrorActionPreference = "Stop"
# Чтобы кириллица в выводе не ломалась в современных терминалах
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $RepoRoot

$DSN = "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable"

function Run($cmd, $argList) {
    Write-Host "> $cmd $($argList -join ' ')" -ForegroundColor DarkGray
    & $cmd @argList
    if ($LASTEXITCODE -ne 0) {
        throw "Команда завершилась с ошибкой (код $LASTEXITCODE): $cmd $($argList -join ' ')"
    }
}

function Show-Help {
    # Ширина колонки команд: "  <prefix> db-reset" + отступ до описания
    $width = $Prefix.Length + 16

    function Row($cmd, $desc) {
        $left = "  {0} {1}" -f $Prefix, $cmd
        Write-Host $left.PadRight($width) -NoNewline -ForegroundColor Green
        Write-Host $desc
    }

    Write-Host ""
    Write-Host "uni-scheduler - доступные команды:" -ForegroundColor Cyan
    Write-Host ""
    Row "fresh"    "Всё с нуля: пересоздать БД, поднять сервисы, залить данные"
    Row "up"       "Поднять postgres + приложение в Docker и дождаться готовности"
    Row "demo"     "Запуск без пересборки - работает без интернета (для защиты)"
    Row "down"     "Остановить сервисы, данные в БД сохраняются"
    Row "seed"     "Залить справочники из data/snapshots - ВНИМАНИЕ: TRUNCATE"
    Write-Host ""
    Row "dev"      "Postgres в Docker, приложение локально через go run"
    Row "db"       "Поднять только postgres"
    Row "db-reset" "Пересоздать БД с нуля, миграции применяются заново"
    Row "db-migrate" "Накатить новые миграции на существующую БД, данные сохраняются"
    Write-Host ""
    Row "test"     "go build + go vet + go test - Definition of Done"
    Row "build"    "Собрать бинарник в bin\scheduler.exe"
    Row "logs"     "Логи приложения"
    Row "ps"       "Статус контейнеров"
    Row "clean"    "Остановить всё и удалить том с данными БД"
    Write-Host ""
    Write-Host "После up или fresh: http://localhost:8080/ui/schedules" -ForegroundColor Yellow
    Write-Host ""
}

function Invoke-Up {
    Run "docker" @("compose", "up", "-d", "--build", "--wait")
    Write-Host ""
    Write-Host "Готово. Веб-интерфейс: http://localhost:8080/ui/schedules" -ForegroundColor Green
    Write-Host "Если справочники пустые - залейте данные: .\make.ps1 seed"
}

function Invoke-Seed {
    Run "powershell" @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts\migrate.ps1")
}

switch ($Target.ToLower()) {
    "help"     { Show-Help }

    "up"       { Invoke-Up }

    # Запуск без пересборки образа — работает без интернета.
    # Сборка тянет базовые образы golang/alpine из реестра, плюс go mod download
    # и apk add; готовый uni-scheduler-app лежит локально и сети не требует.
    "demo"     {
        Run "docker" @("compose", "up", "-d", "--wait")
        Write-Host ""
        Write-Host "Готово (без интернета): http://localhost:8080/ui/schedules" -ForegroundColor Green
    }

    "fresh"    {
        Run "docker" @("compose", "down", "-v")
        Invoke-Up
        Invoke-Seed
        Write-Host ""
        Write-Host "Система поднята с нуля и заполнена реальными данными." -ForegroundColor Green
        Write-Host "Веб-интерфейс: http://localhost:8080/ui/schedules" -ForegroundColor Green
    }

    "down"     { Run "docker" @("compose", "down") }

    "clean"    { Run "docker" @("compose", "down", "-v") }

    "db"       { Run "docker" @("compose", "up", "-d", "--wait", "postgres") }

    # Миграции 0003/0004 подключены как docker-entrypoint-initdb.d и применяются
    # только при создании тома. Поэтому "накатить" их = пересоздать том.
    "db-reset" {
        Run "docker" @("compose", "down", "-v")
        Run "docker" @("compose", "up", "-d", "--wait", "postgres")
        Write-Host "БД пересоздана, миграции 0001/0003-0008 применены." -ForegroundColor Green
    }

    # Миграции 0003+ идемпотентны (IF NOT EXISTS): накатываются на рабочую базу без потери данных.
    "db-migrate" {
        Run "docker" @("compose", "up", "-d", "--wait", "postgres")
        foreach ($f in Get-ChildItem migrations -Filter "000[3-8]_*.sql" | Sort-Object Name) {
            Get-Content $f.FullName -Raw -Encoding UTF8 | docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d scheduler
            if ($LASTEXITCODE -ne 0) { throw "миграция $($f.Name) не применилась" }
        }
        Write-Host "Миграции 0003-0008 применены." -ForegroundColor Green
    }

    "seed"     { Invoke-Seed }

    "dev"      {
        Run "docker" @("compose", "up", "-d", "--wait", "postgres")
        $env:DATABASE_DSN = $DSN
        Run "go" @("run", "./cmd")
    }

    "build"    { Run "go" @("build", "-o", "bin\scheduler.exe", "./cmd") }

    "test"     {
        Run "go" @("build", "./...")
        Run "go" @("vet", "./...")
        Run "go" @("test", "./...")
    }

    "logs"     { Run "docker" @("compose", "logs", "-f", "app") }

    "ps"       { Run "docker" @("compose", "ps") }

    default    {
        Write-Host "Неизвестная команда: $Target" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
