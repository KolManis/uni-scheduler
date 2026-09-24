package rest

import (
	"io"
	"net/http"

	"github.com/KolManis/uni-scheduler/internal/adapters/in/excel"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/importexcel"
)

// ImportHandler — переводчик HTTP (файл xlsx) → команда importexcel → HTTP.
type ImportHandler struct {
	importExcel *importexcel.Handler
}

func NewImportHandler(importExcel *importexcel.Handler) *ImportHandler {
	return &ImportHandler{importExcel: importExcel}
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

	data, warnings, err := excel.ParseReader(content)
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse xlsx: "+err.Error())
		return
	}

	cmd, err := importexcel.NewCommand(data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result, err := h.importExcel.Handle(r.Context(), cmd)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result.Warnings = append(result.Warnings, warnings...)
	writeJSONStatus(w, http.StatusOK, result)
}
