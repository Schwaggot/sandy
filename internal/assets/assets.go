package assets

import "embed"

//go:embed agents/*.yaml
var Agents embed.FS

//go:embed profiles/*.yaml
var Profiles embed.FS
