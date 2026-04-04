package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and edit configuration",
	Long:  `View and edit the claude-env configuration file (config.toml).`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the full configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, paths, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		data, err := os.ReadFile(paths.ConfigFile) //#nosec G304
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No config file found. Run 'claude-env init' first.")
			return nil
		}
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}

		fmt.Print(string(data))
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := config.DefaultPaths()
		if err != nil {
			return fmt.Errorf("resolve paths: %w", err)
		}

		if cfgFile != "" {
			paths.ConfigFile = cfgFile
		}

		fmt.Println(paths.ConfigFile)
		return nil
	},
}

var configSetOverrideCmd = &cobra.Command{
	Use:   "set-override <path>",
	Short: "Set settings_override for an environment",
	Long:  `Sets a custom settings.json path for the specified environment (defaults to current).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		overridePath := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		if err := mgr.SetOverride(envName, overridePath); err != nil {
			return fmt.Errorf("set override: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Set settings_override for %q to %s\n", envName, overridePath)
		return nil
	},
}

var configClearOverrideCmd = &cobra.Command{
	Use:   "clear-override",
	Short: "Clear settings_override for an environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		if err := mgr.ClearOverride(envName); err != nil {
			return fmt.Errorf("clear override: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Cleared settings_override for %q\n", envName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configSetOverrideCmd)
	configCmd.AddCommand(configClearOverrideCmd)

	configSetOverrideCmd.Flags().String("env", "", "target environment (default: current)")
	configClearOverrideCmd.Flags().String("env", "", "target environment (default: current)")
}
