package com.baft.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import java.io.IOException
import java.io.InputStream
import java.lang.ProcessBuilder

/**
 * Standalone compatibility checker that can be tested without the IntelliJ SDK.
 *
 * The checker spawns the baft CLI, parses the JSON compatibility report, and
 * returns a [CompatibilityResult].  Notification callbacks are injected so
 * that production code can wire up IntelliJ notifications, while tests can
 * capture them in memory.
 */
class BaftCompatibilityChecker(
    private val binaryPath: () -> String,
    private val pluginVersion: () -> String,
    private val onVersionMismatch: (message: String, expectedVersion: String?, pluginVersion: String?) -> Unit,
    private val onFailure: (message: String) -> Unit,
    private val gson: Gson = Gson(),
    private val protocolVersion: Int = 3,
    private val processBuilderFactory: ProcessBuilderFactory = DefaultProcessBuilderFactory,
) {

    private var checked = false
    private var failureMessage: String? = null

    /**
     * Run the compatibility check once. Subsequent calls return the cached result.
     */
    fun check(): CompatibilityResult {
        if (checked) {
            return CompatibilityResult.failure(failureMessage)
        }

        synchronized(this) {
            if (checked) {
                return CompatibilityResult.failure(failureMessage)
            }

            val result = doCheck()
            failureMessage = when (result) {
                is CompatibilityResult.Success -> null
                is CompatibilityResult.Failure -> result.message
                is CompatibilityResult.VersionMismatch -> result.message
            }
            checked = true
            when (result) {
                is CompatibilityResult.Success -> { /* ok */ }
                is CompatibilityResult.Failure -> onFailure(result.message)
                is CompatibilityResult.VersionMismatch -> onVersionMismatch(
                    result.message, result.expectedVersion, result.pluginVersion
                )
            }
            return result
        }
    }

    /**
     * Reset the cache so the check can be run again.
     * Intended for testing only.
     */
    internal fun reset() {
        checked = false
        failureMessage = null
    }

    /**
     * Factory interface for creating processes, to allow testing without
     * spawning real subprocesses.
     */
    interface ProcessBuilderFactory {
        fun start(vararg command: String): java.lang.Process
    }

    private object DefaultProcessBuilderFactory : ProcessBuilderFactory {
        override fun start(vararg command: String): java.lang.Process {
            return ProcessBuilder(*command).start()
        }
    }

    private fun doCheck(): CompatibilityResult {
        val process = try {
            processBuilderFactory.start(
                binaryPath(),
                "integrate",
                "--verify-compatible",
                "--integration=jetbrains",
                "--plugin-version=${pluginVersion()}",
                "--protocol=$protocolVersion",
            )
        } catch (e: IOException) {
            return CompatibilityResult.failure("Baft: binary not found in PATH")
        }

        val stdoutText = process.inputStream.bufferedReader().readText().trim()
        val stderrText = process.errorStream.bufferedReader().readText().trim()
        process.waitFor()

        val report: BaftCompatibilityReport? = try {
            if (stdoutText.isBlank()) null
            else gson.fromJson(stdoutText, BaftCompatibilityReport::class.java)
        } catch (_: JsonSyntaxException) {
            null
        }

        return when {
            process.exitValue() == 0 && report?.compatible == true -> {
                CompatibilityResult.success()
            }
            !report?.message.isNullOrBlank() -> {
                val msg = report.message
                if (msg.contains("version mismatch")) {
                    CompatibilityResult.versionMismatch(msg, report.expected_version, report.plugin_version)
                } else {
                    CompatibilityResult.failure(msg)
                }
            }
            stderrText.isNotBlank() -> CompatibilityResult.failure(stderrText)
            else -> CompatibilityResult.failure("Baft compatibility check failed")
        }
    }

    /**
     * Run the reinstall command and dispatch the appropriate callback.
     */
    fun reinstall(
        onSuccess: () -> Unit,
        onError: (message: String) -> Unit,
    ) {
        try {
            val process = processBuilderFactory.start(
                binaryPath(),
                "integrate",
                "--integration=jetbrains",
                "--yes",
            )
            val stdout = process.inputStream.bufferedReader().readText()
            val stderr = process.errorStream.bufferedReader().readText()
            process.waitFor()

            if (process.exitValue() == 0) {
                onSuccess()
            } else {
                onError(stderr.trim().ifBlank { "Reinstall failed" })
            }
        } catch (e: Exception) {
            onError("Could not run reinstall: ${e.message}")
        }
    }
}
