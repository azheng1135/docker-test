// Package service 提供业务逻辑层（Service Layer），编排 Repository 操作与跨模块调用。
//
// 职责边界：
//   - 业务规则校验（账户状态、密码匹配、频率限制）
//   - 跨 Repository 的事务性操作编排
//   - 第三方服务调用编排（SMS、OAuth）
//   - Token 生成与刷新
//   - 不在 Service 层直接操作数据库（通过 Repository 完成）
package service

import (
	"errors"
	"fmt"

	"keystonego/internal/model"
	"keystonego/internal/repository"
	"keystonego/pkg/casbin"
	myoauth "keystonego/pkg/oauth"
	"keystonego/pkg/sms"
	"keystonego/pkg/token"
	"keystonego/pkg/utils"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 全局第三方服务实例（由 main.go 初始化后注入）。
var (
	SMSSvc      *sms.SMSService       // 短信验证码服务
	GitHubOAuth *myoauth.GitHubOAuth  // GitHub OAuth 客户端
)

// InitMultiAuth 初始化多渠道认证依赖（main.go 中调用）。
func InitMultiAuth(smsSvc *sms.SMSService, githubOAuth *myoauth.GitHubOAuth) {
	SMSSvc = smsSvc
	GitHubOAuth = githubOAuth
}

// AuthService 是认证业务逻辑服务，涵盖登录、注册、Token 刷新、多渠道认证。
type AuthService struct {
	userRepo     *repository.UserRepository     // 用户数据
	roleRepo     *repository.RoleRepository     // 角色数据
	userAuthRepo *repository.UserAuthRepository // 多渠道认证凭据
	db           *gorm.DB                        // 数据库实例（用于事务和复杂查询）
}

// NewAuthService 创建认证服务（Wire Provider）。
func NewAuthService(userRepo *repository.UserRepository, roleRepo *repository.RoleRepository, db *gorm.DB) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		userAuthRepo: repository.NewUserAuthRepository(db),
		db:           db,
	}
}

// Login 用户名+密码登录。
// 校验顺序：用户存在 → 账号启用 → 密码匹配 → 获取角色 → 生成 TokenPair。
func (s *AuthService) Login(username, password string) (*token.TokenPair, *model.User, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 统一返回模糊错误提示，防止用户名枚举攻击
			return nil, nil, errors.New("用户名或密码错误")
		}
		return nil, nil, err
	}

	if user.Status == 0 {
		return nil, nil, errors.New("账户已被禁用")
	}

	if !utils.CheckPasswordHash(password, user.Password) {
		return nil, nil, errors.New("用户名或密码错误")
	}

	// 获取用户角色编码列表，写入 JWT Claims
	roles, _ := s.userRepo.GetUserRoles(user.ID)
	var roleNames []string
	for _, r := range roles {
		roleNames = append(roleNames, r.Code)
	}

	pair, err := token.GenerateTokenPair(user.ID, user.Username, roleNames)
	if err != nil {
		return nil, nil, fmt.Errorf("生成 Token 失败: %w", err)
	}

	return pair, user, nil
}

// RefreshAccessToken 使用 Refresh Token 换取新的 TokenPair。
// 会验证用户的当前状态（是否被禁用），确保被封禁用户无法刷新 Token。
func (s *AuthService) RefreshAccessToken(refreshTokenStr string) (*token.TokenPair, error) {
	// 解析并校验 Refresh Token
	claims, err := token.ParseToken(refreshTokenStr)
	if err != nil {
		return nil, errors.New("Refresh Token 无效或已过期")
	}

	if claims.TokenType != "refresh" {
		return nil, errors.New("请使用 Refresh Token 刷新")
	}

	if token.IsBlacklisted(claims.JTIShort) {
		return nil, errors.New("Refresh Token 已被注销")
	}

	// 验证用户当前状态
	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	if user.Status == 0 {
		return nil, errors.New("账户已被禁用")
	}

	roles, _ := s.userRepo.GetUserRoles(user.ID)
	var roleNames []string
	for _, r := range roles {
		roleNames = append(roleNames, r.Code)
	}

	return token.GenerateTokenPair(user.ID, user.Username, roleNames)
}

