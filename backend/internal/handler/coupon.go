package handler

import (
	"strconv"

	"keystonego/internal/model"
	"keystonego/internal/service"
	"keystonego/pkg/response"

	"github.com/gin-gonic/gin"
)

// CouponsHandler 是 coupons 的 HTTP 控制器（由 keystone-cli gen 脚手架自动生成）。
// 提供标准 RESTful CRUD 接口。
type CouponsHandler struct {
	svc *service.CouponsService
}

// NewCouponsHandler 创建 CouponsHandler（Wire Provider）。
func NewCouponsHandler(svc *service.CouponsService) *CouponsHandler {
	return &CouponsHandler{svc: svc}
}

// Create 创建优惠券。
// POST /api/v1/coupons
func (h *CouponsHandler) Create(c *gin.Context) {
	var m model.Coupons
	if !response.ShouldBindJSON(c, &m) {
		return
	}
	if err := h.svc.Create(c.Request.Context(), &m); err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, m)
}

// Get 按 ID 查询优惠券。
// GET /api/v1/coupons/:id
func (h *CouponsHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, response.ErrParamInvalid, "ID 无效")
		return
	}
	m, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, m)
}

// List 分页查询优惠券列表。
// GET /api/v1/coupons
func (h *CouponsHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list, "total": total})
}

// Update 更新优惠券。
// PUT /api/v1/coupons/:id
func (h *CouponsHandler) Update(c *gin.Context) {
	var m model.Coupons
	if !response.ShouldBindJSON(c, &m) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, response.ErrParamInvalid, "ID 无效")
		return
	}
	m.ID = id
	if err := h.svc.Update(c.Request.Context(), &m); err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除优惠券。
// DELETE /api/v1/coupons/:id
func (h *CouponsHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, response.ErrParamInvalid, "ID 无效")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}
