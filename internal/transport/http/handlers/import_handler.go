package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/importer"
)

type importService interface {
	ImportExcel(ctx context.Context, data *importer.ImportedData) (*importer.ImportResult, error)
}

// ImportHandler обрабатывает импорт данных из Excel.
type ImportHandler struct {
	svc importService
}

func NewImportHandler(svc importService) *ImportHandler {
	return &ImportHandler{svc: svc}
}

// ImportExcel godoc
// POST /api/v1/import/excel
// Content-Type: multipart/form-data, поле "file"
func (h *ImportHandler) ImportExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "cannot parse form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field 'file' required")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read file: "+err.Error())
		return
	}

	data, warnings, err := importer.ParseReader(content)
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse xlsx: "+err.Error())
		return
	}

	result, err := h.svc.ImportExcel(r.Context(), data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result.Warnings = append(result.Warnings, warnings...)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
