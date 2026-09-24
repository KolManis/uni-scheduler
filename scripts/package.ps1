# Сборка пакета для установки на сервер: dist/uni-scheduler.tar.gz
#
#   powershell -ExecutionPolicy Bypass -File scripts/package.ps1
#
# Нужны: запущенный Docker, работающий контейнер postgres с данными (make up),
# доступ в интернет для сборки образа (базовые образы golang/alpine).
# Внутри архива: образы, docker-compose.yml, .env.example, миграции,
# дамп справочников (без расписаний), Linux-бинарник, README.

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$out = Join-Path $root 'dist\uni-scheduler'
$dbContainer = 'uni-scheduler-postgres-1'
$version = (git rev-parse --short HEAD).Trim()

function Step($text) { Write-Host "==> $text" -ForegroundColor Cyan }
function Check($what) { if ($LASTEXITCODE -ne 0) { throw "$what завершилось с кодом $LASTEXITCODE" } }

if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force (Join-Path $out 'migrations') | Out-Null

Step "Сборка образа приложения (версия $version)"
docker compose build app; Check 'docker compose build'
docker tag uni-scheduler-app:latest "uni-scheduler-app:$version"; Check 'docker tag'

Step 'Сохранение образов в images.tar'
docker save -o (Join-Path $out 'images.tar') uni-scheduler-app:latest "uni-scheduler-app:$version" postgres:16-alpine
Check 'docker save'

Step 'Сборка бинарников (Linux и Windows, x86-64)'
# ОС сервера заранее неизвестна: бинарники нужны только для запуска без Docker.
$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
try {
    $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    $env:GOOS = 'linux'
    go build -o (Join-Path $out 'scheduler-linux-amd64') ./cmd; Check 'go build linux'
    $env:GOOS = 'windows'
    go build -o (Join-Path $out 'scheduler-windows-amd64.exe') ./cmd; Check 'go build windows'
} finally {
    $env:GOOS = $saved.GOOS; $env:GOARCH = $saved.GOARCH; $env:CGO_ENABLED = $saved.CGO_ENABLED
}

Step 'Дамп справочников (без расписаний)'
# pg_dump пишет в файл внутри контейнера, файл забирается docker cp: при перенаправлении
# вывода через PowerShell кириллица перекодировалась бы в кодировку консоли.
#
# В начале дампа — TRUNCATE: миграция 0001 сама вставляет демо-справочники, и без очистки
# на сервере они оказались бы рядом с настоящими (в рабочей базе их затирает make seed).
$tables = 'buildings', 'departments', 'groups', 'teachers', 'rooms', 'subject_plans'
$tableArgs = ($tables | ForEach-Object { "-t $_" }) -join ' '
$truncate = "TRUNCATE $($tables -join ', ') CASCADE;"
docker exec $dbContainer sh -c "{ echo '$truncate'; pg_dump -U postgres -d scheduler --data-only --no-owner --no-privileges $tableArgs; } > /tmp/seed.sql"
Check 'pg_dump'
docker cp "${dbContainer}:/tmp/seed.sql" (Join-Path $out 'seed.sql'); Check 'docker cp'

Step 'Конфигурация и миграции'
Copy-Item deploy\docker-compose.yml, deploy\.env.example, deploy\README.md $out
Copy-Item migrations\0001_init.up.sql, `
          migrations\0003_add_semester_half_and_required_building.up.sql, `
          migrations\0004_add_unplaced_to_schedules.up.sql, `
          migrations\0005_add_schedule_options.up.sql, `
          migrations\0006_add_teacher_external_pairs.up.sql, `
          migrations\0007_add_teacher_undesired_slots.up.sql, `
          migrations\0008_add_schedule_run.up.sql (Join-Path $out 'migrations')
Set-Content -Path (Join-Path $out 'VERSION') -Value $version -Encoding ascii

Step 'Архив dist/uni-scheduler.tar.gz'
# tar из Windows 10+ пишет пути с прямыми слешами — архив корректно распакуется на Linux.
$archive = Join-Path $root 'dist\uni-scheduler.tar.gz'
if (Test-Path $archive) { Remove-Item $archive }
tar -czf $archive -C (Join-Path $root 'dist') uni-scheduler; Check 'tar'

$sizeMb = [math]::Round((Get-Item $archive).Length / 1MB, 1)
Write-Host "Готово: $archive ($sizeMb МБ), версия $version" -ForegroundColor Green
