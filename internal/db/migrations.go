package db

import (
	"callit/internal/model"

	"gorm.io/gorm"
)

func runMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Worker{},
		&model.WorkerLog{},
		&model.CronTask{},
		&model.AppConfigEntry{},
	); err != nil {
		return err
	}
	return nil
}
