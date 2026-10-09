// E-CMDB Service - 配置管理数据库服务
// 从 e-cam-service 拆分的独立子模块，由运维平台网关统一提供入口。
package main

import (
	"fmt"

	"github.com/Havens-blog/e-cmdb-service/cmd"
)

var version string

func main() {
	ver := version
	if ver == "" {
		ver = "dev"
	}
	fmt.Println()
	fmt.Println("  E-CMDB-SERVICE  (配置管理数据库服务)")
	fmt.Printf("  Version: %s\n", ver)
	fmt.Println()

	cmd.Execute(ver)
}
