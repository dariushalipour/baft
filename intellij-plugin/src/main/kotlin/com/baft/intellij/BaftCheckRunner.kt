package com.baft.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import java.io.File
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap

data class BaftCheckOutput(
    val violations: List<BaftViolation> = emptyList(),
    val errors: List<String> = emptyList(),
)

/**
 * Runs `baft check` once per project root and serves every file of that root
 * from the result. Annotators of different files therefore share a single
 * process instead of racing and cancelling each other, and a run that fails
 * keeps the violations of the last good one so squiggles never vanish silently.
 */
class BaftCheckRunner(
    private val binaryPath: () -> String,
    private val environment: () -> Map<String, String> = { mapOf("PATH" to augmentedPath()) },
    private val gson: Gson = Gson(),
    private val ttlMillis: Long = 1_000,
    private val now: () -> Long = System::currentTimeMillis,
    private val start: (ProcessBuilder) -> Process = ProcessBuilder::start,
) {

    private class Root {
        var signature: String? = null
        var at = 0L
        var result = BaftCheckOutput()
        var lastGood: List<BaftViolation> = emptyList()
    }

    private val roots = ConcurrentHashMap<String, Root>()

    fun check(projectRoot: String, overlayJson: String?): BaftCheckOutput {
        val root = roots.computeIfAbsent(projectRoot) { Root() }
        val signature = overlayJson ?: ""
        synchronized(root) {
            if (root.signature != signature || now() - root.at >= ttlMillis) {
                val output = run(projectRoot, overlayJson)
                root.signature = signature
                root.at = now()
                root.result = if (output.errors.isEmpty()) {
                    root.lastGood = output.violations
                    output
                } else {
                    BaftCheckOutput(root.lastGood, output.errors)
                }
            }
            return root.result
        }
    }

    private fun run(projectRoot: String, overlayJson: String?): BaftCheckOutput {
        val command = mutableListOf(binaryPath(), "check", "--reporter=intellij")
        if (overlayJson != null) command.add("--overlay-stdin")
        command.add(".")

        val process = try {
            val builder = ProcessBuilder(command).directory(File(projectRoot))
            builder.environment().putAll(environment())
            start(builder)
        } catch (e: IOException) {
            return failure("Baft: could not run ${command.first()}: ${e.message}")
        }

        process.outputStream.bufferedWriter().use { writer ->
            if (overlayJson != null) writer.write(overlayJson)
        }
        val stdout = process.inputStream.bufferedReader().readText().trim()
        val stderr = process.errorStream.bufferedReader().readText().trim()
        process.waitFor()

        val payload = try {
            if (stdout.isEmpty()) null else gson.fromJson(stdout, Payload::class.java)
        } catch (_: JsonSyntaxException) {
            null
        } ?: return failure(stderr.ifBlank { "Baft check produced no readable output" })

        return BaftCheckOutput(payload.violations ?: emptyList(), payload.errors ?: emptyList())
    }

    private fun failure(message: String) = BaftCheckOutput(errors = listOf(message))

    private data class Payload(val violations: List<BaftViolation>?, val errors: List<String>?)
}
