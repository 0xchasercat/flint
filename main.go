package main

import (
	"embed"
	"github.com/volantvm/flint/cmd"
)

//go:embed all:web/out all:web/public
var assets embed.FS

func main() {
	cmd.ExecuteWithAssets(assets)
}
