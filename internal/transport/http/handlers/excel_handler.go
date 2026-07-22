package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/usecase/generator"
)

type excelScheduleService interface {
	GetByID(ctx context.Context, id int64) (*schedule.Schedule, error)
}

type ExcelHandler struct {
	usecase   excelScheduleService
	inputRepo generator.InputRepository
}

func NewExcelHandler(usecase excelScheduleService, inputRepo generator.InputRepository) *ExcelHandler {
	return &ExcelHandler{usecase: usecase, inputRepo: inputRepo}
}

func (h *ExcelHandler) Export(w http.ResponseWriter, r *http.Request) {
	scheduleID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || scheduleID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}

	viewType := r.URL.Query().Get("type")
	if viewType == "" {
		viewType = "group"
	}
	entityID := r.URL.Query().Get("id")
	weekParam := r.URL.Query().Get("week")
	if weekParam == "" {
		weekParam = "both"
	}

	sched, err := h.usecase.GetByID(r.Context(), scheduleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	input, err := h.inputRepo.LoadInput(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot load input data")
		return
	}

	groupMap := make(map[string]string)
	for _, g := range input.Groups {
		groupMap[g.ID] = g.Name
	}
	teacherMap := make(map[string]string)
	for _, t := range input.Teachers {
		teacherMap[t.ID] = t.Name
	}
	roomMap := make(map[string]string)
	for _, r := range input.Rooms {
		roomMap[r.ID] = r.Number
	}
	subjectMap := make(map[string]string)
	for _, s := range input.SubjectPlans {
		subjectMap[s.ID] = s.Name
	}

	days := []schedule.Day{
		schedule.Monday, schedule.Tuesday, schedule.Wednesday,
		schedule.Thursday, schedule.Friday, schedule.Saturday,
	}
	dayNames := map[schedule.Day]string{
		schedule.Monday:    "понедельник",
		schedule.Tuesday:   "вторник",
		schedule.Wednesday: "среда",
		schedule.Thursday:  "четверг",
		schedule.Friday:    "пятница",
		schedule.Saturday:  "суббота",
	}
	timeSlots := map[int]string{
		1: "8:00",
		2: "9:40",
		3: "11:30",
		4: "13:20",
		5: "15:00",
		6: "16:40",
	}

	matchesWeek := func(a schedule.Assignment) bool {
		switch weekParam {
		case "even":
			return a.Parity == schedule.Even || a.Parity == schedule.Always
		case "odd":
			return a.Parity == schedule.Odd || a.Parity == schedule.Always
		default:
			return true
		}
	}

	// ========== all_teachers ==========
	if viewType == "all_teachers" {
		type slotInfo struct {
			even *schedule.Assignment
			odd  *schedule.Assignment
		}
		teacherGrid := make(map[string]map[schedule.Day]map[int]*slotInfo)

		for _, a := range sched.Assignments {
			if !matchesWeek(a) {
				continue
			}
			tid := a.TeacherID
			if teacherGrid[tid] == nil {
				teacherGrid[tid] = make(map[schedule.Day]map[int]*slotInfo)
			}
			day := a.TimeSlot.Day
			if teacherGrid[tid][day] == nil {
				teacherGrid[tid][day] = make(map[int]*slotInfo)
			}
			pair := a.TimeSlot.PairNum
			if pair < 1 || pair > 6 {
				continue
			}
			if teacherGrid[tid][day][pair] == nil {
				teacherGrid[tid][day][pair] = &slotInfo{}
			}
			info := teacherGrid[tid][day][pair]

			if a.Parity == schedule.Always {
				info.even = &a
				info.odd = &a
			} else if a.Parity == schedule.Even {
				info.even = &a
			} else if a.Parity == schedule.Odd {
				info.odd = &a
			}
		}

		teachers := input.Teachers
		sort.Slice(teachers, func(i, j int) bool {
			return teacherMap[teachers[i].ID] < teacherMap[teachers[j].ID]
		})

		f := excelize.NewFile()
		defer f.Close()
		sheet := "Все преподаватели"
		f.SetSheetName("Sheet1", sheet)

		borderStyle := []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		}
		style, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
			Border:    borderStyle,
		})
		styleGreen, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
			Border:    borderStyle,
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2EFDA"}},
		})
		dayStyle := func(dayIdx int) int {
			if dayIdx%2 == 0 {
				return styleGreen
			}
			return style
		}

		// Заголовки
		f.SetCellValue(sheet, "A1", "День")
		f.SetCellValue(sheet, "B1", "Время")
		for i, t := range teachers {
			col, _ := excelize.CoordinatesToCellName(i+3, 1)
			f.SetCellValue(sheet, col, teacherMap[t.ID])
		}

		currentRow := 2
		for dayIdx, day := range days {
			ds := dayStyle(dayIdx)
			for pair := 1; pair <= 6; pair++ {
				startRow := currentRow
				timeStr := timeSlots[pair]

				// Для КАЖДОГО преподавателя определяем, сколько строк ему нужно
				teacherRowsNeeded := make(map[string]int)
				maxRowsForSlot := 1

				for _, t := range teachers {
					info := teacherGrid[t.ID][day][pair]
					if info == nil {
						teacherRowsNeeded[t.ID] = 1
						continue
					}

					// always-пара (одна и та же для обеих недель) → 1 строка
					// even-only, odd-only или разные → всегда 2 строки (чётная сверху, нечётная снизу)
					isAlways := info.even != nil && info.odd != nil && info.even == info.odd
					if isAlways {
						teacherRowsNeeded[t.ID] = 1
					} else {
						teacherRowsNeeded[t.ID] = 2
						if maxRowsForSlot < 2 {
							maxRowsForSlot = 2
						}
					}
				}

				// Сначала заполняем значения
				for subRow := 0; subRow < maxRowsForSlot; subRow++ {
					row := currentRow + subRow

					parityFilter := schedule.Always
					if maxRowsForSlot == 2 {
						if subRow == 0 {
							parityFilter = schedule.Even
						} else {
							parityFilter = schedule.Odd
						}
					}

					for colIndex, t := range teachers {
						col := colIndex + 3
						cellRef, _ := excelize.CoordinatesToCellName(col, row)

						info := teacherGrid[t.ID][day][pair]
						if info == nil {
							f.SetCellStyle(sheet, cellRef, cellRef, ds)
							continue
						}

						var a *schedule.Assignment
						rowsNeeded := teacherRowsNeeded[t.ID]

						if rowsNeeded == 1 {
							// Одна строка для этого преподавателя — показываем то, что есть
							if info.even != nil && info.odd != nil && info.even == info.odd {
								a = info.even
							} else if info.even != nil {
								a = info.even
							} else if info.odd != nil {
								a = info.odd
							}
						} else {
							// Две строки для этого преподавателя — чётная и нечётная отдельно
							if parityFilter == schedule.Even && info.even != nil {
								a = info.even
							} else if parityFilter == schedule.Odd && info.odd != nil {
								a = info.odd
							}
						}

						if a != nil {
							subject := subjectMap[a.SubjectID]
							if subject == "" {
								subject = a.SubjectID
							}
							room := roomMap[a.RoomID]
							if room == "" {
								room = a.RoomID
							}
							typeStr := string(a.Type)
							switch a.Type {
							case schedule.Lecture:
								typeStr = "лек"
							case schedule.Practice:
								typeStr = "пр"
							case schedule.Lab:
								typeStr = "лаб"
							}

							teacherName := teacherMap[a.TeacherID]
							value := fmt.Sprintf("%s (%s)\n%s\nауд.%s", subject, typeStr, teacherName, room)

							if rowsNeeded == 2 {
								if parityFilter == schedule.Even {
									value = value + "\n(чётная неделя)"
								} else {
									value = value + "\n(нечётная неделя)"
								}
							}
							f.SetCellValue(sheet, cellRef, value)
						}
						f.SetCellStyle(sheet, cellRef, cellRef, ds)
					}
				}

				// Объединяем ячейки День и Время
				if maxRowsForSlot > 1 {
					f.MergeCell(sheet, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", startRow+maxRowsForSlot-1))
					f.MergeCell(sheet, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", startRow+maxRowsForSlot-1))
				}

				f.SetCellValue(sheet, fmt.Sprintf("A%d", startRow), dayNames[day])
				f.SetCellValue(sheet, fmt.Sprintf("B%d", startRow), timeStr)
				f.SetCellStyle(sheet, fmt.Sprintf("A%d", startRow), fmt.Sprintf("B%d", startRow+maxRowsForSlot-1), ds)

				// Объединяем ячейки для преподавателей с rowsNeeded = 1
				if maxRowsForSlot == 2 {
					for colIndex, t := range teachers {
						if teacherRowsNeeded[t.ID] == 1 {
							col, _ := excelize.CoordinatesToCellName(colIndex+3, startRow)
							endCol, _ := excelize.CoordinatesToCellName(colIndex+3, startRow+1)
							f.MergeCell(sheet, col, endCol)
						}
					}
				}

				// Высота строк
				for subRow := 0; subRow < maxRowsForSlot; subRow++ {
					f.SetRowHeight(sheet, startRow+subRow, 60)
				}

				currentRow += maxRowsForSlot
			}
		}

		// Ширина столбцов
		lastColName, _ := excelize.ColumnNumberToName(len(teachers) + 2)
		f.SetColWidth(sheet, "A", "A", 16)
		f.SetColWidth(sheet, "B", "B", 8)
		f.SetColWidth(sheet, "C", lastColName, 28)

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=teachers_schedule_%d.xlsx", scheduleID))
		w.WriteHeader(http.StatusOK)
		f.Write(w)
		return
	}

	// ========== year ==========
	if viewType == "year" {
		yearParam := r.URL.Query().Get("year")
		if yearParam == "" {
			writeError(w, http.StatusBadRequest, "year param is required")
			return
		}

		// Собираем группы нужного года
		var yearGroups []schedule.Group
		for _, g := range input.Groups {
			m := yearRe.FindStringSubmatch(g.Name)
			if m != nil && "20"+m[1] == yearParam {
				yearGroups = append(yearGroups, g)
			}
		}
		if len(yearGroups) == 0 {
			writeError(w, http.StatusNotFound, "no groups found for year "+yearParam)
			return
		}
		sort.Slice(yearGroups, func(i, j int) bool { return yearGroups[i].Name < yearGroups[j].Name })

		f := excelize.NewFile()
		defer f.Close()

		borderStyleY := []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		}

		firstSheet := true
		for _, grp := range yearGroups {
			sheetName := grp.ID
			if firstSheet {
				f.SetSheetName("Sheet1", sheetName)
				firstSheet = false
			} else {
				f.NewSheet(sheetName)
			}

			styleY, _ := f.NewStyle(&excelize.Style{
				Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
				Border:    borderStyleY,
			})
			styleYGreen, _ := f.NewStyle(&excelize.Style{
				Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
				Border:    borderStyleY,
				Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2EFDA"}},
			})

			type slotInfoY struct {
				even *schedule.Assignment
				odd  *schedule.Assignment
			}
			grid := make(map[schedule.Day]map[int]*slotInfoY)

			for i := range sched.Assignments {
				a := &sched.Assignments[i]
				if !matchesWeek(*a) {
					continue
				}
				found := false
				for _, gid := range a.GroupIDs {
					if gid == grp.ID {
						found = true
						break
					}
				}
				if !found {
					continue
				}
				day := a.TimeSlot.Day
				pair := a.TimeSlot.PairNum
				if pair < 1 || pair > 6 {
					continue
				}
				if grid[day] == nil {
					grid[day] = make(map[int]*slotInfoY)
				}
				if grid[day][pair] == nil {
					grid[day][pair] = &slotInfoY{}
				}
				info := grid[day][pair]
				if a.Parity == schedule.Always {
					info.even = a
					info.odd = a
				} else if a.Parity == schedule.Even {
					info.even = a
				} else if a.Parity == schedule.Odd {
					info.odd = a
				}
			}

			f.SetCellValue(sheetName, "A1", "День")
			f.SetCellValue(sheetName, "B1", "Время")
			f.SetCellValue(sheetName, "C1", "Предмет")

			currentRow := 2
			for dayIdx, day := range days {
				ds := styleY
				if dayIdx%2 == 0 {
					ds = styleYGreen
				}
				for pair := 1; pair <= 6; pair++ {
					startRow := currentRow
					info := grid[day][pair]

					hasEven := info != nil && info.even != nil
					hasOdd := info != nil && info.odd != nil
					isAlways := hasEven && hasOdd && info.even == info.odd

					numRows := 1
					if !isAlways && (hasEven || hasOdd) {
						numRows = 2
					}

					for subRow := 0; subRow < numRows; subRow++ {
						row := currentRow + subRow
						var a *schedule.Assignment
						weekLabel := ""

						if numRows == 1 {
							if isAlways {
								a = info.even
							} else if hasEven {
								a = info.even
								weekLabel = "(чётная неделя)"
							} else if hasOdd {
								a = info.odd
								weekLabel = "(нечётная неделя)"
							}
						} else {
							if subRow == 0 && hasEven {
								a = info.even
								weekLabel = "(чётная неделя)"
							} else if subRow == 1 && hasOdd {
								a = info.odd
								weekLabel = "(нечётная неделя)"
							}
						}

						if a != nil {
							subject := subjectMap[a.SubjectID]
							if subject == "" {
								subject = a.SubjectID
							}
							room := roomMap[a.RoomID]
							if room == "" {
								room = a.RoomID
							}
							typeStr := string(a.Type)
							switch a.Type {
							case schedule.Lecture:
								typeStr = "лек"
							case schedule.Practice:
								typeStr = "пр"
							case schedule.Lab:
								typeStr = "лаб"
							}
							teacherName := teacherMap[a.TeacherID]
							value := fmt.Sprintf("%s (%s)\n%s\nауд.%s %s", subject, typeStr, teacherName, room, weekLabel)
							f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), value)
						}
						f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), ds)
					}

					if numRows > 1 {
						f.MergeCell(sheetName, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", startRow+numRows-1))
						f.MergeCell(sheetName, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", startRow+numRows-1))
					}
					f.SetCellValue(sheetName, fmt.Sprintf("A%d", startRow), dayNames[day])
					f.SetCellValue(sheetName, fmt.Sprintf("B%d", startRow), timeSlots[pair])

					currentRow += numRows
				}
			}

			f.SetColWidth(sheetName, "A", "A", 16)
			f.SetColWidth(sheetName, "B", "B", 8)
			f.SetColWidth(sheetName, "C", "C", 55)
		}

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=schedule_%d_year%s.xlsx", scheduleID, yearParam))
		w.WriteHeader(http.StatusOK)
		f.Write(w)
		return
	}

	// ========== group / teacher ==========
	if entityID == "" {
		writeError(w, http.StatusBadRequest, "entity id is required for group/teacher view")
		return
	}

	type slotInfo struct {
		even *schedule.Assignment
		odd  *schedule.Assignment
	}
	grid := make(map[schedule.Day]map[int]*slotInfo)

	for _, a := range sched.Assignments {
		if !matchesWeek(a) {
			continue
		}
		belongs := false
		if viewType == "group" {
			for _, gid := range a.GroupIDs {
				if gid == entityID {
					belongs = true
					break
				}
			}
		} else {
			if a.TeacherID == entityID {
				belongs = true
			}
		}
		if !belongs {
			continue
		}

		day := a.TimeSlot.Day
		pair := a.TimeSlot.PairNum
		if pair < 1 || pair > 6 {
			continue
		}
		if grid[day] == nil {
			grid[day] = make(map[int]*slotInfo)
		}
		if grid[day][pair] == nil {
			grid[day][pair] = &slotInfo{}
		}
		info := grid[day][pair]

		if a.Parity == schedule.Always {
			info.even = &a
			info.odd = &a
		} else if a.Parity == schedule.Even {
			info.even = &a
		} else if a.Parity == schedule.Odd {
			info.odd = &a
		}
	}

	f := excelize.NewFile()
	defer f.Close()
	sheetName := "Расписание"
	if viewType == "group" {
		sheetName = groupMap[entityID]
	} else {
		sheetName = teacherMap[entityID]
	}
	if sheetName == "" {
		sheetName = entityID
	}
	f.SetSheetName("Sheet1", sheetName)

	borderStyle2 := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1},
		{Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1},
		{Type: "bottom", Color: "000000", Style: 1},
	}
	style, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
		Border:    borderStyle2,
	})
	styleGreen, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
		Border:    borderStyle2,
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2EFDA"}},
	})
	dayStyleSingle := func(dayIdx int) int {
		if dayIdx%2 == 0 {
			return styleGreen
		}
		return style
	}

	f.SetCellValue(sheetName, "A1", "День")
	f.SetCellValue(sheetName, "B1", "Время")
	f.SetCellValue(sheetName, "C1", "Предмет")

	currentRow := 2
	for dayIdx, day := range days {
		ds := dayStyleSingle(dayIdx)
		for pair := 1; pair <= 6; pair++ {
			startRow := currentRow
			timeStr := timeSlots[pair]
			info := grid[day][pair]

			hasEven := info != nil && info.even != nil
			hasOdd := info != nil && info.odd != nil
			isAlways := hasEven && hasOdd && info.even == info.odd

			// always → 1 строка; even-only / odd-only / разные → 2 строки
			numRows := 1
			if !isAlways && (hasEven || hasOdd) {
				numRows = 2
			}

			for subRow := 0; subRow < numRows; subRow++ {
				row := currentRow + subRow
				var a *schedule.Assignment
				weekLabel := ""

				if numRows == 1 {
					if hasEven && hasOdd && info.even == info.odd {
						a = info.even
					} else if hasEven {
						a = info.even
						weekLabel = "(чётная неделя)"
					} else if hasOdd {
						a = info.odd
						weekLabel = "(нечётная неделя)"
					}
				} else {
					if subRow == 0 && hasEven {
						a = info.even
						weekLabel = "(чётная неделя)"
					} else if subRow == 1 && hasOdd {
						a = info.odd
						weekLabel = "(нечётная неделя)"
					}
				}

				if a != nil {
					subject := subjectMap[a.SubjectID]
					if subject == "" {
						subject = a.SubjectID
					}
					room := roomMap[a.RoomID]
					if room == "" {
						room = a.RoomID
					}
					typeStr := string(a.Type)
					switch a.Type {
					case schedule.Lecture:
						typeStr = "лек"
					case schedule.Practice:
						typeStr = "пр"
					case schedule.Lab:
						typeStr = "лаб"
					}
					teacherName := teacherMap[a.TeacherID]
					value := fmt.Sprintf("%s (%s)\n%s\nауд.%s %s", subject, typeStr, teacherName, room, weekLabel)
					f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), value)
				}
				f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), ds)
			}

			if numRows > 1 {
				f.MergeCell(sheetName, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", startRow+numRows-1))
				f.MergeCell(sheetName, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", startRow+numRows-1))
			}

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", startRow), dayNames[day])
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", startRow), timeStr)

			currentRow += numRows
		}
	}

	f.SetColWidth(sheetName, "A", "A", 16)
	f.SetColWidth(sheetName, "B", "B", 8)
	f.SetColWidth(sheetName, "C", "C", 55)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_schedule_%d.xlsx", sheetName, scheduleID))
	w.WriteHeader(http.StatusOK)
	f.Write(w)
}