// Register 用户名+密码注册。
// 密码经 bcrypt 哈希后存储，默认状态为启用。
func (s *AuthService) Register(username, password, nickname, email, phone string) (*model.User, error) {
	// 检查用户名唯一性
	if _, err := s.userRepo.FindByUsername(username); err == nil {
		return nil, errors.New("用户名已存在")
	}

	hashedPwd, err := utils.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: username,
		Password: hashedPwd,
		Nickname: nickname,
		Email:    email,
		Phone:    phone,
		Status:   1, // 默认启用
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

// InitAdmin 初始化系统基础数据：admin/user 角色、默认权限策略、admin 用户。
// 使用 FirstOrCreate 确保多次调用幂等，适合在服务启动时执行。
func (s *AuthService) InitAdmin() error {
	// 创建 admin 角色（超级管理员）
	adminRole, err := s.roleRepo.FindByCode("admin")
	if err != nil {
		adminRole = &model.Role{Name: "超级管理员", Code: "admin", Desc: "系统超级管理员", Status: 1}
		if err := s.roleRepo.Create(adminRole); err != nil {
			return fmt.Errorf("创建 admin 角色失败: %w", err)
		}
	}

	// 创建 user 角色（普通注册用户）
	userRole, err := s.roleRepo.FindByCode("user")
	if err != nil {
		userRole = &model.Role{Name: "普通用户", Code: "user", Desc: "普通注册用户", Status: 1}
		if err := s.roleRepo.Create(userRole); err != nil {
			return fmt.Errorf("创建 user 角色失败: %w", err)
		}
	}

	// 为 user 角色配置基础权限（个人信息、菜单查看等）
	userPerms := []struct{ path, method string }{
		{"/api/v1/auth/logout", "POST"},
		{"/api/v1/auth/channels", "GET"},
		{"/api/v1/auth/bind/phone", "POST"},
		{"/api/v1/auth/unbind", "POST"},
		{"/api/v1/user/info", "GET"},
		{"/api/v1/user/menus", "GET"},
		{"/api/v1/user/profile", "GET"},
		{"/api/v1/user/profile", "PUT"},
		{"/api/v1/user/avatar", "POST"},
	}
	for _, p := range userPerms {
		casbin.AddPolicy("user", p.path, p.method)
	}
	casbin.LoadPolicy()

	// 创建 admin 用户（默认密码 123456）
	adminUser, err := s.userRepo.FindByUsername("admin")
	if err != nil {
		hashedPwd, _ := utils.HashPassword("123456")
		adminUser = &model.User{
			Username: "admin",
			Password: hashedPwd,
			Nickname: "管理员",
			Status:   1,
			IsAdmin:  true,
		}
		if err := s.userRepo.Create(adminUser); err != nil {
			return fmt.Errorf("创建 admin 用户失败: %w", err)
		}
	}

	// 分配 admin 角色给 admin 用户
	if err := s.userRepo.AssignRoles(adminUser.ID, []uint64{adminRole.ID}); err != nil {
		return fmt.Errorf("分配角色失败: %w", err)
	}

	// 同步 Casbin：admin 角色拥有所有 API 权限
	userIDStr := fmt.Sprintf("%d", adminUser.ID)
	casbin.AddRoleForUser(userIDStr, "admin")
	casbin.AddPolicy("admin", "*", "*") // admin 拥有所有接口权限
	casbin.LoadPolicy()

	// 为 admin 用户创建密码登录认证记录
	auth := &model.UserAuth{
		UserID:       adminUser.ID,
		IdentityType: "password",
		Identifier:   adminUser.Username,
		Credential:   adminUser.Password,
	}
	s.db.Where("user_id = ? AND identity_type = ?", adminUser.ID, "password").FirstOrCreate(auth)

	// 数据修复：为所有无角色的已有用户分配 user 角色
	var allUsers []model.User
	s.db.Find(&allUsers)
	for _, u := range allUsers {
		existingRoles, _ := s.userRepo.GetUserRoles(u.ID)
		if len(existingRoles) == 0 {
			if ur, err := s.roleRepo.FindByCode("user"); err == nil {
				s.userRepo.AssignRoles(u.ID, []uint64{ur.ID})
				casbin.AddRoleForUser(fmt.Sprintf("%d", u.ID), "user")
			}
		}
	}
	casbin.LoadPolicy()

	return nil
}

// ---- 多渠道登录支持 ----

// SendSMSCode 向手机号发送短信验证码。
func (s *AuthService) SendSMSCode(phone string) error {
	if SMSSvc == nil {
		return errors.New("短信服务未初始化")
	}
	return SMSSvc.SendCode(phone)
}

// LoginBySMS 短信验证码登录（含无感自动注册）。
// 流程：验证码校验 → 查找 phone 认证记录 → 不存在则自动创建用户 → 生成 Token。
func (s *AuthService) LoginBySMS(phone, code string) (*token.TokenPair, *model.User, error) {
	if SMSSvc == nil {
		return nil, nil, errors.New("短信服务未初始化")
	}
	if !SMSSvc.VerifyCode(phone, code) {
		return nil, nil, errors.New("验证码错误或已过期")
	}

	// 查找已有认证记录
	auth, err := s.userAuthRepo.FindByIdentity("phone", phone)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		// 无感注册：自动创建新用户（用户名 u_{phone}，确保唯一）
		user := &model.User{
			Username: "u_" + phone,
			Nickname: phone,
			Phone:    phone,
			Status:   1,
		}
		baseName := user.Username
		for i := 1; ; i++ {
			if _, err := s.userRepo.FindByUsername(user.Username); err != nil {
				break // 用户名可用
			}
			user.Username = fmt.Sprintf("%s_%d", baseName, i)
		}
		if err := s.userRepo.Create(user); err != nil {
			return nil, nil, fmt.Errorf("自动注册失败: %w", err)
		}
		auth = &model.UserAuth{UserID: user.ID, IdentityType: "phone", Identifier: phone}
		s.userAuthRepo.Create(auth)

		// 自动分配 user 角色
		if userRole, err := s.roleRepo.FindByCode("user"); err == nil {
			s.userRepo.AssignRoles(user.ID, []uint64{userRole.ID})
			casbin.AddRoleForUser(fmt.Sprintf("%d", user.ID), "user")
			casbin.LoadPolicy()
		}

		zap.L().Info("SMS 无感注册", zap.Uint64("user_id", user.ID), zap.String("phone", phone))
		pair, _ := token.GenerateTokenPair(user.ID, user.Username, []string{"user"})
		return pair, user, nil
	}

	// 已有用户：检查状态后签发 Token
	user, err := s.userRepo.FindByID(auth.UserID)
	if err != nil {
		return nil, nil, errors.New("用户不存在")
	}
	if user.Status == 0 {
		return nil, nil, errors.New("账户已被禁用")
	}

	pair, err := token.GenerateTokenPair(user.ID, user.Username, []string{})
	if err != nil {
		return nil, nil, fmt.Errorf("生成 Token 失败: %w", err)
	}
	return pair, user, nil
}

