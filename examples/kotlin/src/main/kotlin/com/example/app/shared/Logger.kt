package com.example.app.shared

import java.time.LocalDateTime

data class LogEntry(
    val level: LogLevel,
    val message: String,
    val context: Map<String, Any?>,
    val timestamp: LocalDateTime = LocalDateTime.now(),
)

enum class LogLevel { DEBUG, INFO, WARN, ERROR }

interface Logger {
    fun debug(message: String, context: Map<String, Any?> = emptyMap())
    fun info(message: String, context: Map<String, Any?> = emptyMap())
    fun warn(message: String, context: Map<String, Any?> = emptyMap())
    fun error(message: String, context: Map<String, Any?> = emptyMap())
}

class ConsoleLogger : Logger {
    override fun debug(message: String, context: Map<String, Any?>) = write(LogLevel.DEBUG, message, context)
    override fun info(message: String, context: Map<String, Any?>) = write(LogLevel.INFO, message, context)
    override fun warn(message: String, context: Map<String, Any?>) = write(LogLevel.WARN, message, context)
    override fun error(message: String, context: Map<String, Any?>) = write(LogLevel.ERROR, message, context)

    private fun write(level: LogLevel, message: String, context: Map<String, Any?>) {
        val entry = LogEntry(level, message, context)
        val line = kotlinx.serialization.json.Json.Default.toString(kotlinx.serialization.Serializable())
        when (level) {
            LogLevel.ERROR -> println("ERROR: $line")
            LogLevel.WARN -> println("WARN: $line")
            else -> println(line)
        }
    }
}
