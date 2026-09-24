// Package migrations — схема базы данных. Файлы встроены в бинарник и применяются
// приложением при старте через goose (postgres.Migrate): отдельно накатывать не нужно.
package migrations

import "embed"

// FS — все файлы миграций. Номер в начале имени — версия для goose; 0002 нет
// (удалена как дубликат 0001), пропуск номера goose не мешает.
//
//go:embed *.sql
var FS embed.FS