// GetGitHubAuthURL 获取 GitHub OAuth 授权页面 URL。
func (s *AuthService) GetGitHubAuthURL() (string, error) {
	if GitHubOAuth == nil {
		return "", errors.New("GitHub OAuth 未配置")
	}
	return GitHubOAuth.GetAuthURL("keystone"), nil
}

// LoginByGitHub GitHub OAuth 登录（含无感自动注册）。
// 流程：code 换 AccessToken → 获取 GitHub 用户信息 → 查找/创建本站用户 → 生成 Token。
func (s *AuthService) LoginByGitHub(code string) (*token.TokenPair, *model.User, error) {
	if GitHubOAuth == nil {
		return nil, nil, errors.New("GitHub OAuth 未配置")
	}

	// 用授权码换取 Access Token
	accessToken, err := GitHubOAuth.ExchangeCode(code)
	if err != nil {
		return nil, nil, fmt.Errorf("GitHub 授权失败: %w", err)
	}

	// 获取 GitHub 用户信息
	ghUser, err := GitHubOAuth.GetUser(accessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("获取 GitHub 用户信息失败: %w", err)
	}

	// 查找是否已有绑定记录
	identifier := fmt.Sprintf("github_%d", ghUser.ID)
	auth, err := s.userAuthRepo.FindByIdentity("github", identifier)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		// 自动注册（用户名 gh_{GitHub用户名}）
		username := "gh_" + ghUser.Login
		baseName := username
		for i := 1; ; i++ {
			if _, err := s.userRepo.FindByUsername(username); err != nil {
				break
			}
			username = fmt.Sprintf("%s_%d", baseName, i)
		}
		user := &model.User{
			Username: username,
			Nickname: ghUser.Name,
			Avatar:   ghUser.AvatarURL,
			Status:   1,
		}
		if user.Nickname == "" {
			user.Nickname = ghUser.Login
		}
		if err := s.userRepo.Create(user); err != nil {
			return nil, nil, fmt.Errorf("自动注册失败: %w", err)
		}
		auth = &model.UserAuth{UserID: user.ID, IdentityType: "github", Identifier: identifier, Credential: accessToken}
		s.userAuthRepo.Create(auth)

		if userRole, err := s.roleRepo.FindByCode("user"); err == nil {
			s.userRepo.AssignRoles(user.ID, []uint64{userRole.ID})
			casbin.AddRoleForUser(fmt.Sprintf("%d", user.ID), "user")
			casbin.LoadPolicy()
		}

		zap.L().Info("GitHub OAuth 自动注册", zap.Uint64("user_id", user.ID), zap.String("github_user", ghUser.Login))
		pair, _ := token.GenerateTokenPair(user.ID, user.Username, []string{"user"})
		return pair, user, nil
	}

	// 已有绑定用户：检查状态并更新头像
	user, err := s.userRepo.FindByID(auth.UserID)
	if err != nil {
		return nil, nil, errors.New("用户不存在")
	}
	if user.Status == 0 {
		return nil, nil, errors.New("账户已被禁用")
	}

	if ghUser.AvatarURL != "" && user.Avatar != ghUser.AvatarURL {
		user.Avatar = ghUser.AvatarURL
		s.db.Save(user)
	}

	roles, _ := s.userRepo.GetUserRoles(user.ID)
	var roleNames []string
	for _, r := range roles {
		roleNames = append(roleNames, r.Code)
	}
	if len(roleNames) == 0 {
		roleNames = []string{"user"}
	}

	pair, err := token.GenerateTokenPair(user.ID, user.Username, roleNames)
	if err != nil {
		return nil, nil, fmt.Errorf("生成 Token 失败: %w", err)
	}
	return pair, user, nil
}

