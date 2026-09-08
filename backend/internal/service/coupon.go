package service

import (
	"context"

	"keystonego/internal/model"
	"keystonego/internal/repository"
)

// CouponsService 是 coupons 的业务逻辑层（由 keystone-cli gen 脚手架自动生成）。
// 当前为透传模式，业务逻辑可直接在方法体中扩展。
type CouponsService struct {
	repo *repository.CouponsRepository
}

// NewCouponsService 创建 CouponsService（Wire Provider）。
func NewCouponsService(repo *repository.CouponsRepository) *CouponsService {
	return &CouponsService{repo: repo}
}

// Create 创建优惠券（可在此添加业务校验逻辑）。
func (s *CouponsService) Create(ctx context.Context, m *model.Coupons) error {
	return s.repo.Create(ctx, m)
}

// GetByID 按 ID 查询优惠券。
func (s *CouponsService) GetByID(ctx context.Context, id uint64) (*model.Coupons, error) {
	return s.repo.GetByID(ctx, id)
}

// List 分页查询优惠券列表。
func (s *CouponsService) List(ctx context.Context, page, pageSize int) ([]model.Coupons, int64, error) {
	return s.repo.List(ctx, page, pageSize)
}

// Update 更新优惠券信息。
func (s *CouponsService) Update(ctx context.Context, m *model.Coupons) error {
	return s.repo.Update(ctx, m)
}

// Delete 删除优惠券。
func (s *CouponsService) Delete(ctx context.Context, id uint64) error {
	return s.repo.Delete(ctx, id)
}
