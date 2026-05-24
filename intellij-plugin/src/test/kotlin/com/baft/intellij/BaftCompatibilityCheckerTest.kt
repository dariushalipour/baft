package com.baft.intellij

import com.google.gson.Gson
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.lang.ProcessBuilder
import kotlin.test.*

class BaftCompatibilityCheckerTest {

    private val gson = Gson()
    private var capturedMismatchMessages = mutableListOf<Triple<String, String?, String?>>()
    private var capturedFailureMessages = mutableListOf<String>()
    private var capturedSuccessCallbacks = mutableListOf<Unit>()
    private var capturedErrorCallbacks = mutableListOf<String>()
    private lateinit var mockFactory: TestProcessBuilderFactory

    @BeforeEach
    fun setUp() {
        capturedMismatchMessages.clear()
        capturedFailureMessages.clear()
        capturedSuccessCallbacks.clear()
        capturedErrorCallbacks.clear()
        mockFactory = TestProcessBuilderFactory()
    }

    @Test
    fun `check returns success when CLI reports compatible`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"message":"compatible"}""",
            exitValue = 0,
        )

        val checker = createChecker()
        val result = checker.check()

        assertTrue(result is CompatibilityResult.Success)
    }

    @Test
    fun `check returns versionMismatch when CLI reports version mismatch`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"message":"Baft plugin version mismatch: expected 0.2.1, got 0.1.2","expected_version":"0.2.1","plugin_version":"0.1.2"}""",
            exitValue = 1,
        )

        val checker = createChecker()
        val result = checker.check()

        assertTrue(result is CompatibilityResult.VersionMismatch)
        val vm = result as CompatibilityResult.VersionMismatch
        assertEquals("0.2.1", vm.expectedVersion)
        assertEquals("0.1.2", vm.pluginVersion)
        assertTrue(vm.message.contains("version mismatch"))
        assertEquals(1, capturedMismatchMessages.size)
    }

    @Test
    fun `check returns failure for non-version-mismatch incompatibility`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"message":"Baft plugin protocol mismatch: plugin uses protocol 2, CLI expects protocol 3"}""",
            exitValue = 1,
        )

        val checker = createChecker()
        val result = checker.check()

        assertTrue(result is CompatibilityResult.Failure)
        val failure = result as CompatibilityResult.Failure
        assertTrue(failure.message.contains("protocol mismatch"))
        assertEquals(1, capturedFailureMessages.size)
    }

    @Test
    fun `check returns failure when binary not found`() {
        val checker = BaftCompatibilityChecker(
            binaryPath = { throw java.io.IOException("not found") },
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            processBuilderFactory = mockFactory,
        )

        val result = checker.check()

        assertTrue(result is CompatibilityResult.Failure)
        assertEquals("Baft: binary not found in PATH", (result as CompatibilityResult.Failure).message)
    }

    @Test
    fun `check returns failure when stdout is invalid json`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = "not json at all",
            exitValue = 1,
        )

        val checker = createChecker()
        val result = checker.check()

        assertTrue(result is CompatibilityResult.Failure)
        assertEquals("Baft compatibility check failed", (result as CompatibilityResult.Failure).message)
    }

    @Test
    fun `check returns failure when stderr has content and stdout is empty`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = "",
            stderr = "some error from CLI",
            exitValue = 1,
        )

        val checker = createChecker()
        val result = checker.check()

        assertTrue(result is CompatibilityResult.Failure)
        assertEquals("some error from CLI", (result as CompatibilityResult.Failure).message)
    }

    @Test
    fun `check caches result on subsequent calls`() {
        var callCount = 0
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"message":"Baft plugin version mismatch: expected 0.2.1, got 0.1.2"}""",
            exitValue = 1,
        )

        val checker = BaftCompatibilityChecker(
            binaryPath = {
                callCount++
                "/fake/baft"
            },
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            processBuilderFactory = mockFactory,
        )

        checker.check()
        checker.check()
        checker.check()

        assertEquals(1, callCount)
    }

    @Test
    fun `reset allows re-running the check`() {
        var callCount = 0
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"message":"compatible"}""",
            exitValue = 0,
        )

        val checker = BaftCompatibilityChecker(
            binaryPath = {
                callCount++
                "/fake/baft"
            },
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            processBuilderFactory = mockFactory,
        )

        checker.check()
        assertEquals(1, callCount)

        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"message":"compatible"}""",
            exitValue = 0,
        )
        checker.reset()
        checker.check()

        assertEquals(2, callCount)
    }

    @Test
    fun `check uses injected plugin version`() {
        var capturedVersion = ""
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"message":"compatible"}""",
            exitValue = 0,
        )

        val checker = BaftCompatibilityChecker(
            binaryPath = { "/fake/baft" },
            pluginVersion = {
                capturedVersion = "1.0.0-beta"
                "1.0.0-beta"
            },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            processBuilderFactory = mockFactory,
        )

        checker.check()
        assertEquals("1.0.0-beta", capturedVersion)
    }

    // --- Reinstall tests ---

    @Test
    fun `reinstall calls onSuccess when CLI exits 0`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = "Baft integration installed successfully for VS Code 1.100.0",
            exitValue = 0,
        )

        val checker = createChecker()
        checker.reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(1, capturedSuccessCallbacks.size)
        assertEquals(0, capturedErrorCallbacks.size)
    }

    @Test
    fun `reinstall calls onError when CLI exits non-zero`() {
        mockFactory.nextProcess = MockProcessResult(
            stderr = "no supported integrations detected",
            exitValue = 1,
        )

        val checker = createChecker()
        checker.reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(0, capturedSuccessCallbacks.size)
        assertEquals(1, capturedErrorCallbacks.size)
        assertEquals("no supported integrations detected", capturedErrorCallbacks[0])
    }

    @Test
    fun `reinstall calls onError with default message when stderr is empty`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = "",
            stderr = "",
            exitValue = 1,
        )

        val checker = createChecker()
        checker.reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals("Reinstall failed", capturedErrorCallbacks[0])
    }

    @Test
    fun `reinstall calls onError when binary not found`() {
        val checker = BaftCompatibilityChecker(
            binaryPath = { throw java.io.IOException("not found") },
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { _, _, _ -> },
            onFailure = { },
            gson = gson,
            processBuilderFactory = mockFactory,
        )

        checker.reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(0, capturedSuccessCallbacks.size)
        assertEquals(1, capturedErrorCallbacks.size)
        assertTrue(capturedErrorCallbacks[0].contains("Could not run reinstall"))
    }

    // --- Version mismatch callback fires ---

    @Test
    fun `check fires onVersionMismatch callback for version mismatch`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"message":"Baft plugin version mismatch: expected 0.2.1, got 0.1.2","expected_version":"0.2.1","plugin_version":"0.1.2"}""",
            exitValue = 1,
        )

        val checker = createChecker()
        checker.check()

        assertEquals(1, capturedMismatchMessages.size)
        val triple = capturedMismatchMessages[0]
        assertEquals("0.2.1", triple.second)
        assertEquals("0.1.2", triple.third)
    }

    @Test
    fun `check fires onFailure callback for other failures`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"message":"Baft plugin protocol mismatch: plugin uses protocol 2, CLI expects protocol 3"}""",
            exitValue = 1,
        )

        val checker = createChecker()
        checker.check()

        assertEquals(1, capturedFailureMessages.size)
        assertTrue(capturedFailureMessages[0].contains("protocol mismatch"))
    }

    // --- Helpers ---

    private fun createChecker(): BaftCompatibilityChecker {
        return BaftCompatibilityChecker(
            binaryPath = { "/fake/baft" },
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            processBuilderFactory = mockFactory,
        )
    }
}

data class MockProcessResult(
    val stdout: String = "",
    val stderr: String = "",
    val exitValue: Int = 0,
)

class TestProcessBuilderFactory : BaftCompatibilityChecker.ProcessBuilderFactory {
    var nextProcess: MockProcessResult? = null

    override fun start(vararg command: String): java.lang.Process {
        val result = nextProcess ?: MockProcessResult()
        return MockProcess(result)
    }
}

class MockProcess(private val result: MockProcessResult) : java.lang.Process() {
    private val stdoutStream = ByteArrayInputStream(result.stdout.toByteArray(Charsets.UTF_8))
    private val stderrStream = ByteArrayInputStream(result.stderr.toByteArray(Charsets.UTF_8))
    private val stdoutCopy = ByteArrayInputStream(result.stdout.toByteArray(Charsets.UTF_8))

    override fun getInputStream(): java.io.InputStream = stdoutStream
    override fun getOutputStream(): java.io.OutputStream = ByteArrayOutputStream()
    override fun getErrorStream(): java.io.InputStream = stderrStream
    override fun waitFor(): Int = result.exitValue
    override fun waitFor(timeout: Long, unit: java.util.concurrent.TimeUnit): Boolean = true
    override fun destroy() {}
    override fun isAlive() = false
    override fun exitValue(): Int = result.exitValue
}
