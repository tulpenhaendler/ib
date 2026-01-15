package backup

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup operations",
	Long:  "Create, list, and restore backups.",
}

func init() {
	Cmd.AddCommand(createCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(restoreCmd)
}
