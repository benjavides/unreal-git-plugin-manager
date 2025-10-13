package new_project_example_config_files

import "embed"

// FS contains the embedded project configuration templates.
//go:embed .gitattributes common.gitignore with_plugin_binaries.gitignore without_plugin_binaries.gitignore
var FS embed.FS
