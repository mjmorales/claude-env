package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/credentials"
)

var exportCmd = &cobra.Command{
	Use:   "export <name>",
	Short: "Print an environment's OAuth token",
	Long: `Prints an environment's stored OAuth credential to stdout. By default the
tokens are redacted for safe display; pass --raw to emit the verbatim
.credentials.json blob (for backup or moving an account to another machine).

  claude-env export work                 # redacted
  claude-env export work --raw > work.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		data, err := mgr.Export(name)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}

		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			return fmt.Errorf("read --raw: %w", err)
		}
		if raw {
			fmt.Fprintln(os.Stdout, string(data))
			return nil
		}

		blob, err := credentials.Parse(data)
		if err != nil {
			return fmt.Errorf("parse credentials: %w", err)
		}
		out, err := json.MarshalIndent(blob.Redacted(), "", "  ")
		if err != nil {
			return fmt.Errorf("marshal redacted credentials: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().Bool("raw", false, "print the verbatim token blob instead of a redacted view")
}
