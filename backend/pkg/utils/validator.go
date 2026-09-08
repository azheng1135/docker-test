package utils

import (
	"github.com/go-playground/validator/v10"
)

// TranslateValidationError 将 validator 包的英文校验错误翻译为中文提示。
//
// 支持的校验标签及其翻译：
//
//	required → "xxx 不能为空"
//	email    → "xxx 格式不正确"
//	min/max  → "xxx 长度不能小于/大于 N"
//	oneof    → "xxx 必须是 [a b c] 之一"
//	url/uuid → "xxx URL/UUID 格式不正确"
//
// 未识别的标签返回"请求参数有误"作为兜底。
func TranslateValidationError(err error) string {
	if errs, ok := err.(validator.ValidationErrors); ok {
		for _, e := range errs {
			switch e.Tag() {
			case "required":
				return e.Field() + " 不能为空"
			case "email":
				return e.Field() + " 格式不正确"
			case "min":
				return e.Field() + " 长度不能小于 " + e.Param()
			case "max":
				return e.Field() + " 长度不能大于 " + e.Param()
			case "len":
				return e.Field() + " 长度必须为 " + e.Param()
			case "eq":
				return e.Field() + " 必须等于 " + e.Param()
			case "gt":
				return e.Field() + " 必须大于 " + e.Param()
			case "gte":
				return e.Field() + " 必须大于等于 " + e.Param()
			case "lt":
				return e.Field() + " 必须小于 " + e.Param()
			case "lte":
				return e.Field() + " 必须小于等于 " + e.Param()
			case "oneof":
				return e.Field() + " 必须是 [" + e.Param() + "] 之一"
			case "url":
				return e.Field() + " URL 格式不正确"
			case "uuid":
				return e.Field() + " UUID 格式不正确"
			}
		}
	}
	return "请求参数有误"
}
