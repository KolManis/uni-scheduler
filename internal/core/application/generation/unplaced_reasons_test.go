package generation

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestExplainUnplaced(t *testing.T) {
	data := domain.InputData{
		Groups:   []domain.Group{{ID: "G1", StudentCount: 20}},
		Teachers: []domain.Teacher{{ID: "T1"}},
		Rooms:    []domain.Room{{ID: "R1", Capacity: 10, Type: "lab"}},
		SubjectPlans: []domain.SubjectPlan{
			{ID: "P1", TeacherID: "T1", GroupIDs: []string{"G1"}, LabHours: 2, RequiresRoomType: "lab"},
		},
	}
	items := explainUnplaced([]domain.UnplacedItem{{SubjectID: "P1", Type: domain.Lab, MissingHours: 2}}, nil, data)
	if want := "нет аудитории типа «lab» на 20 мест в допустимых корпусах"; items[0].Reason != want {
		t.Errorf("причина: %q, ожидалось %q", items[0].Reason, want)
	}
}
