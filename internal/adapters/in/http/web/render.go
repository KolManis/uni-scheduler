package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
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
	"sub": func(a, b int) int { return a - b },
	"contains": func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	},
	"buildingName": func(buildings []domain.Building, id string) string {
		for _, b := range buildings {
			if b.ID == id {
				return b.Name
			}
		}
		return id
	},
	"departmentName": func(departments []domain.Department, id string) string {
		for _, d := range departments {
			if d.ID == id {
				return d.Name
			}
		}
		return id
	},
	"teacherName": func(teachers []domain.Teacher, id string) string {
		for _, t := range teachers {
			if t.ID == id {
				return t.Name
			}
		}
		return id
	},
	"groupName": func(groups []domain.Group, id string) string {
		for _, g := range groups {
			if g.ID == id {
				return g.Name
			}
		}
		return id
	},
	"joinNames": func(groups []domain.Group, ids []string) string {
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
	"subjectName": func(plans []domain.SubjectPlan, id string) string {
		for _, sp := range plans {
			if sp.ID == id {
				return sp.Name
			}
		}
		return id
	},
	"planByID": func(plans []domain.SubjectPlan, id string) domain.SubjectPlan {
		for _, sp := range plans {
			if sp.ID == id {
				return sp
			}
		}
		return domain.SubjectPlan{}
	},
	"roomNumber": func(rooms []domain.Room, id string) string {
		for _, rm := range rooms {
			if rm.ID == id {
				return rm.Number
			}
		}
		return id
	},
	// roomLabel — «номер, корпус»: по одному номеру аудитории не понять, в каком она корпусе.
	"roomLabel": func(rooms []domain.Room, buildings []domain.Building, id string) string {
		for _, rm := range rooms {
			if rm.ID != id {
				continue
			}
			for _, b := range buildings {
				if b.ID == rm.BuildingID {
					return rm.Number + ", " + b.Name
				}
			}
			return rm.Number
		}
		return id
	},
	"hasUnavailable": func(slots []domain.TimeSlot, day domain.Day, pair int) bool {
		for _, s := range slots {
			if s.Day() == day && s.PairNum() == pair {
				return true
			}
		}
		return false
	},
	"allPairsUnavailable": func(slots []domain.TimeSlot, day domain.Day, pairs []int) bool {
		for _, p := range pairs {
			found := false
			for _, s := range slots {
				if s.Day() == day && s.PairNum() == p {
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
	"dayLabel": func(d domain.Day) string {
		switch d {
		case domain.Monday:
			return "Пн"
		case domain.Tuesday:
			return "Вт"
		case domain.Wednesday:
			return "Ср"
		case domain.Thursday:
			return "Чт"
		case domain.Friday:
			return "Пт"
		case domain.Saturday:
			return "Сб"
		default:
			return string(d)
		}
	},
}

var allDays = domain.AllDays
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
