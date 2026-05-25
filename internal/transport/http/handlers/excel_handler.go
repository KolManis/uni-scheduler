package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
	"github.com/KolManis/uni-scheduler/internal/usecase/generator"
)

type ExcelHandler struct {
	usecase   generator.Usecase
	inputRepo generator.InputRepository
}

func NewExcelHandler(usecase generator.Usecase, inputRepo generator.InputRepository) *ExcelHandler {
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

		style, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "000000", Style: 1},
				{Type: "right", Color: "000000", Style: 1},
				{Type: "top", Color: "000000", Style: 1},
				{Type: "bottom", Color: "000000", Style: 1},
			},
		})

		// Заголовки
		f.SetCellValue(sheet, "A1", "День")
		f.SetCellValue(sheet, "B1", "Время")
		for i, t := range teachers {
			col, _ := excelize.CoordinatesToCellName(i+3, 1)
			f.SetCellValue(sheet, col, teacherMap[t.ID])
		}

		currentRow := 2
		for _, day := range days {
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

					hasEven := info.even != nil
					hasOdd := info.odd != nil
					hasBothDifferent := hasEven && hasOdd && info.even != info.odd

					if hasBothDifferent {
						teacherRowsNeeded[t.ID] = 2
						if maxRowsForSlot < 2 {
							maxRowsForSlot = 2
						}
					} else {
						teacherRowsNeeded[t.ID] = 1
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
							f.SetCellStyle(sheet, cellRef, cellRef, style)
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
						f.SetCellStyle(sheet, cellRef, cellRef, style)
					}
				}

				// Объединяем ячейки День и Время
				if maxRowsForSlot > 1 {
					f.MergeCell(sheet, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", startRow+maxRowsForSlot-1))
					f.MergeCell(sheet, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", startRow+maxRowsForSlot-1))
				}

				f.SetCellValue(sheet, fmt.Sprintf("A%d", startRow), dayNames[day])
				f.SetCellValue(sheet, fmt.Sprintf("B%d", startRow), timeStr)
				f.SetCellStyle(sheet, fmt.Sprintf("A%d", startRow), fmt.Sprintf("B%d", startRow+maxRowsForSlot-1), style)

				// *** КЛЮЧЕВОЕ ИСПРАВЛЕНИЕ: объединяем ячейки для преподавателей с rowsNeeded = 1 ***
				if maxRowsForSlot == 2 {
					for colIndex, t := range teachers {
						if teacherRowsNeeded[t.ID] == 1 {
							// Объединяем две строки в одну ячейку для этого преподавателя
							col, _ := excelize.CoordinatesToCellName(colIndex+3, startRow)
							endCol, _ := excelize.CoordinatesToCellName(colIndex+3, startRow+1)
							f.MergeCell(sheet, col, endCol)
						}
					}
				}

				currentRow += maxRowsForSlot
			}
		}

		// Ширина столбцов
		lastCol := len(teachers) + 2
		f.SetColWidth(sheet, "A", string(rune('A'+lastCol)), 22)

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=teachers_schedule_%d.xlsx", scheduleID))
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

	style, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	f.SetCellValue(sheetName, "A1", "День")
	f.SetCellValue(sheetName, "B1", "Время")
	f.SetCellValue(sheetName, "C1", "Предмет")

	currentRow := 2
	for _, day := range days {
		for pair := 1; pair <= 6; pair++ {
			startRow := currentRow
			timeStr := timeSlots[pair]
			info := grid[day][pair]

			hasEven := info != nil && info.even != nil
			hasOdd := info != nil && info.odd != nil
			hasBothDifferent := hasEven && hasOdd && info.even != info.odd

			numRows := 1
			if hasBothDifferent {
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
				f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), style)
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

	f.SetColWidth(sheetName, "A", "C", 25)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_schedule_%d.xlsx", sheetName, scheduleID))
	w.WriteHeader(http.StatusOK)
	f.Write(w)
}
