package postgres

import (
	"strings"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name        string
		raw         []byte
		wantSlots   []domain.TimeSlot
		wantErr     bool
		wantErrText string
	}{
		{
			// Расписания до миграции 0004 хранят unplaced = NULL: это не ошибка,
			// иначе старые расписания перестанут открываться.
			name:      "NULL в базе оставляет поле пустым",
			raw:       nil,
			wantSlots: nil,
		},
		{
			name: "корректный JSON разбирается",
			raw:  []byte(`[{"day":"monday","pair_num":2}]`),
			wantSlots: []domain.TimeSlot{
				domain.MustNewTimeSlot(domain.Monday, 2),
			},
		},
		{
			name:      "JSON-литерал null не ошибка",
			raw:       []byte(`null`),
			wantSlots: nil,
		},
		{
			// Раньше ошибка json.Unmarshal отбрасывалась, и запись молча загружалась
			// с пустым полем — например, у преподавателя пропадали недоступные слоты.
			name:        "повреждённый JSON — ошибка с указанием поля",
			raw:         []byte(`[{"day":"monday"`),
			wantErr:     true,
			wantErrText: "teacher 42 unavailable_slots",
		},
		{
			name:        "JSON не того типа — ошибка",
			raw:         []byte(`{"not":"an array"}`),
			wantErr:     true,
			wantErrText: "teacher 42 unavailable_slots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []domain.TimeSlot

			err := decodeJSON(tt.raw, &got, "teacher 42 unavailable_slots")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получено nil; разобрано: %v", got)
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("в ошибке нет контекста %q: %v", tt.wantErrText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if len(got) != len(tt.wantSlots) {
				t.Fatalf("получено %d слотов, ожидалось %d: %v", len(got), len(tt.wantSlots), got)
			}
			for i := range got {
				if got[i] != tt.wantSlots[i] {
					t.Errorf("слот %d: получено %v, ожидалось %v", i, got[i], tt.wantSlots[i])
				}
			}
		})
	}
}
