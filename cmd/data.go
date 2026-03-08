package cmd

import "github.com/spf13/cobra"

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Query stored data and artifacts",
	Long:  `Query imported data and method execution artifacts in the PUDL catalog.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(dataCmd)
}
