package rest

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"

	"github.com/KolManis/uni-scheduler/internal/core/application/queries/getschedule"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// ExcelHandler — выгрузка расписания в Excel: запрос getschedule плюс справочники для подписей.
type ExcelHandler struct {
	getSchedule *getschedule.Handler
	inputRepo   ports.InputRepository
}

func NewExcelHandler(getSchedule *getschedule.Handler, inputRepo ports.InputRepository) *ExcelHandler {
	return &ExcelHandler{getSchedule: getSchedule, inputRepo: inputRepo}
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

	q, err := getschedule.NewQuery(scheduleID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	sched, err := h.getSchedule.Handle(r.Context(), q)
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
	buildingMap := make(map[string]string)
	for _, b := range input.Buildings {
		buildingMap[b.ID] = b.Name
	}
	// «номер, корпус»: по одному номеру аудитории не понять, в каком она корпусе.
	roomMap := make(map[string]string)
	for _, r := range input.Rooms {
		roomMap[r.ID] = r.Number
		if name := buildingMap[r.BuildingID]; name != "" {
			roomMap[r.ID] = r.Number + ", " + name
		}
	}
	subjectMap := make(map[string]string)
	for _, s := range input.SubjectPlans {
		subjectMap[s.ID] = s.Name
	}

	days := []domain.Day{
		domain.Monday, domain.Tuesday, domain.Wednesday,
		domain.Thursday, domain.Friday, domain.Saturday,
	}
	dayNames := map[domain.Day]string{
		domain.Monday:    "понедельник",
		domain.Tuesday:   "вторник",
		domain.Wednesday: "среда",
		domain.Thursday:  "четверг",
		domain.Friday:    "пятница",
		domain.Saturday:  "суббота",
	}
	timeSlots := map[int]string{
		1: "8:00",
		2: "9:40",
		3: "11:30",
		4: "13:20",
		5: "15:00",
		6: "16:40",
	}

	matchesWeek := func(a domain.Assignment) bool {
		switch weekParam {
		case "even":
			return a.Parity == domain.Even || a.Parity == domain.Always
		case "odd":
			return a.Parity == domain.Odd || a.Parity == domain.Always
		default:
			return true
		}
	}

	// ========== all_teachers ==========
	if viewType == "all_teachers" {
		type slotInfo struct {
			even *domain.Assignment
			odd  *domain.Assignment
		}
		teacherGrid := make(map[string]map[domain.Day]map[int]*slotInfo)

		// Пары на других факультетах — в сетку как занятия особого вида: у преподавателя
		// в выгрузке видно всё его время, а не «дыра» там, где он занят вне кафедры.
		cells := withExternalPairs(sched.Assignments, input.Teachers)
		for _, a := range cells {
			if !matchesWeek(a) {
				continue
			}
			tid := a.TeacherID
			if teacherGrid[tid] == nil {
				teacherGrid[tid] = make(map[domain.Day]map[int]*slotInfo)
			}
			day := a.TimeSlot.Day()
			if teacherGrid[tid][day] == nil {
				teacherGrid[tid][day] = make(map[int]*slotInfo)
			}
			pair := a.TimeSlot.PairNum()
			if pair < 1 || pair > 6 {
				continue
			}
			if teacherGrid[tid][day][pair] == nil {
				teacherGrid[tid][day][pair] = &slotInfo{}
			}
			info := teacherGrid[tid][day][pair]

			if a.Parity == domain.Always {
				info.even = &a
				info.odd = &a
			} else if a.Parity == domain.Even {
				info.even = &a
			} else if a.Parity == domain.Odd {
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
		styleExternal, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "center"},
			Border:    borderStyle,
			Font:      &excelize.Font{Italic: true, Color: "595959"},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EDEDED"}},
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

					parityFilter := domain.Always
					if maxRowsForSlot == 2 {
						if subRow == 0 {
							parityFilter = domain.Even
						} else {
							parityFilter = domain.Odd
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

						var a *domain.Assignment
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
							if parityFilter == domain.Even && info.even != nil {
								a = info.even
							} else if parityFilter == domain.Odd && info.odd != nil {
								a = info.odd
							}
						}

						if a != nil && a.Type == externalPairType {
							value := "Другой факультет"
							if a.SubjectID != "" {
								value += "\n" + a.SubjectID
							}
							if rowsNeeded == 2 {
								if parityFilter == domain.Even {
									value += "\n(чётная неделя)"
								} else {
									value += "\n(нечётная неделя)"
								}
							}
							f.SetCellValue(sheet, cellRef, value)
							f.SetCellStyle(sheet, cellRef, cellRef, styleExternal)
							continue
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
							case domain.Lecture:
								typeStr = "лек"
							case domain.Practice:
								typeStr = "пр"
							case domain.Lab:
								typeStr = "лаб"
							}

							teacherName := teacherMap[a.TeacherID]
							value := fmt.Sprintf("%s (%s)\n%s\nауд.%s", subject, typeStr, teacherName, room)

							if rowsNeeded == 2 {
								if parityFilter == domain.Even {
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

	// ========== all_groups ==========
	if viewType == "all_groups" {
		type slotInfo struct {
			even *domain.Assignment
			odd  *domain.Assignment
		}
		groupGrid := make(map[string]map[domain.Day]map[int]*slotInfo)

		for i := range sched.Assignments {
			a := sched.Assignments[i]
			if !matchesWeek(a) {
				continue
			}
			day := a.TimeSlot.Day()
			pair := a.TimeSlot.PairNum()
			if pair < 1 || pair > 6 {
				continue
			}
			for _, gid := range a.GroupIDs {
				if groupGrid[gid] == nil {
					groupGrid[gid] = make(map[domain.Day]map[int]*slotInfo)
				}
				if groupGrid[gid][day] == nil {
					groupGrid[gid][day] = make(map[int]*slotInfo)
				}
				if groupGrid[gid][day][pair] == nil {
					groupGrid[gid][day][pair] = &slotInfo{}
				}
				info := groupGrid[gid][day][pair]

				if a.Parity == domain.Always {
					info.even = &a
					info.odd = &a
				} else if a.Parity == domain.Even {
					info.even = &a
				} else if a.Parity == domain.Odd {
					info.odd = &a
				}
			}
		}

		groups := input.Groups
		sort.Slice(groups, func(i, j int) bool {
			return groupMap[groups[i].ID] < groupMap[groups[j].ID]
		})

		f := excelize.NewFile()
		defer f.Close()
		sheet := "Все группы"
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
		for i, g := range groups {
			col, _ := excelize.CoordinatesToCellName(i+3, 1)
			f.SetCellValue(sheet, col, groupMap[g.ID])
		}

		currentRow := 2
		for dayIdx, day := range days {
			ds := dayStyle(dayIdx)
			for pair := 1; pair <= 6; pair++ {
				startRow := currentRow
				timeStr := timeSlots[pair]

				// Для КАЖДОЙ группы определяем, сколько строк ей нужно
				groupRowsNeeded := make(map[string]int)
				maxRowsForSlot := 1

				for _, g := range groups {
					info := groupGrid[g.ID][day][pair]
					if info == nil {
						groupRowsNeeded[g.ID] = 1
						continue
					}

					isAlways := info.even != nil && info.odd != nil && info.even == info.odd
					if isAlways {
						groupRowsNeeded[g.ID] = 1
					} else {
						groupRowsNeeded[g.ID] = 2
						if maxRowsForSlot < 2 {
							maxRowsForSlot = 2
						}
					}
				}

				// Сначала заполняем значения
				for subRow := 0; subRow < maxRowsForSlot; subRow++ {
					row := currentRow + subRow

					parityFilter := domain.Always
					if maxRowsForSlot == 2 {
						if subRow == 0 {
							parityFilter = domain.Even
						} else {
							parityFilter = domain.Odd
						}
					}

					for colIndex, g := range groups {
						col := colIndex + 3
						cellRef, _ := excelize.CoordinatesToCellName(col, row)

						info := groupGrid[g.ID][day][pair]
						if info == nil {
							f.SetCellStyle(sheet, cellRef, cellRef, ds)
							continue
						}

						var a *domain.Assignment
						rowsNeeded := groupRowsNeeded[g.ID]

						if rowsNeeded == 1 {
							if info.even != nil && info.odd != nil && info.even == info.odd {
								a = info.even
							} else if info.even != nil {
								a = info.even
							} else if info.odd != nil {
								a = info.odd
							}
						} else {
							if parityFilter == domain.Even && info.even != nil {
								a = info.even
							} else if parityFilter == domain.Odd && info.odd != nil {
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
							case domain.Lecture:
								typeStr = "лек"
							case domain.Practice:
								typeStr = "пр"
							case domain.Lab:
								typeStr = "лаб"
							}

							teacherName := teacherMap[a.TeacherID]
							value := fmt.Sprintf("%s (%s)\n%s\nауд.%s", subject, typeStr, teacherName, room)

							if rowsNeeded == 2 {
								if parityFilter == domain.Even {
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

				// Объединяем ячейки для групп с rowsNeeded = 1
				if maxRowsForSlot == 2 {
					for colIndex, g := range groups {
						if groupRowsNeeded[g.ID] == 1 {
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
		lastColName, _ := excelize.ColumnNumberToName(len(groups) + 2)
		f.SetColWidth(sheet, "A", "A", 16)
		f.SetColWidth(sheet, "B", "B", 8)
		f.SetColWidth(sheet, "C", lastColName, 28)

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=groups_schedule_%d.xlsx", scheduleID))
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
		var yearGroups []domain.Group
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
				even *domain.Assignment
				odd  *domain.Assignment
			}
			grid := make(map[domain.Day]map[int]*slotInfoY)

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
				day := a.TimeSlot.Day()
				pair := a.TimeSlot.PairNum()
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
				if a.Parity == domain.Always {
					info.even = a
					info.odd = a
				} else if a.Parity == domain.Even {
					info.even = a
				} else if a.Parity == domain.Odd {
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
						var a *domain.Assignment
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
							case domain.Lecture:
								typeStr = "лек"
							case domain.Practice:
								typeStr = "пр"
							case domain.Lab:
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
		even *domain.Assignment
		odd  *domain.Assignment
	}
	grid := make(map[domain.Day]map[int]*slotInfo)

	cells := sched.Assignments
	if viewType != "group" {
		cells = withExternalPairs(sched.Assignments, input.Teachers)
	}
	for _, a := range cells {
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

		day := a.TimeSlot.Day()
		pair := a.TimeSlot.PairNum()
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

		if a.Parity == domain.Always {
			info.even = &a
			info.odd = &a
		} else if a.Parity == domain.Even {
			info.even = &a
		} else if a.Parity == domain.Odd {
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
				var a *domain.Assignment
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

				if a != nil && a.Type == externalPairType {
					value := "Другой факультет"
					if a.SubjectID != "" {
						value += "\n" + a.SubjectID
					}
					f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), strings.TrimSpace(value+" "+weekLabel))
				} else if a != nil {
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
					case domain.Lecture:
						typeStr = "лек"
					case domain.Practice:
						typeStr = "пр"
					case domain.Lab:
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

// externalPairType — вид «занятия» для пары преподавателя на другом факультете в выгрузке;
// в SubjectID такой записи лежит пометка пары.
const externalPairType domain.ClassType = "external"

// withExternalPairs — пары расписания плюс пары преподавателей на других факультетах
// в виде записей externalPairType (пометка — в SubjectID).
func withExternalPairs(assignments []domain.Assignment, teachers []domain.Teacher) []domain.Assignment {
	cells := append([]domain.Assignment(nil), assignments...)
	for _, t := range teachers {
		for _, ep := range t.ExternalPairs {
			p := ep.Parity
			if p == "" {
				p = domain.Always
			}
			cells = append(cells, domain.Assignment{
				TeacherID: t.ID, SubjectID: ep.Note, Type: externalPairType, TimeSlot: ep.TimeSlot, Parity: p,
			})
		}
	}
	return cells
}
