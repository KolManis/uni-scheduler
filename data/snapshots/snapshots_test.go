package snapshots

import "testing"

// Встроенные справочники читаются целиком — столько записей, сколько в данных кафедры.
func TestLoad_Embedded(t *testing.T) {
	in, err := Load(FS)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "корпуса", got: len(in.Buildings), want: 2},
		{name: "кафедры", got: len(in.Departments), want: 8},
		{name: "аудитории", got: len(in.Rooms), want: 31},
		{name: "группы", got: len(in.Groups), want: 39},
		{name: "преподаватели", got: len(in.Teachers), want: 51},
		{name: "учебные планы", got: len(in.SubjectPlans), want: 320},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("встроено %d, ожидалось %d", tt.got, tt.want)
			}
		})
	}
}
