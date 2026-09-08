// Package main 是 KeystoneGo CLI 脚手架工具的入口。
//
// keystone-cli 提供代码生成命令，可一键生成标准分层核心业务代码：
//
//	keystone-cli gen --table coupons --module coupon
//
// 生成的文件分布在四层：
//   - internal/model/coupon.go       — GORM 实体定义
//   - internal/repository/coupon.go  — 数据访问层
//   - internal/service/coupon.go     — 业务逻辑层
//   - internal/handler/coupon.go     — HTTP 控制器
//
// 生成后会自动 go fmt 格式化代码并运行 wire 重新生成依赖注入图谱。
package main

import (
	"fmt"
	"os"

	"keystonego/cmd/cli/generator"

	"github.com/spf13/cobra"
)

var (
	tableName  string // 数据库表名（如 coupons）
	moduleName string // 生成的模块包名（如 coupon）
)

// rootCmd 是 CLI 根命令。
var rootCmd = &cobra.Command{
	Use:   "keystone-cli",
	Short: "KeystoneGo 通用网站底座工业级脚手架生成工具",
}

// genCmd 是代码生成子命令，根据数据库表名自动生成四层代码。
var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "一键生成标准分层核心业务逻辑代码",
	Run: func(cmd *cobra.Command, args []string) {
		if tableName == "" || moduleName == "" {
			fmt.Println("错误: 必须指定 --table 和 --module 参数")
			_ = cmd.Help()
			return
		}

		// 表名 → 驼峰命名转换
		camel := generator.ToCamelCase(tableName)
		lowerCamel := generator.ToLowerCamelCase(tableName)

		data := generator.TemplateData{
			TableName:      tableName,
			CamelName:      camel,
			LowerCamelName: lowerCamel,
			ModulePackage:  moduleName,
		}

		// 执行代码生成
		if err := generator.GenerateModule(data); err != nil {
			fmt.Printf("代码生成失败: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// 注册命令行参数
	genCmd.Flags().StringVarP(&tableName, "table", "t", "", "数据库表名 (如: users)")
	genCmd.Flags().StringVarP(&moduleName, "module", "m", "", "生成的模块包名 (如: user)")
	rootCmd.AddCommand(genCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
