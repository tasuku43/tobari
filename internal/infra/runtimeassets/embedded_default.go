//go:build !tobari_exposure_helper

package runtimeassets

import "embed"

//go:embed all:assets all:_helper-source
var embedded embed.FS
