package com.baft.intellij

import java.io.File

const val BAFT_NOTIFICATION_GROUP_ID = "BAFT"

internal fun findBinary(): String {
    val os = System.getProperty("os.name").lowercase()
    val isWin = os.contains("win")
    val name = if (isWin) "baft.exe" else "baft"
    val separator = if (isWin) ";" else ":"
    return augmentedPath().split(separator)
        .map { File(it, name) }
        .firstOrNull { it.canExecute() }
        ?.absolutePath ?: name
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
