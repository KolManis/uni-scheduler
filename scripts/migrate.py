"""
Migrate reference data:
  1. Load current data from API
  2. Truncate all reference tables via psql
  3. Re-insert via new API endpoints (POST -> auto-UUID)
  4. Build old_id -> new_uuid maps for cross-references
"""

import json, subprocess, urllib.request, urllib.error, sys

BASE = "http://localhost:8080/api/v1"
DSN  = "postgres://postgres:postgres@127.0.0.1:5432/scheduler?sslmode=disable"

def get(path):
    with urllib.request.urlopen(BASE + path) as r:
        return json.loads(r.read())

def post(path, body):
    data = json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data,
          headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())

# ── 1. Load current data from saved snapshots ────────────────────────────────
print("Loading data from snapshots...")
def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)

import os
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
SNAP = os.path.join(SCRIPT_DIR, "..", "data", "snapshots")

buildings     = load(os.path.join(SNAP, "buildings.json"))
departments   = load(os.path.join(SNAP, "departments.json"))
rooms         = load(os.path.join(SNAP, "rooms.json"))
teachers      = load(os.path.join(SNAP, "teachers.json"))
groups        = load(os.path.join(SNAP, "groups.json"))
subject_plans = load(os.path.join(SNAP, "subject_plans.json"))

print(f"  buildings={len(buildings)} departments={len(departments)} "
      f"rooms={len(rooms)} teachers={len(teachers)} "
      f"groups={len(groups)} subject_plans={len(subject_plans)}")

# ── 2. Truncate via Go helper ─────────────────────────────────────────────────
print("\nTruncating tables...")
TRUNC_DIR = os.path.join(SCRIPT_DIR, "truncate")
result = subprocess.run(["go", "run", "."], capture_output=True, text=True, cwd=TRUNC_DIR)
if result.returncode != 0:
    print("truncate error:", result.stderr)
    sys.exit(1)
print("  Done:", result.stdout.strip())

# ── 3. Re-insert ──────────────────────────────────────────────────────────────

# buildings: old_id -> new_uuid
print("\nInserting buildings...")
bld_map = {}  # old_id -> new_uuid
for b in buildings:
    old_id = b["id"]
    resp = post("/buildings", {"name": b["name"], "address": b["address"]})
    bld_map[old_id] = resp["id"]
    print(f"  {old_id} -> {resp['id']}  ({b['name']})")

# departments: old_id -> new_uuid
print("\nInserting departments...")
dept_map = {}
seen_depts = {}
for d in departments:
    if d["name"] in seen_depts:
        # duplicate department (ITAS / itas) — map both old IDs to same new UUID
        dept_map[d["id"]] = seen_depts[d["name"]]
        print(f"  {d['id']} -> {seen_depts[d['name']]}  (duplicate, reusing)")
        continue
    resp = post("/departments", {"name": d["name"]})
    dept_map[d["id"]] = resp["id"]
    seen_depts[d["name"]] = resp["id"]
    print(f"  {d['id']} -> {resp['id']}  ({d['name']})")

# rooms: old_id -> new_uuid
print("\nInserting rooms...")
room_map = {}
for rm in rooms:
    old_id = rm["id"]
    body = {
        "number":      rm["number"],
        "building_id": bld_map[rm["building_id"]],
        "capacity":    rm["capacity"],
        "type":        rm["type"],
    }
    resp = post("/rooms", body)
    room_map[old_id] = resp["id"]
print(f"  Inserted {len(room_map)} rooms")

# groups: old_id -> new_uuid
print("\nInserting groups...")
group_map = {}
for g in groups:
    old_id = g["id"]
    body = {
        "name":          g["name"],
        "student_count": g["student_count"],
        "building_ids":  [bld_map[bid] for bid in g["building_ids"]],
    }
    resp = post("/groups", body)
    group_map[old_id] = resp["id"]
print(f"  Inserted {len(group_map)} groups")

# teachers: old_id -> new_uuid
print("\nInserting teachers...")
teacher_map = {}
for t in teachers:
    old_id = t["id"]
    body = {
        "name":                t["name"],
        "department_id":       dept_map[t["department_id"]],
        "max_weekly_hours":    t["max_weekly_hours"],
        "unavailable_slots":   t.get("unavailable_slots") or [],
        "preferred_buildings": [bld_map[b] for b in (t.get("preferred_buildings") or [])],
    }
    resp = post("/teachers", body)
    teacher_map[old_id] = resp["id"]
print(f"  Inserted {len(teacher_map)} teachers")

# subject_plans
print("\nInserting subject_plans...")
ok, failed = 0, []
for sp in subject_plans:
    try:
        body = {
            "name":                 sp["name"],
            "department_id":        dept_map[sp["department_id"]],
            "lecture_hours":        sp["lecture_hours"],
            "practice_hours":       sp["practice_hours"],
            "lab_hours":            sp["lab_hours"],
            "requires_room_type":   sp["requires_room_type"],
            "required_building_id": bld_map[sp["required_building_id"]] if sp.get("required_building_id") else "",
            "teacher_id":           teacher_map[sp["teacher_id"]],
            "group_ids":            [group_map[gid] for gid in sp["group_ids"]],
            "parity":               sp["parity"],
            "semester_half":        sp["semester_half"],
        }
        post("/subject-plans", body)
        ok += 1
    except Exception as e:
        failed.append((sp["id"], str(e)))

print(f"  Inserted {ok}/{len(subject_plans)} subject_plans")
if failed:
    print(f"  FAILED ({len(failed)}):")
    for sid, err in failed[:5]:
        print(f"    {sid}: {err}")

# ── 4. Save ID maps for reference ─────────────────────────────────────────────
maps = {
    "buildings":   bld_map,
    "departments": dept_map,
    "rooms":       room_map,
    "groups":      group_map,
    "teachers":    teacher_map,
}
with open("id_map.json", "w", encoding="utf-8") as f:
    json.dump(maps, f, ensure_ascii=False, indent=2)

print("\nDone! ID mapping saved to id_map.json")
print("Verify: GET /api/v1/teachers  -> should return", len(teachers), "teachers")
