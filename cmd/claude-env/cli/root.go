package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "claude-env",
	Short: "Manage multiple Claude Code subscriptions",
	Long:  `claude-env manages multiple Claude Code OAuth sessions with easy swapping and declarative shared state (agents, skills, commands, plugins).`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.claude-envs/config.toml)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := homeDir()
		if err != nil {
			return
		}

		viper.AddConfigPath(home + "/.claude-envs")
		viper.SetConfigName("config")
		viper.SetConfigType("toml")
	}

	viper.SetEnvPrefix("CLAUDE_ENV")
	viper.AutomaticEnv()

	_ = viper.ReadInConfig()
}
