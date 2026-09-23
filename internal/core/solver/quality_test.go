package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestCalculateQuality(t *testing.T) {
	tests := []struct {
		name        string
		assignments []domain.Assignment
		want        domain.QualityStats
	}{
		{
			name:        "пустое расписание — все показатели нулевые",
			assignments: nil,
			want:        domain.QualityStats{},
		},
		{
			name: "одна пара в день — одиночный день",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 2),
					Parity:    domain.Always,
				},
			},
			want: domain.QualityStats{
				SingleClassDays:      1,
				MaxGroupPairsPerDay:  1,
				MaxTeacherPairsInDay: 1,
			},
		},
		{
			name: "пары 1 и 4 у группы — два окна",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Tuesday, 1),
					Parity:    domain.Always,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T2",
					TimeSlot:  domain.MustNewTimeSlot(domain.Tuesday, 4),
					Parity:    domain.Always,
				},
			},
			want: domain.QualityStats{
				GroupGaps:            2,
				MaxGroupPairsPerDay:  2,
				MaxTeacherPairsInDay: 1,
			},
		},
		{
			name: "чётная и нечётная в одном слоте — одна пара, не две",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Saturday, 1),
					Parity:    domain.Even,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Saturday, 1),
					Parity:    domain.Odd,
				},
			},
			want: domain.QualityStats{
				SingleClassDays:      1,
				SaturdayPairs:        1,
				MaxGroupPairsPerDay:  1,
				MaxTeacherPairsInDay: 1,
			},
		},
		{
			name: "поток из двух групп считается у каждой группы",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1", "G2"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Friday, 3),
					Parity:    domain.Always,
				},
			},
			want: domain.QualityStats{
				SingleClassDays:      2,
				MaxGroupPairsPerDay:  1,
				MaxTeacherPairsInDay: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateQuality(tt.assignments)
			if got != tt.want {
				t.Errorf("получено %+v, ожидалось %+v", got, tt.want)
			}
		})
	}
}
