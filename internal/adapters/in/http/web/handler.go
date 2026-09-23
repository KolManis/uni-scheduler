package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/KolManis/uni-scheduler/internal/core/app"
	"github.com/KolManis/uni-scheduler/internal/core/domain"
	"github.com/gorilla/mux"
)

type inputLoader interface {
	LoadInput(ctx context.Context) (*domain.InputData, error)
}

// refWriter — те же операции, что использует JSON-API (internal/adapters/in/http/rest.RefWriteHandler),
// UI-слой работает поверх того же репозитория, просто рендерит HTML вместо JSON.
type refWriter interface {
	InsertBuilding(ctx context.Context, b domain.Building) error
	UpdateBuilding(ctx context.Context, b domain.Building) (bool, error)
	DeleteBuilding(ctx context.Context, id string) (bool, error)

	InsertDepartment(ctx context.Context, d domain.Department) error
	UpdateDepartment(ctx context.Context, d domain.Department) (bool, error)
	DeleteDepartment(ctx context.Context, id string) (bool, error)

	InsertRoom(ctx context.Context, rm domain.Room) error
	UpdateRoom(ctx context.Context, rm domain.Room) (bool, error)
	DeleteRoom(ctx context.Context, id string) (bool, error)

	InsertGroup(ctx context.Context, g domain.Group) error
	UpdateGroup(ctx context.Context, g domain.Group) (bool, error)
	DeleteGroup(ctx context.Context, id string) (bool, error)

	InsertTeacher(ctx context.Context, t domain.Teacher) error
	UpdateTeacher(ctx context.Context, t domain.Teacher) (bool, error)
	DeleteTeacher(ctx context.Context, id string) (bool, error)

	InsertSubjectPlan(ctx context.Context, sp domain.SubjectPlan) error
	UpdateSubjectPlan(ctx context.Context, sp domain.SubjectPlan) (bool, error)
	DeleteSubjectPlan(ctx context.Context, id string) (bool, error)
}

type scheduleService interface {
	Generate(ctx context.Context, in app.GenerateInput) (*domain.Schedule, error)
	GenerateAllMethods(ctx context.Context, in app.GenerateInput) ([]*domain.Schedule, error)
	GetByID(ctx context.Context, id int64) (*domain.Schedule, error)
	List(ctx context.Context) ([]domain.ScheduleSummary, error)
	Breakdown(sched *domain.Schedule, input domain.InputData) domain.FitnessBreakdown
	Quality(sched *domain.Schedule) domain.QualityStats
	Delete(ctx context.Context, id int64) error
	PatchAssignment(ctx context.Context, schedID int64, idx int, req app.PatchRequest) (*domain.Schedule, error)
}

// Handler отдаёт серверно-рендеренный UI (Go html/template + лёгкий AJAX-хелпер вместо htmx)
// поверх тех же репозиториев/сервиса, что использует JSON REST API.
type Handler struct {
	input inputLoader
	ref   refWriter
	svc   scheduleService
	pages pageTemplates
}

