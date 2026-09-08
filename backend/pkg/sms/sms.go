// Package sms 提供短信验证码的发送与校验能力。
//
// 当前为开发模式实现，验证码固定为 "123456"，通过 Zap 日志输出。
// 生产环境需接入阿里云/腾讯云等 SMS 服务商，替换 SendCode 中的发送逻辑。
//
// 安全措施：
//   - 60 秒发送频率限制，防止短信轰炸
//   - 验证码 5 分钟有效期，过期自动失效
//   - sync.Mutex 保护内存中的验证码数据，并发安全
package sms

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SMSService 管理短信验证码的生成、存储与校验。
// 使用内存 map 存储，服务重启后验证码失效。
type SMSService struct {
	mu      sync.Mutex              // 互斥锁，保护 codes 和 rateMap 的并发访问
	codes   map[string]codeEntry    // phone → 验证码及过期时间
	rateMap map[string]time.Time    // phone → 上次发送时间，用于频率限制
}

// codeEntry 存储验证码及其过期时间戳。
type codeEntry struct {
	Code      string    // 6 位数字验证码
	ExpiresAt time.Time // 过期时间点
}

// NewSMSService 创建短信服务实例。
func NewSMSService() *SMSService {
	return &SMSService{
		codes:   make(map[string]codeEntry),
		rateMap: make(map[string]time.Time),
	}
}

// SendCode 向指定手机号发送验证码。
// 包含 60 秒频率限制，开发模式下固定发送 "123456"。
func (s *SMSService) SendCode(phone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 频率限制：同一手机号 60 秒内只能发送一次
	if lastSend, ok := s.rateMap[phone]; ok && time.Since(lastSend) < 60*time.Second {
		return fmt.Errorf("验证码发送过于频繁，请 60 秒后再试")
	}

	code := "123456" // 开发模式固定验证码，生产环境需生成随机 6 位数字
	s.codes[phone] = codeEntry{Code: code, ExpiresAt: time.Now().Add(5 * time.Minute)}
	s.rateMap[phone] = time.Now()

	// 开发模式通过日志输出验证码，生产环境需替换为短信服务商 API 调用
	zap.L().Info("SMS 验证码（开发模式）",
		zap.String("phone", phone),
		zap.String("code", code),
	)

	return nil
}

// VerifyCode 校验手机号与验证码是否匹配。
// 验证成功后立即删除验证码，防止重复使用。
func (s *SMSService) VerifyCode(phone, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.codes[phone]
	if !ok {
		return false
	}

	// 过期检查
	if time.Now().After(entry.ExpiresAt) {
		delete(s.codes, phone)
		return false
	}

	// 验证码比对
	if entry.Code != code {
		return false
	}

	// 验证成功，立即删除（一次性使用）
	delete(s.codes, phone)
	return true
}
