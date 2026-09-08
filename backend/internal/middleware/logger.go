package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger 是结构化请求日志中间件，记录每个 HTTP 请求的关键信息。
//
// 记录字段：
//   - trace_id: 链路追踪 ID（关联 GORM SQL 日志 + 业务日志）
//   - status: HTTP 响应状态码
//   - method + path + query: 请求方法和路径
//   - ip: 客户端 IP
//   - cost: 请求处理耗时
//   - user-agent: 客户端标识
//
// 日志级别统一为 Info，异常请求通过 status 过滤查询。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 先执行后续中间件和 Handler
		c.Next()

		// 请求结束后记录日志
		cost := time.Since(start)
		traceID, _ := c.Get(TraceIDKey)

		zap.L().Info("HTTP Request",
			zap.Any("trace_id", traceID),
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("cost", cost),
			zap.String("user-agent", c.Request.UserAgent()),
		)
	}
}