func NewHandler(input inputLoader, ref refWriter, svc scheduleService) *Handler {
	return &Handler{
		input: input,
		ref:   ref,
		svc:   svc,
		pages: loadPages(
			"buildings_list.html", "buildings_form.html",
			"departments_list.html", "departments_form.html",
			"rooms_list.html", "rooms_form.html",
			"groups_list.html", "groups_form.html",
			"teachers_list.html", "teachers_form.html",
			"subject_plans_list.html", "subject_plans_form.html",
			"schedules_list.html", "schedules_view.html", "schedules_assignment_form.html", "schedules_groups_view.html",
			"help.html",
		),
	}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/schedules", http.StatusFound)
	}).Methods(http.MethodGet)

	ui := r.PathPrefix("/ui").Subrouter()

	ui.HandleFunc("/buildings", h.buildingsList).Methods(http.MethodGet)
	ui.HandleFunc("/buildings/new", h.buildingsForm).Methods(http.MethodGet)
	ui.HandleFunc("/buildings/{id}/edit", h.buildingsForm).Methods(http.MethodGet)
	ui.HandleFunc("/buildings", h.buildingsCreate).Methods(http.MethodPost)
	ui.HandleFunc("/buildings/{id}", h.buildingsUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/buildings/{id}", h.buildingsDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/departments", h.departmentsList).Methods(http.MethodGet)
	ui.HandleFunc("/departments/new", h.departmentsForm).Methods(http.MethodGet)
	ui.HandleFunc("/departments/{id}/edit", h.departmentsForm).Methods(http.MethodGet)
	ui.HandleFunc("/departments", h.departmentsCreate).Methods(http.MethodPost)
	ui.HandleFunc("/departments/{id}", h.departmentsUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/departments/{id}", h.departmentsDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/rooms", h.roomsList).Methods(http.MethodGet)
	ui.HandleFunc("/rooms/new", h.roomsForm).Methods(http.MethodGet)
	ui.HandleFunc("/rooms/{id}/edit", h.roomsForm).Methods(http.MethodGet)
	ui.HandleFunc("/rooms", h.roomsCreate).Methods(http.MethodPost)
	ui.HandleFunc("/rooms/{id}", h.roomsUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/rooms/{id}", h.roomsDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/groups", h.groupsList).Methods(http.MethodGet)
	ui.HandleFunc("/groups/new", h.groupsForm).Methods(http.MethodGet)
	ui.HandleFunc("/groups/{id}/edit", h.groupsForm).Methods(http.MethodGet)
	ui.HandleFunc("/groups", h.groupsCreate).Methods(http.MethodPost)
	ui.HandleFunc("/groups/{id}", h.groupsUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/groups/{id}", h.groupsDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/teachers", h.teachersList).Methods(http.MethodGet)
	ui.HandleFunc("/teachers/new", h.teachersForm).Methods(http.MethodGet)
	ui.HandleFunc("/teachers/{id}/edit", h.teachersForm).Methods(http.MethodGet)
	ui.HandleFunc("/teachers", h.teachersCreate).Methods(http.MethodPost)
	ui.HandleFunc("/teachers/{id}", h.teachersUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/teachers/{id}", h.teachersDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/subject-plans", h.subjectPlansList).Methods(http.MethodGet)
	ui.HandleFunc("/subject-plans/new", h.subjectPlansForm).Methods(http.MethodGet)
	ui.HandleFunc("/subject-plans/{id}/edit", h.subjectPlansForm).Methods(http.MethodGet)
	ui.HandleFunc("/subject-plans", h.subjectPlansCreate).Methods(http.MethodPost)
	ui.HandleFunc("/subject-plans/{id}", h.subjectPlansUpdate).Methods(http.MethodPut)
	ui.HandleFunc("/subject-plans/{id}", h.subjectPlansDelete).Methods(http.MethodDelete)

	ui.HandleFunc("/help", h.help).Methods(http.MethodGet)

	ui.HandleFunc("/schedules", h.schedulesList).Methods(http.MethodGet)
	ui.HandleFunc("/schedules/generate", h.schedulesGenerate).Methods(http.MethodPost)
	ui.HandleFunc("/schedules/{id:[0-9]+}", h.schedulesView).Methods(http.MethodGet)
	ui.HandleFunc("/schedules/{id:[0-9]+}/groups", h.schedulesGroupsView).Methods(http.MethodGet)
	ui.HandleFunc("/schedules/{id:[0-9]+}", h.schedulesDelete).Methods(http.MethodDelete)
	ui.HandleFunc("/schedules/{id:[0-9]+}/assignments/{idx:[0-9]+}/edit", h.schedulesAssignmentForm).Methods(http.MethodGet)
	ui.HandleFunc("/schedules/{id:[0-9]+}/assignments/{idx:[0-9]+}", h.schedulesPatchAssignment).Methods(http.MethodPatch)
}

// --- общие хелперы ---

// parseForm разбирает тело формы и при ошибке сразу отвечает 400.
// Возвращает false, если обработчик должен прекратить работу.
//
// Проверять обязательно: при молча проигнорированной ошибке r.Form остаётся пустым, и форма
// сохраняется так, будто пользователь всё очистил — например, у преподавателя пропадают все
// отмеченные недоступные пары, ведь они читаются из r.Form["unavailable"].
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "не удалось разобрать данные формы: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func idFromPath(r *http.Request) string { return mux.Vars(r)["id"] }

func int64FromPath(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(mux.Vars(r)[name], 10, 64)
}

func intFromPath(r *http.Request, name string) (int, error) {
	return strconv.Atoi(mux.Vars(r)[name])
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func formStrings(r *http.Request, key string) []string {
	if r.Form == nil {
		return nil
	}
	return r.Form[key]
}

func (h *Handler) help(w http.ResponseWriter, r *http.Request) {
	render(w, r, h.pages["help.html"], nil)
}
