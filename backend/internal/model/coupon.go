package model

import "time"

// Coupons 是 coupons 表实体（由 keystone-cli gen 脚手架自动生成）。
// 包含 GORM 基础字段：自增主键 ID + 自动时间戳 CreatedAt/UpdatedAt。
type Coupons struct {
	ID        uint64    `gorm:"primarykey" json:"id"` // 自增主键
	CreatedAt time.Time `json:"created_at"`           // 创建时间（GORM 自动填充）
	UpdatedAt time.Time `json:"updated_at"`           // 更新时间（GORM 自动填充）
}

// TableName 返回 coupons 表名。
func (Coupons) TableName() string {
	return "coupons"
}
