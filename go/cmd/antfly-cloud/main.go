package main

import (
	"fmt"
	"os"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(sdk.Version)
		return
	}

	fmt.Fprintln(os.Stderr, "antfly-cloud CLI is being migrated into this repository")
	os.Exit(1)
}
