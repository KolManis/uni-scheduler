// Package app — сценарии работы с расписанием (слой приложения гексагональной архитектуры).
//
// Сценарии разделены на команды и запросы:
//
//	Команды — меняют данные:
//	  command_generate.go  GenerateSchedule, GenerateAllMethods — составить расписание
//	  command_move.go      MoveAssignment — перенести пару вручную
//	  command_pin.go       PinAssignment — закрепить пару или снять закрепление
//	  command_delete.go    DeleteSchedule — удалить расписание
//	  command_import.go    ImportExcel — загрузить справочники из Excel
//
//	Запросы — только читают:
//	  query_schedule.go      GetSchedule, ListSchedules, Breakdown, Quality
//	  query_move_options.go  MoveOptions — куда можно перенести пару
//	  query_check_input.go   CheckInput — ошибки в данных до генерации
//
// Общие проверки жёстких ограничений — в conflict.go, причины неразмещения — в
// unplaced_reasons.go. Слой зависит только от domain, ports и solver; адаптеры (HTTP,
// Postgres) подключаются через порты.
package app

import (
	"errors"

	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// ErrInvalidInput — неверные параметры команды или запроса (адаптер отвечает 400).
var ErrInvalidInput = errors.New("invalid input")

// Service — точка входа в сценарии. Хранит только порты: всё состояние — в хранилище.
type Service struct {
	inputRepo  ports.InputRepository
	outputRepo ports.OutputRepository
	importRepo ports.ImportRepository
}

func NewService(inputRepo ports.InputRepository, outputRepo ports.OutputRepository, importRepo ports.ImportRepository) *Service {
	return &Service{
		inputRepo:  inputRepo,
		outputRepo: outputRepo,
		importRepo: importRepo,
	}
}
