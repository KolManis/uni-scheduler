package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Границы сетки расписания: пары нумеруются с 1 по 6.
const (
	FirstPair = 1
	LastPair  = 6
)

// ErrInvalidTimeSlot — день или номер пары вне сетки расписания.
var ErrInvalidTimeSlot = errors.New("invalid time slot")

// TimeSlot — ячейка сетки расписания: день недели и номер пары.
//
// Объект-значение: неизменяемый, сравнивается по значению и годится как ключ map.
// Создаётся только через NewTimeSlot, поэтому некорректный слот («воскресенье, 9-я пара»)
// в системе существовать не может — раньше такой слот можно было собрать прямо из формы.
type TimeSlot struct {
	day     Day
	pairNum int
}

// NewTimeSlot создаёт слот и проверяет, что он лежит в сетке расписания.
func NewTimeSlot(day Day, pairNum int) (TimeSlot, error) {
	if !day.IsValid() {
		return TimeSlot{}, fmt.Errorf("%w: unknown day %q", ErrInvalidTimeSlot, day)
	}
	if pairNum < FirstPair || pairNum > LastPair {
		return TimeSlot{}, fmt.Errorf("%w: pair %d outside %d..%d", ErrInvalidTimeSlot, pairNum, FirstPair, LastPair)
	}
	return TimeSlot{day: day, pairNum: pairNum}, nil
}

// MustNewTimeSlot — как NewTimeSlot, но паникует на некорректном слоте.
// Только для заведомо корректных значений: перебора по сетке и литералов в тестах.
// Данные извне (формы, API, БД) — только через NewTimeSlot с обработкой ошибки.
func MustNewTimeSlot(day Day, pairNum int) TimeSlot {
	slot, err := NewTimeSlot(day, pairNum)
	if err != nil {
		panic(err)
	}
	return slot
}

func (s TimeSlot) Day() Day      { return s.day }
func (s TimeSlot) PairNum() int  { return s.pairNum }
func (s TimeSlot) String() string { return fmt.Sprintf("%s:%d", s.day, s.pairNum) }

func (s TimeSlot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Day     Day `json:"day"`
		PairNum int `json:"pair_num"`
	}{s.day, s.pairNum})
}

func (s *TimeSlot) UnmarshalJSON(data []byte) error {
	var raw struct {
		Day     Day `json:"day"`
		PairNum int `json:"pair_num"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	ts, err := NewTimeSlot(raw.Day, raw.PairNum)
	if err != nil {
		return err
	}
	*s = ts
	return nil
}
