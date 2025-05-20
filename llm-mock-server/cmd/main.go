package main

import (
	"fmt"
	"os"

	"llm-mock-server/pkg/cmd"
	"llm-mock-server/pkg/log"
)

func main() {
	log.InitLogger()

	if err := cmd.NewServerCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
