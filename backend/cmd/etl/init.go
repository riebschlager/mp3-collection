package main

import (
	"fmt"
	"os"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the database",
	Long:  "Creates the SQLite database and runs the schema file.",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath, _ := cmd.Flags().GetString("db")
		schemaPath, _ := cmd.Flags().GetString("schema")

		fmt.Printf("Initializing database at %s with schema %s\n", dbPath, schemaPath)
		database, err := db.InitDB(dbPath, schemaPath)
		if err != nil {
			fmt.Printf("Error initializing database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()
		fmt.Println("Database initialized successfully.")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
	initCmd.Flags().String("schema", "schema.sql", "Path to schema file")
}
