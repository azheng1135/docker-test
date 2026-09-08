// Package handler 提供 HTTP 控制层（Controller），处理请求解析、参数校验、响应返回。
//
// 设计原则：
//   - 仅做请求/响应转换，不包含业务逻辑
//   - 统一使用 response.ShouldBindJSON / response.Success / response.Fail
//   - 从 Gin Context 中提取当前用户信息（由 JWTAuth 中间件注入）
package handler

import (
	"fmt"
	"net/url"

	"keystonego/internal/service"
	"keystonego/pkg/config"
	"keystonego/pkg/response"
	"keystonego/pkg/token"

	"github.com/gin-gonic/gin"
)

// AuthHandler 是认证相关的 HTTP 控制器。
type AuthHandler struct {
	authSvc *service.AuthService
}

// NewAuthHandler 创建认证 Handler（Wire Provider）。
func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

// ---- 请求结构体定义 ----

type LoginReq struct {
	Username string `json:"username" binding:"required"` // 用户名
	Password string `json:"password" binding:"required"` // 密码
}

type RegisterReq struct {
	Username string `json:"username" binding:"required"` // 用户名
	Password string `json:"password" binding:"required"` // 密码
	Nickname string `json:"nickname"`                    // 昵称（可选）
	Email    string `json:"email"`                       // 邮箱（可选）
	Phone    string `json:"phone"`                       // 手机号（可选）
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"` // Refresh Token
}

type SendSMSReq struct {
	Phone string `json:"phone" binding:"required"` // 手机号
}

type SMSLoginReq struct {
	Phone string `json:"phone" binding:"required"` // 手机号
	Code  string `json:"code" binding:"required"`  // 短信验证码
}

type BindPhoneReq struct {
	Phone string `json:"phone" binding:"required"` // 手机号
	Code  string `json:"code" binding:"required"`  // 短信验证码
}

type UnbindReq struct {
	IdentityType string `json:"identity_type" binding:"required"` // 要解绑的认证类型
}

type ForgotPwdReq struct {
	Phone string `json:"phone" binding:"required"` // 手机号
}

type ResetPwdReq struct {
	Phone       string `json:"phone" binding:"required"`        // 手机号
	Code        string `json:"code" binding:"required"`         // 短信验证码
	NewPassword string `json:"new_password" binding:"required"` // 新密码
}

// ---- 基础认证 ----

// Login 用户名+密码登录。
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginReq
	if !response.ShouldBindJSON(c, &req) {
		return
	}

	pair, user, err := h.authSvc.Login(req.Username, req.Password)

	if err != nil {
		response.Fail(c, response.ErrUnauthorized, err.Error())
		return
	}

	response.Success(c, gin.H{
		"user_id":       user.ID,
		"username":      user.Username,
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

// Register 新用户注册。
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	// 检查系统是否开放注册开关
	if !config.GlobalManager.Get().SystemSwitches.EnableRegister {
		response.Fail(c, 403, "系统当前已暂停新用户注册，请联系管理员")
		return
	}

	var req RegisterReq
	if !response.ShouldBindJSON(c, &req) {
		return
	}

	user, err := h.authSvc.Register(req.Username, req.Password, req.Nickname, req.Email, req.Phone)
	if err != nil {
		response.Fail(c, response.ErrUserAlreadyExists, err.Error())
		return
	}

	response.Success(c, gin.H{
		"user_id":  user.ID,
		"username": user.Username,
	})
}

// Refresh 使用 Refresh Token 刷新 Access Token。
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请提供 Refresh Token")
		return
	}

	pair, err := h.authSvc.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		response.Fail(c, response.ErrTokenExpired, err.Error())
		return
	}

	response.Success(c, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

// Logout 登出，将当前 Access Token 的 JTI 加入 Redis 黑名单。
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	jti, exists := c.Get("current_token_jti")
	if !exists {
		response.Fail(c, response.ErrUnauthorized, "未登录")
		return
	}
	jtiStr, _ := jti.(string)

	// 将 token 标记为已注销，TTL 等于 AccessToken 有效期，到期自动清理
	token.BlacklistToken(jtiStr, token.AccessTokenTTL)

	response.Success(c, gin.H{})
}

// ---- 多渠道登录 ----

