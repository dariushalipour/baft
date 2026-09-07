package com.baft.intellij

import java.io.File
import java.util.concurrent.atomic.AtomicReference

const val BAFT_NOTIFICATION_GROUP_ID = "BAFT"

// Everything that can change a check: the sources baft scans, the contract,
// the capsule manifests that delimit them, and the files that decide what is
// scanned at all. Keep in sync with SCANNED in the VS Code extension.
private val scannedFile = Regex(
    """(\.(go|ts|tsx|py|pyi|rs|java|kt|cs|csproj|dart)|[\\/](BAFT\.md|go\.mod|package\.json|pom\.xml|build\.gradle(\.kts)?|Cargo\.toml|pyproject\.toml|setup\.py|pubspec\.yaml|tsconfig[^\\/]*\.json|\.gitignore|\.baftignore))$"""
)

internal fun isScannedByBaft(path: String): Boolean = scannedFile.containsMatchIn(path)

internal fun findBinary(): String = resolveBinary(BaftSettings.getInstance().binaryPath)

// The configured executable wins; otherwise the first `baft` on PATH.
internal fun resolveBinary(configured: String): String {
    if (configured.isNotBlank()) return configured.trim()
    val os = System.getProperty("os.name").lowercase()
    val isWin = os.contains("win")
    val name = if (isWin) "baft.exe" else "baft"
    val separator = if (isWin) ";" else ":"
    return augmentedPath().split(separator)
        .map { File(it, name) }
        .firstOrNull { it.canExecute() }
        ?.absolutePath ?: name
}

/** Every notification funnels through here so a repeated message is shown once. */
internal class NotificationDeduper {
    private val last = AtomicReference<String?>(null)

    fun isNew(message: String): Boolean = last.getAndSet(message) != message

    /** A good run forgets the last failure, so the same one is reported again if it returns. */
    fun reset() = last.set(null)
}

// A mismatch is only actionable when both versions are known.
internal fun versionMismatchDetail(message: String, expectedVersion: String?, pluginVersion: String?): String =
    if (expectedVersion != null && pluginVersion != null) "Installed: $pluginVersion, Expected: $expectedVersion" else message

// The CLI identifies each IDE by these ids; `integrate --integration=<id>` installs
// into exactly that IDE instead of the first one of the JetBrains family.
internal fun jetbrainsIntegrationId(productName: String): String {
    val name = productName.lowercase()
    return when {
        name.contains("goland") -> "goland"
        name.contains("intellij") && name.contains("community") -> "intellij-community"
        name.contains("intellij") -> "intellij-ultimate"
        name.contains("webstorm") -> "webstorm"
        name.contains("rider") -> "rider"
        name.contains("android") -> "android-studio"
        name.contains("rustrover") -> "rustrover"
        else -> "jetbrains"
    }
}

internal fun augmentedPath(): String {
    val current = System.getenv("PATH") ?: ""
    val home = System.getProperty("user.home") ?: return current
    val os = System.getProperty("os.name").lowercase()
    val extras = when {
        os.contains("win") -> listOf(
            "$home\\go\\bin",
            "$home\\AppData\\Local\\Programs\\Go\\bin",
            "C:\\Go\\bin",
            "C:\\Program Files\\Go\\bin",
        )
        os.contains("mac") -> listOf(
            "$home/go/bin",
            "$home/.local/bin",
            "/usr/local/go/bin",
            "/opt/homebrew/bin",
            "/usr/local/bin",
        )
        else -> listOf(
            "$home/go/bin",
            "$home/.local/bin",
            "/usr/local/go/bin",
            "/usr/local/bin",
            "/snap/bin",
        )
    }
    val separator = if (os.contains("win")) ";" else ":"
    val parts = current.split(separator).toMutableList()
    for (extra in extras.reversed()) {
        if (extra !in parts) parts.add(0, extra)
    }
    return parts.joinToString(separator)
}
