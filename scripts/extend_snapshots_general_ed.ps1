# Разовый скрипт: добавляет в data/snapshots синтетические общеобразовательные дисциплины
# (высшая математика, философия, история, психология, иностранный язык, физкультура,
# БЖД, начертательная геометрия) на нескольких новых кафедрах — чтобы проверить сценарий
# с разными кафедрами, большими потоками (10 групп одновременно) и физкультурой в зале/на улице.
# Группы для потока — весь набор групп набора 2025 года (бакалавриат/специалитет).

$ErrorActionPreference = "Stop"
$Snap = Join-Path $PSScriptRoot "../data/snapshots"

function Load($name) { Get-Content -Raw -Encoding UTF8 (Join-Path $Snap $name) | ConvertFrom-Json }
function Save($name, $obj) {
    ($obj | ConvertTo-Json -Depth 10 -Compress) | Set-Content -Encoding UTF8 (Join-Path $Snap $name)
}

# ВАЖНО: @(Load "x.json") напрямую вокруг вызова функции не разворачивает массив (PowerShell
# заворачивает весь вывод функции в один элемент). Поэтому сначала кладём результат в
# переменную, и только затем оборачиваем её в @() — так корректно нормализуется и
# однострочный JSON-массив (который ConvertFrom-Json иначе вернул бы скаляром).
$buildingsRaw    = Load "buildings.json"
$departmentsRaw  = Load "departments.json"
$roomsRaw        = Load "rooms.json"
$teachersRaw     = Load "teachers.json"
$subjectPlansRaw = Load "subject_plans.json"

$buildings    = @($buildingsRaw)
$departments  = @($departmentsRaw)
$rooms        = @($roomsRaw)
$teachers     = @($teachersRaw)
$subjectPlans = @($subjectPlansRaw)

# ── Новое здание: стадион / открытая площадка ─────────────────────────────────
$buildings += [ordered]@{ id = "B"; name = "Стадион (открытая площадка)"; address = "ул. Спортивная" }

# ── Новые аудитории: спортзал (в главном корпусе) и стадион ───────────────────
$rooms += [ordered]@{ id = "R-GYM";      number = "Спортзал"; building_id = "A"; capacity = 60;  type = "gym" }
$rooms += [ordered]@{ id = "R-STADIUM";  number = "Стадион";  building_id = "B"; capacity = 200; type = "outdoor" }

# ── Новые кафедры ───────────────────────────────────────────────────────────────
$departments += [ordered]@{ id = "MATH"; name = "Кафедра высшей математики" }
$departments += [ordered]@{ id = "HUM";  name = "Кафедра гуманитарных и социальных дисциплин" }
$departments += [ordered]@{ id = "LANG"; name = "Кафедра иностранных языков" }
$departments += [ordered]@{ id = "PHYS"; name = "Кафедра физического воспитания" }
$departments += [ordered]@{ id = "BZD";  name = "Кафедра безопасности жизнедеятельности" }
$departments += [ordered]@{ id = "ENG";  name = "Кафедра инженерной графики" }

# ── Новые преподаватели ───────────────────────────────────────────────────────
$newTeachers = @(
    @{ id="TG-MATH1"; name="Доц. Смирнова И. П.";     department_id="MATH"; max_weekly_hours=20; preferred_buildings=@("A") }
    @{ id="TG-MATH2"; name="Проф. Кузнецов В. А.";    department_id="MATH"; max_weekly_hours=10; preferred_buildings=@("A") }
    @{ id="TG-MATH3"; name="Ст.пр. Волкова Н. С.";    department_id="MATH"; max_weekly_hours=20; preferred_buildings=@("A") }
    @{ id="TG-HUM1";  name="Доц. Соколова Е. В.";     department_id="HUM";  max_weekly_hours=8;  preferred_buildings=@("A") }
    @{ id="TG-HUM2";  name="Доц. Орлов Д. М.";        department_id="HUM";  max_weekly_hours=8;  preferred_buildings=@("A") }
    @{ id="TG-HUM3";  name="Ст.пр. Белякова Т. Н.";   department_id="HUM";  max_weekly_hours=8;  preferred_buildings=@("A") }
    @{ id="TG-LANG1"; name="Ст.пр. Иванова М. А.";    department_id="LANG"; max_weekly_hours=20; preferred_buildings=@("A") }
    @{ id="TG-LANG2"; name="Ст.пр. Петрова О. И.";    department_id="LANG"; max_weekly_hours=20; preferred_buildings=@("A") }
    @{ id="TG-LANG3"; name="Доц. Сидорова Л. К.";     department_id="LANG"; max_weekly_hours=20; preferred_buildings=@("A") }
    @{ id="TG-PHYS1"; name="Ст.пр. Козлов А. Н.";     department_id="PHYS"; max_weekly_hours=16; preferred_buildings=@("A","B") }
    @{ id="TG-PHYS2"; name="Ст.пр. Морозова Ю. В.";   department_id="PHYS"; max_weekly_hours=16; preferred_buildings=@("A","B") }
    @{ id="TG-BZD1";  name="Доц. Николаев П. С.";     department_id="BZD";  max_weekly_hours=8;  preferred_buildings=@("A") }
    @{ id="TG-ENG1";  name="Доц. Тимофеева А. Л.";    department_id="ENG";  max_weekly_hours=8;  preferred_buildings=@("A") }
    @{ id="TG-ENG2";  name="Ст.пр. Романов К. Д.";    department_id="ENG";  max_weekly_hours=16; preferred_buildings=@("A") }
)
$teachers += $newTeachers

