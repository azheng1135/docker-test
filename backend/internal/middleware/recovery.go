package middleware

import (
	"net/http"
	"runtime/debug"

	"keystonego/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 是全局 Panic 恢复中间件，应作为 Gin 的第一个中间件注册。
//
// 职责：
//  1. 捕获 Handler 链中任何未被 recover 的 panic
//  2. 记录完整的错误信息和堆栈到 Zap 日志（携带 TraceID）
//  3. 返回统一格式的 500 JSON 响应，避免客户端收到空响应或 HTML 错误页
//
// 注意：此中间件不会让服务崩溃，但 panic 说明存在代码缺陷，需要根据日志及时修复。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 从 Gin Context 获取 TraceID（Trace 中间件已设置）
				traceID, _ := c.Get("trace_id")
				traceStr, _ := traceID.(string)

				// 记录完整的 panic 信息和堆栈
				zap.L().Error("系统 Panic 捕获",
					zap.Any("error", err),
					zap.String("stack", string(debug.Stack())),
					zap.String("trace_id", traceStr),
				)

				// 返回统一错误格式，不暴露内部错误细节
				c.AbortWithStatusJSON(http.StatusOK, response.Response{
					Code:    response.ErrSystemError,
					Msg:     "服务器开小差了，请稍后再试",
					TraceID: traceStr,
					Data:    gin.H{},
				})
			}
		}()
		c.Next()
	}
}
