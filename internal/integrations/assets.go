package integrations

import "embed"

//go:embed embedded/vscode/baft-vscode.vsix embedded/jetbrains/baft-intellij.zip
var embeddedAssets embed.FS

const (
	vscodeAssetPath      = "embedded/vscode/baft-vscode.vsix"
	jetbrainsAssetPath   = "embedded/jetbrains/baft-intellij.zip"
	vscodeExtensionID    = "dariushalipour.baft"
	jetbrainsArchiveRoot = "baft-intellij"
	jetbrainsPluginID    = "com.baft.intellij"
)
