package utils

import (
	"fmt"
	"log/slog"
	"reflect"

	"gorm.io/gorm"
)

// InsertOrUpdate is a wrapper for GORM that insert a row to a table if the row not exist and update that row otherwise.
// It takes [*gorm.io/gorm.DB] and [*slog.Logger] as the db connection and the logger.
// It takes tableName as the name of the SQL table.
// v is the pointer to the struct that the data is parsed onto.
// query and args are the parameters those are parsed directly to [*gorm.io/gorm.DB.Where()]
func InsertOrUpdate(db *gorm.DB, logger *slog.Logger, table string, dest interface{}, query interface{}, args ...interface{}) error {
	var exists bool

	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("dest is not of type pointer but %s instead", v.Type().String())
	}

	elem := v.Elem().Interface()

	if err := db.Table(table).Select("count(*) > 0").Where(query, args...).Find(&exists).Error; err != nil {
		return err
	}

	if exists {
		if err := db.Table(table).Where(query, args...).First(dest).Error; err != nil {
			return err
		}

		logger.Warn(fmt.Sprintf("this row is already in %s", table), "row", elem)

		return nil
	}

	if err := db.Table(table).Where(query, args...).Create(dest).Error; err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("new row inserted into %s", table), "row", elem)

	return nil
}
