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

	// Справочники
	api.HandleFunc("/teachers", referenceHandler.GetTeachers).Methods(http.MethodGet)
	api.HandleFunc("/groups", referenceHandler.GetGroups).Methods(http.MethodGet)
	api.HandleFunc("/rooms", referenceHandler.GetRooms).Methods(http.MethodGet)
	api.HandleFunc("/subject-plans", referenceHandler.GetSubjectPlans).Methods(http.MethodGet)

	return router
}
