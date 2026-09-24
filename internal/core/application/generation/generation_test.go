package generation

import (
	"errors"
	"testing"

	"github.com/KolManis/uni-scheduler/internal/core/domain"
)

func TestRequestValidate_Defaults(t *testing.T) {
	req, err := Request{}.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if req.Name != "Untitled" || req.TimeoutSec != DefaultTimeoutSec {
		t.Errorf("значения по умолчанию не подставлены: %+v", req)
	}
}

func TestRequestValidate_UnknownConstruction(t *testing.T) {
	if _, err := (Request{SolverType: "subject"}).Validate(); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("ожидалась ErrInvalidInput, получено %v", err)
	}
}
