package com.baft.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import java.io.IOException

const val COMPATIBILITY_VERSION_MISMATCH = "version_mismatch"

data class BaftCompatibilityReport(
    val compatible: Boolean = false,
    val code: String? = null,
    val message: String? = null,
    val expected_version: String? = null,
    val plugin_version: String? = null,
)

sealed class CompatibilityResult {
    object Success : CompatibilityResult()
    data class Failure(val message: String) : CompatibilityResult()
    data class VersionMismatch(val message: String, val expectedVersion: String?, val pluginVersion: String?) :
        CompatibilityResult()

    companion object {
        fun success(): CompatibilityResult = Success
        fun failure(message: String?) = Failure(message ?: "unknown error")
        fun versionMismatch(message: String, expectedVersion: String?, pluginVersion: String?) =
            VersionMismatch(message, expectedVersion, pluginVersion)
    }
}

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
    private val integrationId: () -> String = { "jetbrains" },
    private val protocolVersion: Int = 3,
    private val processBuilderFactory: ProcessBuilderFactory = DefaultProcessBuilderFactory,
) {

    @Volatile
    private var compatible: CompatibilityResult? = null

    /**
     * Run the compatibility check. A success is cached for the rest of the
     * session; a failure is retried, and the callbacks (which de-duplicate
     * notifications) report it again.
     */
    fun check(): CompatibilityResult {
        compatible?.let { return it }

        synchronized(this) {
            compatible?.let { return it }

            val result = doCheck()
            when (result) {
                is CompatibilityResult.Success -> compatible = result
                is CompatibilityResult.Failure -> onFailure(result.message)
                is CompatibilityResult.VersionMismatch -> onVersionMismatch(
                    result.message, result.expectedVersion, result.pluginVersion
                )
            }
            return result
        }
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
                "--integration=${integrationId()}",
                "--plugin-version=${pluginVersion()}",
                "--protocol=$protocolVersion",
            )
        } catch (e: IOException) {
            return CompatibilityResult.failure("Baft: binary not found. Install the CLI or set the Baft executable in Settings.")
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
            report?.compatible == true -> CompatibilityResult.success()
            report?.code == COMPATIBILITY_VERSION_MISMATCH -> CompatibilityResult.versionMismatch(
                report.message ?: "Baft plugin version mismatch",
                report.expected_version,
                report.plugin_version,
            )
            !report?.message.isNullOrBlank() -> CompatibilityResult.failure(report?.message)
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
                "--integration=${integrationId()}",
                "--yes",
            )
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
