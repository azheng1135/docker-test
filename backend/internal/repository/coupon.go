package repository

import (
	"context"

	"gorm.io/gorm"
	"keystonego/internal/model"
)

// CouponsRepository 是 coupons 表的数据访问层（由 keystone-cli gen 脚手架自动生成）。
// 封装了标准 CRUD 操作，所有方法通过 ctx 传递上下文（支持 TraceID 追踪）。
type CouponsRepository struct {
	db *gorm.DB
}

// NewCouponsRepository 创建 CouponsRepository（Wire Provider）。
func NewCouponsRepository(db *gorm.DB) *CouponsRepository {
	return &CouponsRepository{db: db}
}

// Create 创建一条 coupons 记录。
func (r *CouponsRepository) Create(ctx context.Context, m *model.Coupons) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID 按主键 ID 查询单条记录。
func (r *CouponsRepository) GetByID(ctx context.Context, id uint64) (*model.Coupons, error) {
	var m model.Coupons
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// List 分页查询 coupons 列表，返回数据 + 总数。
func (r *CouponsRepository) List(ctx context.Context, page, pageSize int) ([]model.Coupons, int64, error) {
	var list []model.Coupons
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Coupons{})
	q.Count(&total)
	err := q.Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// Update 全量更新一条 coupons 记录（使用 Save，会更新所有字段）。
func (r *CouponsRepository) Update(ctx context.Context, m *model.Coupons) error {
	return r.db.WithContext(ctx).Save(m).Error
}

// Delete 按主键 ID 删除一条 coupons 记录。
func (r *CouponsRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&model.Coupons{}, id).Error
}
