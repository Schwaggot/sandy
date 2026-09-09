// Package assets embeds the bundled agent manifests and profiles. Files in
// ~/.sandy override these by name at load time.
package assets

import "embed"

// Agents holds the bundled agent manifests.
//
//go:embed agents/*.yaml
var Agents embed.FS

// Profiles holds the bundled sandbox profiles.
//
//go:embed profiles/*.yaml
var Profiles embed.FS
