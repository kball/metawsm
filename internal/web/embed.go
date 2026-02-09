//go:build embed

package web

import (
	"embed"
	"io/fs"
)

//go:embed embed/public
var embeddedFS embed.FS

var PublicFS, _ = fs.Sub(embeddedFS, "embed/public")
