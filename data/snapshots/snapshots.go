// Package snapshots — справочники кафедры (корпуса, кафедры, группы, преподаватели,
// аудитории, учебные планы) в JSON. Встроены в бинарник: команда `scheduler seed`
// загружает их в базу без внешних файлов и без интернета.
package snapshots

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

// FS — встроенные файлы справочников.
//
//go:embed *.json
var FS embed.FS

// Load читает справочники из fsys: FS или каталог (os.DirFS). Файлы сохранены с BOM.
func Load(fsys fs.FS) (domain.InputData, error) {
	var in domain.InputData
	files := []struct {
		name string
		dst  any
	}{
		{"buildings.json", &in.Buildings},
		{"departments.json", &in.Departments},
		{"groups.json", &in.Groups},
		{"teachers.json", &in.Teachers},
		{"rooms.json", &in.Rooms},
		{"subject_plans.json", &in.SubjectPlans},
	}
	for _, f := range files {
		raw, err := fs.ReadFile(fsys, f.name)
		if err != nil {
			return domain.InputData{}, fmt.Errorf("read %s: %w", f.name, err)
		}
		raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return domain.InputData{}, fmt.Errorf("decode %s: %w", f.name, err)
		}
	}
	return in, nil
}
