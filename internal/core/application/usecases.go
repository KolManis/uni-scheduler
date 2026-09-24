// Package application — сценарии работы с расписанием (слой приложения гексагональной
// архитектуры). Каждый сценарий — отдельный пакет:
//
//	commands/ — меняют данные: command.go (что сделать, NewCommand проверяет вход) и
//	            command_handler.go (как сделать, Handler.Handle)
//	  generateschedule    составить расписание
//	  generateallmethods  составить всеми методами улучшения для сравнения
//	  moveassignment      перенести пару вручную
//	  pinassignment       закрепить пару или снять закрепление
//	  deleteschedule      удалить расписание
//	  importexcel         загрузить справочники из Excel
//
//	queries/  — только читают: query.go и query_handler.go
//	  getschedule         расписание целиком
//	  listschedules       список расписаний
//	  evaluateschedule    из чего складывается score и показатели качества
//	  moveoptions         куда можно перенести пару и что изменится
//	  checkinput          ошибки в данных до генерации
//
// Общее для нескольких сценариев: generation/ (подготовка и запуск солвера), rules/
// (жёсткие ограничения и расчёт нагрузки). Сценарии зависят только от domain, ports и
// solver; адаптеры (HTTP, Postgres) подключаются через порты.
package application

import (
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/deleteschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/generateallmethods"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/generateschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/importexcel"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/moveassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/commands/pinassignment"
	"github.com/KolManis/uni-scheduler/internal/core/application/generation"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/checkinput"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/evaluateschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/getschedule"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/listschedules"
	"github.com/KolManis/uni-scheduler/internal/core/application/queries/moveoptions"
	"github.com/KolManis/uni-scheduler/internal/core/ports"
)

// UseCases — все сценарии, собранные с хранилищами. Адаптеры берут отсюда нужные.
type UseCases struct {
	GenerateSchedule   *generateschedule.Handler
	GenerateAllMethods *generateallmethods.Handler
	MoveAssignment     *moveassignment.Handler
	PinAssignment      *pinassignment.Handler
	DeleteSchedule     *deleteschedule.Handler
	ImportExcel        *importexcel.Handler

	GetSchedule      *getschedule.Handler
	ListSchedules    *listschedules.Handler
	EvaluateSchedule *evaluateschedule.Handler
	MoveOptions      *moveoptions.Handler
	CheckInput       *checkinput.Handler
}

// NewUseCases собирает сценарии поверх хранилищ.
func NewUseCases(input ports.InputRepository, output ports.OutputRepository, imports ports.ImportRepository) UseCases {
	generator := generation.NewGenerator(input, output)
	return UseCases{
		GenerateSchedule:   generateschedule.NewHandler(generator),
		GenerateAllMethods: generateallmethods.NewHandler(generator),
		MoveAssignment:     moveassignment.NewHandler(input, output),
		PinAssignment:      pinassignment.NewHandler(output),
		DeleteSchedule:     deleteschedule.NewHandler(output),
		ImportExcel:        importexcel.NewHandler(imports),

		GetSchedule:      getschedule.NewHandler(output),
		ListSchedules:    listschedules.NewHandler(output),
		EvaluateSchedule: evaluateschedule.NewHandler(input, output),
		MoveOptions:      moveoptions.NewHandler(input, output),
		CheckInput:       checkinput.NewHandler(input),
	}
}
