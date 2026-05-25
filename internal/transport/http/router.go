package http

import (
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/transport/http/handlers"
	"github.com/gorilla/mux"
)

func NewRouter(scheduleHandler *handlers.ScheduleHandler, excelHandler *handlers.ExcelHandler) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	// Health check
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods(http.MethodGet)

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()

	api.HandleFunc("/schedules/generate", scheduleHandler.Generate).Methods(http.MethodPost)
	api.HandleFunc("/schedules", scheduleHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/schedules/{id:[0-9]+}", scheduleHandler.GetByID).Methods(http.MethodGet)
	api.HandleFunc("/schedules/{id:[0-9]+}/excel", excelHandler.Export).Methods(http.MethodGet)

	return router
}