// ---- 账户绑定管理 ----

// GetUserChannels 获取用户的所有认证渠道。
func (s *AuthService) GetUserChannels(userID uint64) ([]model.UserAuth, error) {
	return s.userAuthRepo.ListByUserID(userID)
}

// BindPhone 为用户绑定手机号（需短信验证码校验）。
func (s *AuthService) BindPhone(userID uint64, phone, code string) error {
	if SMSSvc == nil {
		return errors.New("短信服务未初始化")
	}
	if !SMSSvc.VerifyCode(phone, code) {
		return errors.New("验证码错误或已过期")
	}

	// 手机号不可重复绑定
	_, err := s.userAuthRepo.FindByIdentity("phone", phone)
	if err == nil {
		return errors.New("该手机号已绑定其他账户")
	}

	auth := &model.UserAuth{UserID: userID, IdentityType: "phone", Identifier: phone}
	return s.userAuthRepo.Create(auth)
}

// UnbindChannel 解绑认证渠道（至少保留一种登录方式）。
func (s *AuthService) UnbindChannel(userID uint64, identityType string) error {
	count, _ := s.userAuthRepo.ListByUserID(userID)
	if len(count) <= 1 {
		return errors.New("至少保留一种登录方式")
	}
	return s.userAuthRepo.DeleteByType(userID, identityType)
}

// GetFrontendURL 返回前端地址（OAuth 成功后的跳转目标）。
func (s *AuthService) GetFrontendURL() string {
	if GitHubOAuth == nil {
		return "http://localhost:3001"
	}
	return GitHubOAuth.GetFrontendURL()
}

// ---- 找回密码 ----

// ForgotPassword 向已注册手机号发送找回密码验证码。
func (s *AuthService) ForgotPassword(phone string) error {
	if SMSSvc == nil {
		return errors.New("短信服务未初始化")
	}
	// 校验手机号已注册
	_, err := s.userAuthRepo.FindByIdentity("phone", phone)
	if err != nil {
		return errors.New("该手机号未注册")
	}
	return SMSSvc.SendCode(phone)
}

// ResetPassword 验证短信验证码后重置密码。
func (s *AuthService) ResetPassword(phone, code, newPassword string) error {
	if SMSSvc == nil {
		return errors.New("短信服务未初始化")
	}
	if !SMSSvc.VerifyCode(phone, code) {
		return errors.New("验证码错误或已过期")
	}

	auth, err := s.userAuthRepo.FindByIdentity("phone", phone)
	if err != nil {
		return errors.New("该手机号未绑定任何账户")
	}

	hashedPwd, err := utils.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePassword(auth.UserID, hashedPwd)
}
