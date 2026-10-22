package cmd

import (
	"github.com/Havens-blog/e-cmdb-service/cmd/seed"
	"github.com/Havens-blog/e-cmdb-service/cmd/start"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var confFile string

var rootCmd = &cobra.Command{
	Use:   "e-cmdb-service",
	Short: "CMDB 配置管理服务",
	Long:  "CMDB 配置管理服务 - 从 e-cam-service 拆分的独立子模块",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func initViper() {
	viper.SetConfigFile(confFile)
	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}

// Execute 执行根命令。
func Execute(version string) {
	cobra.OnInitialize(initViper)
	rootCmd.AddCommand(start.Cmd)
	rootCmd.AddCommand(seed.Cmd)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&confFile, "config-file", "f", "config/prod.yaml", "the service config file")
}
