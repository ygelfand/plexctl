package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"
	"github.com/ygelfand/plexctl/cmd"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	outDir := filepath.Join(cwd, "docs", "content", "cli")
	if err := os.RemoveAll(outDir); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatal(err)
	}

	root := cmd.GetRootCmd()
	root.DisableAutoGenTag = true

	err = doc.GenMarkdownTree(root, outDir)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Successfully generated CLI documentation in %s\n", outDir)
}
