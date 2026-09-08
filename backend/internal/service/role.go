package service

import (
	"keystonego/internal/model"
	"keystonego/internal/repository"
	casbinpkg "keystonego/pkg/casbin"
)

// RoleService 是角色管理业务逻辑服务。
type RoleService struct {
	roleRepo *repository.RoleRepository
	menuRepo *repository.MenuRepository
}

// NewRoleService 创建角色服务（Wire Provider）。
func NewRoleService(roleRepo *repository.RoleRepository, menuRepo *repository.MenuRepository) *RoleService {
	return &RoleService{roleRepo: roleRepo, menuRepo: menuRepo}
}

// List 角色列表查询（支持关键词搜索）。
func (s *RoleService) List(keyword string) ([]model.Role, error) {
	return s.roleRepo.ListWithKeyword(keyword)
}

// GetByID 按 ID 查询角色。
func (s *RoleService) GetByID(id uint64) (*model.Role, error) {
	return s.roleRepo.FindByID(id)
}

// Create 创建新角色（默认状态启用）。
func (s *RoleService) Create(role *model.Role) error {
	role.Status = 1
	return s.roleRepo.Create(role)
}

// Update 更新角色信息（部分更新，仅更新有值的字段）。
func (s *RoleService) Update(id uint64, updates map[string]interface{}) error {
	role, err := s.roleRepo.FindByID(id)
	if err != nil {
		return err
	}
	if v, ok := updates["name"]; ok {
		role.Name = v.(string)
	}
	if v, ok := updates["desc"]; ok {
		role.Desc = v.(string)
	}
	if v, ok := updates["code"]; ok {
		role.Code = v.(string)
	}
	return s.roleRepo.Update(role)
}

// Delete 级联删除角色（含关联数据和 Casbin 策略）。
func (s *RoleService) Delete(id uint64) error {
	return s.roleRepo.DeleteCascade(id)
}

// UpdateStatus 更新角色启用/禁用状态。
func (s *RoleService) UpdateStatus(id uint64, status int8) error {
	return s.roleRepo.UpdateStatus(id, status)
}

// AssignMenus 为角色分配菜单（替换式）。
func (s *RoleService) AssignMenus(roleID uint64, menuIDs []uint64) error {
	return s.roleRepo.AssignRoleMenus(roleID, menuIDs)
}

// GetMenus 获取角色拥有的菜单列表。
func (s *RoleService) GetMenus(roleID uint64) ([]model.Menu, error) {
	return s.roleRepo.GetRoleMenus(roleID)
}

// PermItem 是权限项定义，对应 Casbin 策略的 (obj, act) 两元组。
type PermItem struct {
	Path   string `json:"path"`   // API 路径
	Method string `json:"method"` // HTTP 方法
}

// AssignPermissions 为角色分配权限策略（先清除旧规则，再添加新规则）。
func (s *RoleService) AssignPermissions(roleID uint64, perms []PermItem) error {
	role, err := s.roleRepo.FindByID(roleID)
	if err != nil {
		return err
	}

	// 清除旧权限策略（p 规则中以角色 code 为 subject 的规则）
	casbinpkg.DeletePermissionsForUser(role.Code)

	// 添加新权限策略
	for _, p := range perms {
		casbinpkg.AddPolicy(role.Code, p.Path, p.Method)
	}

	casbinpkg.LoadPolicy()
	return nil
}
