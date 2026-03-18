package model

import "time"

// AppConfigEntry 表示 app_config 表中的一条配置记录。
type AppConfigEntry struct {
	Key       string    `gorm:"column:key;primaryKey;type:text"`
	Value     string    `gorm:"column:value;type:text"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoCreateTime:false;autoUpdateTime:false"`
}

func (AppConfigEntry) TableName() string {
	return "app_config"
}
