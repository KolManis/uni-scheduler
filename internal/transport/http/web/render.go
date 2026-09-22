package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/domain/schedule"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// StaticHandler отдаёт содержимое static/ (JS/CSS) без префикса "static/" в пути.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

var funcMap = template.FuncMap{
	"contains": func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	},
	"buildingName": func(buildings []schedule.Building, id string) string {
		for _, b := range buildings {
			if b.ID == id {
				return b.Name
			}
		}
		return id
	},
	"departmentName": func(departments []schedule.Department, id string) string {
		for _, d := range departments {
			if d.ID == id {
				return d.Name
			}
		}
		return id
	},
	"teacherName": func(teachers []schedule.Teacher, id string) string {
		for _, t := range teachers {
			if t.ID == id {
				return t.Name
			}
		}
		return id
	},
	"groupName": func(groups []schedule.Group, id string) string {
		for _, g := range groups {
			if g.ID == id {
				return g.Name
			}
		}
		return id
	},
	"joinNames": func(groups []schedule.Group, ids []string) string {
		out := ""
		for i, id := range ids {
			if i > 0 {
				out += ", "
			}
			found := false
			for _, g := range groups {
				if g.ID == id {
					out += g.Name
					found = true
					break
				}
			}
			if !found {
				out += id
			}
		}
		return out
	},
	"subjectName": func(plans []schedule.SubjectPlan, id string) string {
		for _, sp := range plans {
			if sp.ID == id {
				return sp.Name
			}
		}
		return id
	},
	"planByID": func(plans []schedule.SubjectPlan, id string) schedule.SubjectPlan {
		for _, sp := range plans {
			if sp.ID == id {
				return sp
			}
		}
		return schedule.SubjectPlan{}
	},
	"roomNumber": func(rooms []schedule.Room, id string) string {
		for _, rm := range rooms {
			if rm.ID == id {
				return rm.Number
			}
		}
		return id
	},
	"hasUnavailable": func(slots []schedule.TimeSlot, day schedule.Day, pair int) bool {
		for _, s := range slots {
			if s.Day == day && s.PairNum == pair {
				return true
			}
		}
		return false
	},
	"allPairsUnavailable": func(slots []schedule.TimeSlot, day schedule.Day, pairs []int) bool {
		for _, p := range pairs {
			found := false
			for _, s := range slots {
				if s.Day == day && s.PairNum == p {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	},
	"dayLabel": func(d schedule.Day) string {
		switch d {
		case schedule.Monday:
			return "Пн"
		case schedule.Tuesday:
			return "Вт"
		case schedule.Wednesday:
			return "Ср"
		case schedule.Thursday:
			return "Чт"
		case schedule.Friday:
			return "Пт"
		case schedule.Saturday:
			return "Сб"
		default:
			return string(d)
		}
	},
}

var allDays = schedule.AllDays
var allPairs = []int{1, 2, 3, 4, 5, 6}

// pageTemplates хранит по одному изолированному набору шаблонов (layout + конкретная страница)
// на каждый файл, чтобы одноимённые блоки {{define "content"}} в разных страницах не конфликтовали.
type pageTemplates map[string]*template.Template

func loadPages(names ...string) pageTemplates {
	pages := make(pageTemplates, len(names))
	for _, name := range names {
		t := template.Must(
			template.New("layout.html").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+name),
		)
		pages[name] = t
	}
	return pages
}

// render выбирает: если запрос пришёл через наш AJAX-хелпер (заголовок HX-Request),
// отдаём только фрагмент "content"; иначе — полную страницу "layout.html" вокруг него.
func render(w http.ResponseWriter, r *http.Request, t *template.Template, data any) {
	renderStatus(w, r, http.StatusOK, t, data)
}

func renderStatus(w http.ResponseWriter, r *http.Request, status int, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	name := "content"
	if r.Header.Get("HX-Request") != "true" {
		name = "layout.html"
	}
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
