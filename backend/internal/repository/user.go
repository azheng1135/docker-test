package repository

import (
	"keystonego/internal/model"

	"gorm.io/gorm"
)

// UserRepository 是用户数据访问层，继承 BaseRepository 的通用 CRUD 能力。
type UserRepository struct {
	*BaseRepository[model.User] // 嵌入泛型基类，自动获得 Create/Update/Delete/FindByID/List
	db *gorm.DB                 // 保留 db 引用，用于扩展自定义查询
}

// NewUserRepository 创建用户 Repository（Wire Provider）。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		BaseRepository: NewBaseRepository[model.User](db),
		db:             db,
	}
}

// FindByUsername 按用户名精确查询。
func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

// UpdateStatus 更新用户启用/禁用状态。
func (r *UserRepository) UpdateStatus(id uint64, status int8) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

// UpdatePassword 更新用户密码哈希。
func (r *UserRepository) UpdatePassword(id uint64, password string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("password", password).Error
}

// UpdateAvatar 更新用户头像 URL。
func (r *UserRepository) UpdateAvatar(id uint64, avatarURL string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("avatar", avatarURL).Error
}

// AssignRoles 替换用户的角色分配（先清空再设置）。
// 使用 GORM Association Replace，自动管理 user_roles 中间表。
func (r *UserRepository) AssignRoles(userID uint64, roleIDs []uint64) error {
	var user model.User
	user.ID = userID

	var roles []model.Role
	for _, rid := range roleIDs {
		roles = append(roles, model.Role{BaseModel: model.BaseModel{ID: rid}})
	}

	return r.db.Model(&user).Association("Roles").Replace(roles)
}

// GetUserRoles 获取用户拥有的所有角色（预加载 Roles 关联）。
func (r *UserRepository) GetUserRoles(userID uint64) ([]model.Role, error) {
	var user model.User
	if err := r.db.Preload("Roles").First(&user, userID).Error; err != nil {
		return nil, err
	}
	return user.Roles, nil
}

// ListWithKeyword 分页 + 关键词搜索用户列表。
// keyword 同时匹配用户名、昵称、邮箱字段。
func (r *UserRepository) ListWithKeyword(keyword string, page, pageSize int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	q := r.db.Model(&model.User{})
	if keyword != "" {
		// LIKE 模糊匹配多字段
		q = q.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 先查总数
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := q.Offset(offset).Limit(pageSize).Order("id DESC").Find(&users).Error
	return users, total, err
}
