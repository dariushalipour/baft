package com.baft.intellij

import java.io.File

const val BAFT_NOTIFICATION_GROUP_ID = "BAFT"

// Files baft scans, plus the contract itself. Nothing else can change a result.
private val scannedFile = Regex("""(\.(go|ts|tsx|py|pyi|rs|java|kt|cs|dart)|[\\/]BAFT\.md)$""")

internal fun isScannedByBaft(path: String): Boolean = scannedFile.containsMatchIn(path)

internal fun findBinary(): String {
    val configured = BaftSettings.getInstance().binaryPath.trim()
    if (configured.isNotEmpty()) return configured
    val os = System.getProperty("os.name").lowercase()
    val isWin = os.contains("win")
    val name = if (isWin) "baft.exe" else "baft"
    val separator = if (isWin) ";" else ":"
    return augmentedPath().split(separator)
        .map { File(it, name) }
        .firstOrNull { it.canExecute() }
        ?.absolutePath ?: name
}

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