// SendSMS 发送短信验证码。
// POST /api/v1/auth/sms/send
func (h *AuthHandler) SendSMS(c *gin.Context) {
	var req SendSMSReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请输入手机号")
		return
	}
	if err := h.authSvc.SendSMSCode(req.Phone); err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"msg": "验证码已发送（开发模式: 123456）"})
}

// SMSLogin 短信验证码登录。
// POST /api/v1/auth/sms/login
func (h *AuthHandler) SMSLogin(c *gin.Context) {
	var req SMSLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请输入手机号和验证码")
		return
	}

	pair, user, err := h.authSvc.LoginBySMS(req.Phone, req.Code)
	if err != nil {
		response.Fail(c, response.ErrUnauthorized, err.Error())
		return
	}

	response.Success(c, gin.H{
		"user_id":       user.ID,
		"username":      user.Username,
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

// GitHubOAuth 跳转到 GitHub 授权页面。
// GET /api/v1/auth/oauth/github
func (h *AuthHandler) GitHubOAuth(c *gin.Context) {
	authURL, err := h.authSvc.GetGitHubAuthURL()
	if err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	c.Redirect(302, authURL)
}

// GitHubCallback 处理 GitHub OAuth 回调，登录或自动注册后重定向到前端。
// GET /api/v1/auth/oauth/github/callback?code=xxx
func (h *AuthHandler) GitHubCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		response.Fail(c, response.ErrParamInvalid, "缺少授权码")
		return
	}

	pair, user, err := h.authSvc.LoginByGitHub(code)
	if err != nil {
		response.Fail(c, response.ErrUnauthorized, err.Error())
		return
	}

	// 将 Token 作为 URL Fragment 参数传递到前端（不经过服务端日志）
	frontendURL := h.authSvc.GetFrontendURL()
	redirectURL := fmt.Sprintf("%s/auth/oauth/callback#access_token=%s&refresh_token=%s&expires_in=%d&user_id=%d&username=%s",
		frontendURL, pair.AccessToken, pair.RefreshToken, pair.ExpiresIn, user.ID, url.QueryEscape(user.Username))
	c.Redirect(302, redirectURL)
}

// ---- 账户绑定 ----

// GetChannels 获取当前用户的所有认证渠道。
// GET /api/v1/auth/channels
func (h *AuthHandler) GetChannels(c *gin.Context) {
	userID, _ := c.Get("current_user_id")
	channels, err := h.authSvc.GetUserChannels(userID.(uint64))
	if err != nil {
		response.Fail(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, channels)
}

// BindPhone 绑定手机号到当前账户。
// POST /api/v1/auth/bind/phone
func (h *AuthHandler) BindPhone(c *gin.Context) {
	var req BindPhoneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请输入手机号和验证码")
		return
	}
	userID, _ := c.Get("current_user_id")
	if err := h.authSvc.BindPhone(userID.(uint64), req.Phone, req.Code); err != nil {
		response.Fail(c, response.ErrParamInvalid, err.Error())
		return
	}
	response.Success(c, gin.H{})
}

// Unbind 解绑指定的认证渠道。
// POST /api/v1/auth/unbind
func (h *AuthHandler) Unbind(c *gin.Context) {
	var req UnbindReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请指定要解绑的登录方式")
		return
	}
	userID, _ := c.Get("current_user_id")
	if err := h.authSvc.UnbindChannel(userID.(uint64), req.IdentityType); err != nil {
		response.Fail(c, response.ErrParamInvalid, err.Error())
		return
	}
	response.Success(c, gin.H{})
}

// ---- 找回密码 ----

// ForgotPassword 向已注册手机号发送找回密码验证码。
// POST /api/v1/auth/password/forgot
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请输入手机号")
		return
	}
	if err := h.authSvc.ForgotPassword(req.Phone); err != nil {
		response.Fail(c, response.ErrParamInvalid, err.Error())
		return
	}
	response.Success(c, gin.H{"msg": "验证码已发送（开发模式: 123456）"})
}

// ResetPassword 验证短信验证码后重置密码。
// POST /api/v1/auth/password/reset
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrParamInvalid, "请填写完整信息")
		return
	}
	if err := h.authSvc.ResetPassword(req.Phone, req.Code, req.NewPassword); err != nil {
		response.Fail(c, response.ErrParamInvalid, err.Error())
		return
	}
	response.Success(c, gin.H{"msg": "密码重置成功，请重新登录"})
}