# ── Группы 1 курса (набор 2025), общий поток общеобразовательных предметов ────
$stream = @("AT-25-1B","IPR-25-1B","IVT-25-1B","IVT-25-2B","KOB-25-1S","RIS-25-1B","RIS-25-2B","RIS-25-3B","TK-25-1B","UTS-25-1S")
$mathGroupA  = @("AT-25-1B","IPR-25-1B","IVT-25-1B","IVT-25-2B","KOB-25-1S")
$mathGroupB  = @("RIS-25-1B","RIS-25-2B","RIS-25-3B","TK-25-1B","UTS-25-1S")
$langGroupA  = @("AT-25-1B","IPR-25-1B","IVT-25-1B","IVT-25-2B")
$langGroupB  = @("KOB-25-1S","RIS-25-1B","RIS-25-2B")
$langGroupC  = @("RIS-25-3B","TK-25-1B","UTS-25-1S")
$psyGroup    = @("IVT-25-1B","IVT-25-2B","RIS-25-1B","RIS-25-2B")
$peGym       = @("KOB-25-1S","TK-25-1B","UTS-25-1S")
$peOutdoor   = @("AT-25-1B","IPR-25-1B","IVT-25-1B","IVT-25-2B","RIS-25-1B","RIS-25-2B","RIS-25-3B")
$graphSubset = @("AT-25-1B","IVT-25-1B","IVT-25-2B","RIS-25-1B","RIS-25-2B","RIS-25-3B")

function NewPlan($id, $name, $dept, $teacher, $groups, $lec, $prac, $lab, $roomType, $reqBld) {
    [ordered]@{
        id = $id; name = $name; department_id = $dept
        lecture_hours = $lec; practice_hours = $prac; lab_hours = $lab
        requires_room_type = $roomType
        required_building_id = $reqBld
        teacher_id = $teacher
        group_ids = @($groups)
        parity = "always"
        semester_half = "full"
    }
}

$newPlans = New-Object System.Collections.Generic.List[object]

$newPlans.Add((NewPlan "GEN-MATH-LEC" "Высшая математика (лекция)" "MATH" "TG-MATH2" $stream 2 0 0 "lecture" ""))
foreach ($g in $mathGroupA) { $newPlans.Add((NewPlan "GEN-MATH-PR-$g" "Высшая математика (практика)" "MATH" "TG-MATH1" @($g) 0 2 0 "lecture" "")) }
foreach ($g in $mathGroupB) { $newPlans.Add((NewPlan "GEN-MATH-PR-$g" "Высшая математика (практика)" "MATH" "TG-MATH3" @($g) 0 2 0 "lecture" "")) }

$newPlans.Add((NewPlan "GEN-PHIL-LEC" "Философия (лекция)" "HUM" "TG-HUM1" $stream 2 0 0 "lecture" ""))
$newPlans.Add((NewPlan "GEN-HIST-LEC" "История (лекция)" "HUM" "TG-HUM2" $stream 2 0 0 "lecture" ""))
$newPlans.Add((NewPlan "GEN-PSY-PR" "Психология" "HUM" "TG-HUM3" $psyGroup 0 2 0 "lecture" ""))

foreach ($g in $langGroupA) { $newPlans.Add((NewPlan "GEN-LANG-PR-$g" "Иностранный язык" "LANG" "TG-LANG1" @($g) 0 2 0 "" "")) }
foreach ($g in $langGroupB) { $newPlans.Add((NewPlan "GEN-LANG-PR-$g" "Иностранный язык" "LANG" "TG-LANG2" @($g) 0 2 0 "" "")) }
foreach ($g in $langGroupC) { $newPlans.Add((NewPlan "GEN-LANG-PR-$g" "Иностранный язык" "LANG" "TG-LANG3" @($g) 0 2 0 "" "")) }

$newPlans.Add((NewPlan "GEN-PE-GYM" "Физическая культура и спорт (зал)" "PHYS" "TG-PHYS1" $peGym 0 2 0 "gym" ""))
$newPlans.Add((NewPlan "GEN-PE-OUTDOOR" "Физическая культура и спорт (улица)" "PHYS" "TG-PHYS2" $peOutdoor 0 2 0 "outdoor" "B"))

$newPlans.Add((NewPlan "GEN-BZD-LEC" "Безопасность жизнедеятельности (лекция)" "BZD" "TG-BZD1" $stream 2 0 0 "lecture" ""))

$newPlans.Add((NewPlan "GEN-GRAPH-LEC" "Начертательная геометрия и инженерная графика (лекция)" "ENG" "TG-ENG1" $graphSubset 2 0 0 "lecture" ""))
foreach ($g in $graphSubset) { $newPlans.Add((NewPlan "GEN-GRAPH-PR-$g" "Начертательная геометрия и инженерная графика (практика)" "ENG" "TG-ENG2" @($g) 0 2 0 "lab" "")) }

$subjectPlans += $newPlans

Save "buildings.json" $buildings
Save "rooms.json" $rooms
Save "departments.json" $departments
Save "teachers.json" $teachers
Save "subject_plans.json" $subjectPlans

Write-Output ("Buildings: {0}, Rooms: {1}, Departments: {2}, Teachers: {3}, SubjectPlans: {4}" -f `
    $buildings.Count, $rooms.Count, $departments.Count, $teachers.Count, $subjectPlans.Count)
Write-Output ("Added: {0} new subject_plans" -f $newPlans.Count)
