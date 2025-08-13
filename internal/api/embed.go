package api

import "embed"

//go:embed templates/doc.html templates/index.html
var webTemplate embed.FS
