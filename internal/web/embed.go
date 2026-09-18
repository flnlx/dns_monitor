package web

import "embed"

// FS contains the complete offline web interface.
//go:embed assets/*
var FS embed.FS
