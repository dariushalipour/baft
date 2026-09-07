package com.baft.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import java.io.File
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

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
    private val timeoutMillis: Long = 60_000,
    private val staleMillis: Long = 5 * 60_000,
    private val now: () -> Long = System::currentTimeMillis,
    private val start: (ProcessBuilder) -> Process = ProcessBuilder::start,
) {

    private class Root {
        var signature: String? = null
        var at = 0L
        var goodAt = 0L
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
                    root.goodAt = now()
                    output
                } else {
                    // Stale squiggles must not outlive a failure that keeps failing.
                    if (now() - root.goodAt >= staleMillis) root.lastGood = emptyList()
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

        // Both pipes are drained concurrently, so neither can fill and deadlock
        // the other, and the wait is bounded so a hung CLI cannot block every
        // other annotator. This thread swallows nothing, so a cancellation
        // (ProcessCanceledException) still reaches IntelliJ.
        val stdout = StringBuilder()
        val stderr = StringBuilder()
        val pipes = listOf(
            pipe { process.outputStream.bufferedWriter().use { writer -> overlayJson?.let(writer::write) } },
            pipe { process.inputStream.bufferedReader().use { reader -> stdout.append(reader.readText()) } },
            pipe { process.errorStream.bufferedReader().use { reader -> stderr.append(reader.readText()) } },
        )
        val finished = process.waitFor(timeoutMillis, TimeUnit.MILLISECONDS)
        if (!finished) process.destroyForcibly()
        pipes.forEach { it.join(JOIN_MILLIS) }
        if (!finished) return failure("Baft check timed out after ${timeoutMillis}ms")

        val text = stdout.toString().trim()
        val payload = try {
            if (text.isEmpty()) null else gson.fromJson(text, Payload::class.java)
        } catch (_: JsonSyntaxException) {
            null
        } ?: return failure(stderr.toString().trim().ifBlank { "Baft check produced no readable output" })

        return BaftCheckOutput(payload.violations ?: emptyList(), payload.errors ?: emptyList())
    }

    // A killed process breaks its pipes; that is expected, anything else is not.
    private fun pipe(body: () -> Unit) = Thread {
        try {
            body()
        } catch (_: IOException) {
        }
    }.apply { isDaemon = true; start() }

    private fun failure(message: String) = BaftCheckOutput(errors = listOf(message))

    private data class Payload(val violations: List<BaftViolation>?, val errors: List<String>?)

    private companion object {
        const val JOIN_MILLIS = 1_000L
    }
}
