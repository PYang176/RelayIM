// Package main 把 OpenAPI YAML 机械转换为通用 JSON 格式。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// main 读取 OpenAPI 定义并生成同步 JSON
func main() {
	source := filepath.Join("docs", "openapi.yaml")
	target := filepath.Join("docs", "openapi.json")

	content, err := os.ReadFile(source)
	if err != nil {
		exitf("读取 %s 失败: %v", source, err)
	}
	var document any
	if err := yaml.Unmarshal(content, &document); err != nil {
		exitf("解析 %s 失败: %v", source, err)
	}
	formatted, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		exitf("序列化 OpenAPI JSON 失败: %v", err)
	}
	formatted = append(formatted, '\n')
	if err := os.WriteFile(target, formatted, 0o644); err != nil {
		exitf("写入 %s 失败: %v", target, err)
	}
	fmt.Printf("已生成 %s\n", target)
}

// exitf 输出格式化错误并以失败状态退出 OpenAPI 生成脚本
func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
