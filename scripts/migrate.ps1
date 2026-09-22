# Аналог migrate.py для Windows/PowerShell (там, где нет Python).
# 1. Читает снапшоты из data/snapshots
# 2. Очищает справочные таблицы через scripts/truncate
# 3. Заново вставляет через REST API (POST -> сервер сам генерирует UUID)
# 4. Строит карты old_id -> new_uuid и сохраняет их в id_map.json

$ErrorActionPreference = "Stop"

$Base = "http://localhost:8080/api/v1"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Snap = Join-Path $ScriptDir "../data/snapshots"
$RepoRoot = Join-Path $ScriptDir ".."

function Load-Json($name) {
    Get-Content -Raw -Encoding UTF8 (Join-Path $Snap $name) | ConvertFrom-Json
}

function Post-Json($path, $body) {
    $json = $body | ConvertTo-Json -Depth 10 -Compress
    Invoke-RestMethod -Uri "$Base$path" -Method Post -ContentType "application/json; charset=utf-8" `
        -Body ([System.Text.Encoding]::UTF8.GetBytes($json))
}

Write-Output "Loading data from snapshots..."
$buildings     = Load-Json "buildings.json"
$departments   = Load-Json "departments.json"
$rooms         = Load-Json "rooms.json"
$teachers      = Load-Json "teachers.json"
$groups        = Load-Json "groups.json"
$subjectPlans  = Load-Json "subject_plans.json"

Write-Output ("  buildings={0} departments={1} rooms={2} teachers={3} groups={4} subject_plans={5}" -f `
    $buildings.Count, $departments.Count, $rooms.Count, $teachers.Count, $groups.Count, $subjectPlans.Count)

Write-Output "`nTruncating tables..."
Push-Location (Join-Path $ScriptDir "truncate")
try {
    $out = & go run . 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Output "truncate error: $out"
        exit 1
    }
    Write-Output "  Done: $out"
} finally {
    Pop-Location
}

# ── buildings: old_id -> new_uuid ─────────────────────────────────────────────
Write-Output "`nInserting buildings..."
$bldMap = @{}
foreach ($b in $buildings) {
    $resp = Post-Json "/buildings" @{ name = $b.name; address = $b.address }
    $bldMap[$b.id] = $resp.id
    Write-Output ("  {0} -> {1}  ({2})" -f $b.id, $resp.id, $b.name)
}

# ── departments: old_id -> new_uuid (дедуп по имени) ──────────────────────────
Write-Output "`nInserting departments..."
$deptMap = @{}
$seenDepts = @{}
foreach ($d in $departments) {
    if ($seenDepts.ContainsKey($d.name)) {
        $deptMap[$d.id] = $seenDepts[$d.name]
        Write-Output ("  {0} -> {1}  (duplicate, reusing)" -f $d.id, $seenDepts[$d.name])
        continue
    }
    $resp = Post-Json "/departments" @{ name = $d.name }
    $deptMap[$d.id] = $resp.id
    $seenDepts[$d.name] = $resp.id
    Write-Output ("  {0} -> {1}  ({2})" -f $d.id, $resp.id, $d.name)
}

# ── rooms: old_id -> new_uuid ──────────────────────────────────────────────────
Write-Output "`nInserting rooms..."
$roomMap = @{}
foreach ($rm in $rooms) {
    $body = @{
        number      = $rm.number
        building_id = $bldMap[$rm.building_id]
        capacity    = $rm.capacity
        type        = $rm.type
    }
    $resp = Post-Json "/rooms" $body
    $roomMap[$rm.id] = $resp.id
}
Write-Output ("  Inserted {0} rooms" -f $roomMap.Count)

# ── groups: old_id -> new_uuid ─────────────────────────────────────────────────
Write-Output "`nInserting groups..."
$groupMap = @{}
foreach ($g in $groups) {
    $bids = @($g.building_ids | ForEach-Object { $bldMap[$_] })
    $body = @{
        name          = $g.name
        student_count = $g.student_count
        building_ids  = $bids
    }
    $resp = Post-Json "/groups" $body
    $groupMap[$g.id] = $resp.id
}
Write-Output ("  Inserted {0} groups" -f $groupMap.Count)

# ── teachers: old_id -> new_uuid ───────────────────────────────────────────────
Write-Output "`nInserting teachers..."
$teacherMap = @{}
foreach ($t in $teachers) {
    $unavail = @()
    if ($t.PSObject.Properties.Name -contains "unavailable_slots" -and $t.unavailable_slots) {
        $unavail = @($t.unavailable_slots)
    }
    $prefBld = @()
    if ($t.PSObject.Properties.Name -contains "preferred_buildings" -and $t.preferred_buildings) {
        $prefBld = @($t.preferred_buildings | ForEach-Object { $bldMap[$_] })
    }
    $body = @{
        name                = $t.name
        department_id       = $deptMap[$t.department_id]
        max_weekly_hours    = $t.max_weekly_hours
        unavailable_slots   = $unavail
        preferred_buildings = $prefBld
    }
    $resp = Post-Json "/teachers" $body
    $teacherMap[$t.id] = $resp.id
}
Write-Output ("  Inserted {0} teachers" -f $teacherMap.Count)

# ── subject_plans ──────────────────────────────────────────────────────────────
Write-Output "`nInserting subject_plans..."
$ok = 0
$failed = @()
foreach ($sp in $subjectPlans) {
    try {
        $reqBld = ""
        if ($sp.required_building_id) { $reqBld = $bldMap[$sp.required_building_id] }
        $gids = @($sp.group_ids | ForEach-Object { $groupMap[$_] })
        $body = @{
            name                 = $sp.name
            department_id        = $deptMap[$sp.department_id]
            lecture_hours        = $sp.lecture_hours
            practice_hours       = $sp.practice_hours
            lab_hours            = $sp.lab_hours
            requires_room_type   = $sp.requires_room_type
            required_building_id = $reqBld
            teacher_id           = $teacherMap[$sp.teacher_id]
            group_ids            = $gids
            parity               = $sp.parity
            semester_half        = $sp.semester_half
        }
        Post-Json "/subject-plans" $body | Out-Null
        $ok++
    } catch {
        $failed += , @($sp.id, $_.Exception.Message)
    }
}
Write-Output ("  Inserted {0}/{1} subject_plans" -f $ok, $subjectPlans.Count)
if ($failed.Count -gt 0) {
    Write-Output ("  FAILED ({0}):" -f $failed.Count)
    $failed | Select-Object -First 5 | ForEach-Object { Write-Output ("    {0}: {1}" -f $_[0], $_[1]) }
}

# ── сохраняем карты id ──────────────────────────────────────────────────────────
$maps = @{
    buildings   = $bldMap
    departments = $deptMap
    rooms       = $roomMap
    groups      = $groupMap
    teachers    = $teacherMap
}
$maps | ConvertTo-Json -Depth 10 | Set-Content -Encoding UTF8 (Join-Path $RepoRoot "id_map.json")

Write-Output "`nDone! ID mapping saved to id_map.json"
Write-Output ("Verify: GET /api/v1/teachers -> should return {0} teachers" -f $teachers.Count)
