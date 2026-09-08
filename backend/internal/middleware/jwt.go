package middleware

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"keystonego/pkg/casbin"
	"keystonego/pkg/response"
	"keystonego/pkg/token"

	"github.com/gin-gonic/gin"
)

// numericPathRegex 匹配路径中的数字 ID 段（如 /users/123 → /users/:id）
// 用于 Casbin 权限匹配时，将具体的资源 ID 归一化为路由模式。
var numericPathRegex = regexp.MustCompile(`/\d+`)

// JWTAuth 是 JWT 认证 + Casbin 鉴权中间件。
//
// 认证流程：
//  1. 提取 Authorization: Bearer <token> 头
//  2. 解析并验证 JWT 签名与有效期
//  3. 校验 Token 类型必须为 "access"（拒绝 Refresh Token 访问 API）
//  4. 检查 Token 的 JTI 是否在 Redis 黑名单中（已注销的 Token）
//  5. 将用户信息（ID、用户名、角色）注入 Gin Context
//
// 鉴权流程（认证通过后）：
//  1. 用原始路径 + 方法做 Casbin Enforce 检查
//  2. 若失败，将 /users/123 归一化为 /users/:id 后重试
//  3. 两次都失败则返回 403
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 第一步：提取 Bearer Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusOK, response.Response{
				Code:    response.ErrUnauthorized,
				Msg:     "未登录或 Token 格式错误",
				TraceID: "",
				Data:    gin.H{},
			})
			return
		}

		// 第二步：解析 JWT
		tokenStr := authHeader[7:] // 去掉 "Bearer " 前缀
		claims, err := token.ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, response.Response{
				Code:    response.ErrTokenExpired,
				Msg:     "Token 已过期或无效",
				TraceID: "",
				Data:    gin.H{},
			})
			return
		}

		// 第三步：只允许 Access Token 访问 API
		if claims.TokenType != "access" {
			c.AbortWithStatusJSON(http.StatusOK, response.Response{
				Code:    response.ErrUnauthorized,
				Msg:     "请使用 Access Token 访问",
				TraceID: "",
				Data:    gin.H{},
			})
			return
		}

		// 第四步：黑名单检查（已注销的 Token）
		if token.IsBlacklisted(claims.JTIShort) {
			c.AbortWithStatusJSON(http.StatusOK, response.Response{
				Code:    response.ErrTokenExpired,
				Msg:     "Token 已被注销",
				TraceID: "",
				Data:    gin.H{},
			})
			return
		}

		// 将用户信息注入 Gin Context，供后续 Handler 使用
		c.Set("current_user_id", claims.UserID)
		c.Set("current_username", claims.Username)
		c.Set("current_user_roles", claims.Roles)
		c.Set("current_token_jti", claims.JTIShort)

		// 第五步：Casbin 权限鉴权
		if casbin.Enforcer != nil {
			userIDStr := fmt.Sprintf("%d", claims.UserID)
			path := c.Request.URL.Path
			method := c.Request.Method

			// 先用原始路径鉴权
			hasAccess := false
			if ok, _ := casbin.Enforcer.Enforce(userIDStr, path, method); ok {
				hasAccess = true
			}

			// 原始路径失败时，尝试将数字 ID 归一化为 :id 后重试
			normalizedPath := numericPathRegex.ReplaceAllString(path, "/:id")
			if !hasAccess && normalizedPath != path {
				if ok, _ := casbin.Enforcer.Enforce(userIDStr, normalizedPath, method); ok {
					hasAccess = true
				}
			}

			if !hasAccess {
				c.AbortWithStatusJSON(http.StatusOK, response.Response{
					Code:    response.ErrForbidden,
					Msg:     "权限不足，拒绝访问",
					TraceID: "",
					Data:    gin.H{},
				})
				return
			}
		}

		c.Next()
	}
}
