package config

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// backupCachePath 是主配置解析成功后写入的本地备份缓存路径。
// 当 bootstrap.yaml 损坏或不可读时，系统降级从此文件启动。
const backupCachePath = "config/backup_cache.yaml"

// InitConfig 加载并解析 bootstrap.yaml，建立热更新监听。
//
// 启动流程：
//  1. 使用 Viper 读取 bootstrap.yaml
//  2. 通过 DecryptHook 自动解密 ENC() 加密字段
//  3. 反序列化为 AppConfig 并写入 GlobalManager
//  4. 成功后写入 backup_cache.yaml 作为容灾备份
//  5. 启动 fsnotify 监听，文件变更时自动热加载
//
// 容灾策略：
//   - 主配置读取失败 → 尝试从 backup_cache.yaml 降级启动
//   - 备份配置也不可用 → panic 拒绝启动（fail-fast）
func InitConfig(bootstrapPath string) {
	v := viper.New()
	v.SetConfigFile(bootstrapPath)
	v.SetConfigType("yaml")

	// 第一步：读取主配置文件
	if err := v.ReadInConfig(); err != nil {
		zap.L().Warn("读取 bootstrap.yaml 失败，尝试备份缓存启动...", zap.Error(err))
		loadFromBackup()
		return
	}

	// 第二步：反序列化配置，同时通过 DecryptHook 解密敏感字段
	var cfg AppConfig
	if err := v.Unmarshal(&cfg, viper.DecodeHook(DecryptHook())); err != nil {
		zap.L().Warn("解析配置失败，尝试备份缓存启动...", zap.Error(err))
		loadFromBackup()
		return
	}

	// 第三步：写入全局配置管理器
	GlobalManager.Update(&cfg)

	// 第四步：写入本地备份缓存，供下次冷启动容灾
	if err := v.WriteConfigAs(backupCachePath); err != nil {
		zap.L().Warn("写入配置备份缓存失败", zap.Error(err))
	} else {
		zap.L().Info("配置已同步，本地备份缓存已更新", zap.String("path", backupCachePath))
	}

	// 第五步：启动文件变更监听，实现热更新
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		zap.L().Info("检测到配置文件变更，开始热加载...", zap.String("file", e.Name))
		var newCfg AppConfig
		if err := v.Unmarshal(&newCfg, viper.DecodeHook(DecryptHook())); err != nil {
			zap.L().Error("热加载配置失败，继续使用旧配置", zap.Error(err))
			return
		}

		// 关键开关变更时发出警告，便于运维追溯
		oldCfg := GlobalManager.Get()
		if oldCfg.SystemSwitches.MaintenanceMode != newCfg.SystemSwitches.MaintenanceMode {
			zap.L().Warn("系统维护模式开关变更",
				zap.Bool("old", oldCfg.SystemSwitches.MaintenanceMode),
				zap.Bool("new", newCfg.SystemSwitches.MaintenanceMode),
			)
		}

		// 原子替换全局配置
		GlobalManager.Update(&newCfg)
		// 同步更新备份缓存
		_ = v.WriteConfigAs(backupCachePath)
		zap.L().Info("配置热加载完成")
	})
}

// loadFromBackup 从本地备份缓存降级启动。
// 如果备份也不可用，直接 panic 拒绝启动（fail-fast 策略）。
func loadFromBackup() {
	fallback := viper.New()
	fallback.SetConfigFile(backupCachePath)
	if err := fallback.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("灾难：本地备份配置缓存不可用，拒绝启动！%v", err))
	}

	var cfg AppConfig
	if err := fallback.Unmarshal(&cfg, viper.DecodeHook(DecryptHook())); err != nil {
		panic(fmt.Sprintf("灾难：解析备份配置失败，拒绝启动！%v", err))
	}

	GlobalManager.Update(&cfg)
	zap.L().Warn("已通过本地备份缓存降级启动，部分功能可能受限")
}
