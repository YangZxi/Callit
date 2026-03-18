package db

import (
	"callit/internal/model"

	"gorm.io/gorm"
)

func runMigrations(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Worker{},
		&model.WorkerLog{},
		&model.AppConfigEntry{},
	)
}
