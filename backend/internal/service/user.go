package service

import (
	"fmt"

	"keystonego/internal/model"
	"keystonego/internal/repository"
	casbinpkg "keystonego/pkg/casbin"
	"keystonego/pkg/utils"
)

// UserService 是用户管理业务逻辑服务。
type UserService struct {
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
}

// NewUserService 创建用户服务（Wire Provider）。
func NewUserService(userRepo *repository.UserRepository, roleRepo *repository.RoleRepository) *UserService {
	return &UserService{userRepo: userRepo, roleRepo: roleRepo}
}

// UserListReq 用户列表查询请求参数。
type UserListReq struct {
	Keyword  string `form:"keyword"`   // 模糊搜索关键词（匹配用户名、昵称、邮箱）
	Page     int    `form:"page"`      // 页码（从 1 开始）
	PageSize int    `form:"page_size"` // 每页条数
}

// AssignRolesReq 用户角色分配请求参数。
type AssignRolesReq struct {
	RoleIDs []uint64 `json:"role_ids" binding:"required"` // 角色 ID 列表
}

// List 分页 + 关键词查询用户列表。
func (s *UserService) List(req UserListReq) ([]model.User, int64, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	return s.userRepo.ListWithKeyword(req.Keyword, req.Page, req.PageSize)
}

// GetByID 按 ID 查询用户。
func (s *UserService) GetByID(id uint64) (*model.User, error) {
	return s.userRepo.FindByID(id)
}

// Create 创建新用户（默认密码 123456，状态启用）。
func (s *UserService) Create(req *model.User) error {
	hashedPwd, _ := utils.HashPassword("123456")
	req.Password = hashedPwd
	req.Status = 1
	return s.userRepo.Create(req)
}

// Update 更新用户信息（仅更新有值的字段）。
func (s *UserService) Update(id uint64, req *model.User) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Password != "" {
		hashedPwd, _ := utils.HashPassword(req.Password)
		user.Password = hashedPwd
	}
	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	return s.userRepo.Update(user)
}

// Delete 删除用户。
func (s *UserService) Delete(id uint64) error {
	return s.userRepo.Delete(id)
}

// UpdateStatus 更新用户启用/禁用状态。
func (s *UserService) UpdateStatus(id uint64, status int8) error {
	return s.userRepo.UpdateStatus(id, status)
}

// UpdatePassword 更新用户密码（需传入新密码明文，内部 bcrypt 哈希）。
func (s *UserService) UpdatePassword(id uint64, password string) error {
	hashedPwd, _ := utils.HashPassword(password)
	return s.userRepo.UpdatePassword(id, hashedPwd)
}

// AssignRoles 为用户分配角色（先清除旧角色，再设置新角色，同步更新 Casbin 策略）。
func (s *UserService) AssignRoles(userID uint64, roleIDs []uint64) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}

	// 清除旧的 Casbin 用户-角色映射
	userIDStr := fmt.Sprintf("%d", user.ID)
	casbinpkg.DeletePermissionsForUser(userIDStr)

	// 分配新角色到 user_roles 表
	if err := s.userRepo.AssignRoles(userID, roleIDs); err != nil {
		return err
	}

	// 同步到 Casbin 策略（g 规则：user → role）
	for _, rid := range roleIDs {
		role, err := s.roleRepo.FindByID(rid)
		if err != nil {
			continue
		}
		casbinpkg.AddRoleForUser(userIDStr, role.Code)
	}

	casbinpkg.LoadPolicy()
	return nil
}

// GetRoles 获取用户拥有的所有角色。
func (s *UserService) GetRoles(userID uint64) ([]model.Role, error) {
	return s.userRepo.GetUserRoles(userID)
}

// UpdateProfile 用户自行修改个人信息（昵称、邮箱、手机）。
func (s *UserService) UpdateProfile(userID uint64, nickname, email, phone string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}
	if nickname != "" {
		user.Nickname = nickname
	}
	if email != "" {
		user.Email = email
	}
	if phone != "" {
		user.Phone = phone
	}
	return s.userRepo.Update(user)
}

// UpdateAvatar 更新用户头像 URL。
func (s *UserService) UpdateAvatar(userID uint64, avatarURL string) error {
	return s.userRepo.UpdateAvatar(userID, avatarURL)
}
