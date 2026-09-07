// Package ui embeds and serves the Assay browser interface.
package ui

import "embed"

//go:embed dist
var embeddedAssets embed.FS
