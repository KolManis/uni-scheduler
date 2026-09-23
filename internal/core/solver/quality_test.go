package solver

import (
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestCalculateQuality(t *testing.T) {
	both := func(s domain.QualityStats) domain.WeekQuality {
		return domain.WeekQuality{Even: s, Odd: s}
	}

	tests := []struct {
		name        string
		assignments []domain.Assignment
		want        domain.WeekQuality
	}{
		{
			name:        "пустое расписание — все показатели нулевые",
			assignments: nil,
			want:        domain.WeekQuality{},
		},
		{
			name: "одна пара в день — одиночный день в обе недели",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 2),
					Parity:    domain.Always,
				},
			},
			want: both(domain.QualityStats{
				SingleClassDays:      1,
				MaxGroupPairsPerDay:  1,
				MaxTeacherPairsInDay: 1,
			}),
		},
		{
			name: "пары 1 и 4 у группы — два окна в обе недели",
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
			want: both(domain.QualityStats{
				GroupGaps:            2,
				MaxGroupPairsPerDay:  2,
				MaxTeacherPairsInDay: 1,
			}),
		},
		{
			name: "пара только по чётным: в нечётную неделю у группы остаётся одна пара",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 1),
					Parity:    domain.Always,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T2",
					TimeSlot:  domain.MustNewTimeSlot(domain.Monday, 2),
					Parity:    domain.Even,
				},
			},
			want: domain.WeekQuality{
				Even: domain.QualityStats{
					MaxGroupPairsPerDay:  2,
					MaxTeacherPairsInDay: 1,
				},
				Odd: domain.QualityStats{
					SingleClassDays:      1,
					MaxGroupPairsPerDay:  1,
					MaxTeacherPairsInDay: 1,
				},
			},
		},
		{
			name: "«мигалка»: чётная и нечётная пары в одном слоте закрывают его в обе недели",
			assignments: []domain.Assignment{
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T1",
					TimeSlot:  domain.MustNewTimeSlot(domain.Wednesday, 1),
					Parity:    domain.Always,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T2",
					TimeSlot:  domain.MustNewTimeSlot(domain.Wednesday, 2),
					Parity:    domain.Even,
				},
				{
					GroupIDs:  []string{"G1"},
					TeacherID: "T3",
					TimeSlot:  domain.MustNewTimeSlot(domain.Wednesday, 2),
					Parity:    domain.Odd,
				},
			},
			want: both(domain.QualityStats{
				MaxGroupPairsPerDay:  2,
				MaxTeacherPairsInDay: 1,
			}),
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
			want: both(domain.QualityStats{
				SingleClassDays:      2,
				MaxGroupPairsPerDay:  1,
				MaxTeacherPairsInDay: 1,
			}),
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
