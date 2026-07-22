package http

import (
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/transport/http/handlers"
	"github.com/gorilla/mux"
)

func NewRouter(
	scheduleHandler *handlers.ScheduleHandler,
	excelHandler *handlers.ExcelHandler,
	importHandler *handlers.ImportHandler,
	referenceHandler *handlers.ReferenceHandler,
	refWriteHandler *handlers.RefWriteHandler,
) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods(http.MethodGet)

	api := router.PathPrefix("/api/v1").Subrouter()

	// Расписания
	api.HandleFunc("/schedules/generate", scheduleHandler.Generate).Methods(http.MethodPost)
	api.HandleFunc("/schedules", scheduleHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/schedules/{id:[0-9]+}", scheduleHandler.GetByID).Methods(http.MethodGet)
	api.HandleFunc("/schedules/{id:[0-9]+}", scheduleHandler.Delete).Methods(http.MethodDelete)
	api.HandleFunc("/schedules/{id:[0-9]+}/assignments/{idx:[0-9]+}", scheduleHandler.PatchAssignment).Methods(http.MethodPatch)

	// Экспорт Excel
	api.HandleFunc("/schedules/{id:[0-9]+}/excel", excelHandler.Export).Methods(http.MethodGet)

	// Импорт из Excel
	api.HandleFunc("/import/excel", importHandler.ImportExcel).Methods(http.MethodPost)

	// Справочники — чтение
	api.HandleFunc("/teachers", referenceHandler.GetTeachers).Methods(http.MethodGet)
	api.HandleFunc("/groups/by-year", referenceHandler.GetGroupsByYear).Methods(http.MethodGet)
	api.HandleFunc("/groups", referenceHandler.GetGroups).Methods(http.MethodGet)
	api.HandleFunc("/rooms", referenceHandler.GetRooms).Methods(http.MethodGet)
	api.HandleFunc("/subject-plans", referenceHandler.GetSubjectPlans).Methods(http.MethodGet)
	api.HandleFunc("/buildings", referenceHandler.GetBuildings).Methods(http.MethodGet)
	api.HandleFunc("/departments", referenceHandler.GetDepartments).Methods(http.MethodGet)

	// Справочники — запись
	api.HandleFunc("/buildings", refWriteHandler.CreateBuilding).Methods(http.MethodPost)
	api.HandleFunc("/buildings/{id}", refWriteHandler.UpdateBuilding).Methods(http.MethodPut)
	api.HandleFunc("/buildings/{id}", refWriteHandler.DeleteBuilding).Methods(http.MethodDelete)

	api.HandleFunc("/departments", refWriteHandler.CreateDepartment).Methods(http.MethodPost)
	api.HandleFunc("/departments/{id}", refWriteHandler.UpdateDepartment).Methods(http.MethodPut)
	api.HandleFunc("/departments/{id}", refWriteHandler.DeleteDepartment).Methods(http.MethodDelete)

	api.HandleFunc("/rooms", refWriteHandler.CreateRoom).Methods(http.MethodPost)
	api.HandleFunc("/rooms/{id}", refWriteHandler.UpdateRoom).Methods(http.MethodPut)
	api.HandleFunc("/rooms/{id}", refWriteHandler.DeleteRoom).Methods(http.MethodDelete)

	api.HandleFunc("/groups", refWriteHandler.CreateGroup).Methods(http.MethodPost)
	api.HandleFunc("/groups/{id}", refWriteHandler.UpdateGroup).Methods(http.MethodPut)
	api.HandleFunc("/groups/{id}", refWriteHandler.DeleteGroup).Methods(http.MethodDelete)

	api.HandleFunc("/teachers", refWriteHandler.CreateTeacher).Methods(http.MethodPost)
	api.HandleFunc("/teachers/{id}", refWriteHandler.UpdateTeacher).Methods(http.MethodPut)
	api.HandleFunc("/teachers/{id}", refWriteHandler.DeleteTeacher).Methods(http.MethodDelete)

	api.HandleFunc("/subject-plans", refWriteHandler.CreateSubjectPlan).Methods(http.MethodPost)
	api.HandleFunc("/subject-plans/{id}", refWriteHandler.UpdateSubjectPlan).Methods(http.MethodPut)
	api.HandleFunc("/subject-plans/{id}", refWriteHandler.DeleteSubjectPlan).Methods(http.MethodDelete)

	return router
}
