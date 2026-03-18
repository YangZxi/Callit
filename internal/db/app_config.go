package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"callit/internal/config"
	"callit/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AppConfigDAO struct {
	db *gorm.DB
}

func (dao *AppConfigDAO) getConfigs(ctx context.Context) (map[string]string, error) {
	var entries []model.AppConfigEntry
	if err := dao.db.WithContext(ctx).Find(&entries).Error; err != nil {
		return nil, err
	}

	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := strings.ToUpper(strings.TrimSpace(entry.Key))
		if key == "" {
			continue
		}
		out[key] = entry.Value
	}
	return out, nil
}

// GetConfigs 从数据库读取所有配置项，返回 kv。
func (dao *AppConfigDAO) GetConfigs(ctx context.Context) (map[string]string, error) {
	return dao.getConfigs(ctx)
}

// Sync 将数据库中的白名单配置(仅 AppConfig)同步到 cfg。
func (dao *AppConfigDAO) Sync(ctx context.Context, cfg *config.Config) error {
	items, err := dao.getConfigs(ctx)
	if err != nil {
		return err
	}
	for k, v := range items {
		cfg.SetAppConfigValue(k, v)
	}
	return nil
}

func (dao *AppConfigDAO) setConfig(ctx context.Context, key, value string) error {
	key = strings.ToUpper(strings.TrimSpace(key))
	entry := model.AppConfigEntry{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now().UTC(),
	}
	return dao.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&entry).Error
}

// SetConfig 写入数据库并更新本地 cfg（仅允许白名单 AppConfig）。
func (dao *AppConfigDAO) SetConfig(ctx context.Context, cfg *config.Config, key, value string) error {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !config.IsAppConfigKey(key) {
		return fmt.Errorf("不支持的配置项: %s", key)
	}
	if err := dao.setConfig(ctx, key, value); err != nil {
		return err
	}
	if ok := cfg.SetAppConfigValue(key, value); !ok {
		return errors.New("配置项写入成功，但应用失败")
	}
	return nil
}
