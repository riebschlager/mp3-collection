package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "etl",
	Short: "MP3 Collection ETL Pipeline",
	Long:  "A unified CLI to ingest data into the MP3 collection SQLite database.",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
