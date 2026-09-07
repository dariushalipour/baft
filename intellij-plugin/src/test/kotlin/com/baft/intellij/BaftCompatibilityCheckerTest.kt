package com.baft.intellij

import com.google.gson.Gson
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
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
            stdout = """{"compatible":true,"code":"ok","message":"compatible"}""",
            exitValue = 0,
        )

        assertTrue(createChecker().check() is CompatibilityResult.Success)
    }

    @Test
    fun `check classifies a version mismatch by code, not by message`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"code":"version_mismatch","message":"les versions ne correspondent pas","expected_version":"0.2.1","plugin_version":"0.1.2"}""",
            exitValue = 1,
        )

        val result = assertIs<CompatibilityResult.VersionMismatch>(createChecker().check())

        assertEquals("0.2.1", result.expectedVersion)
        assertEquals("0.1.2", result.pluginVersion)
        assertEquals(1, capturedMismatchMessages.size)
    }

    @Test
    fun `check returns failure for non-version-mismatch incompatibility`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"code":"protocol_mismatch","message":"Baft plugin protocol mismatch: plugin uses protocol 2, CLI expects protocol 3"}""",
            exitValue = 1,
        )

        val result = assertIs<CompatibilityResult.Failure>(createChecker().check())

        assertTrue(result.message.contains("protocol mismatch"))
        assertEquals(1, capturedFailureMessages.size)
    }

    @Test
    fun `check returns failure when binary not found`() {
        val checker = createChecker(binaryPath = { throw java.io.IOException("not found") })

        val result = assertIs<CompatibilityResult.Failure>(checker.check())

        assertTrue(result.message.contains("Baft: binary not found"))
    }

    @Test
    fun `check returns failure when stdout is invalid json`() {
        mockFactory.nextProcess = MockProcessResult(stdout = "not json at all", exitValue = 1)

        val result = assertIs<CompatibilityResult.Failure>(createChecker().check())

        assertEquals("Baft compatibility check failed", result.message)
    }

    @Test
    fun `check returns failure when stderr has content and stdout is empty`() {
        mockFactory.nextProcess = MockProcessResult(stderr = "some error from CLI", exitValue = 1)

        val result = assertIs<CompatibilityResult.Failure>(createChecker().check())

        assertEquals("some error from CLI", result.message)
    }

    @Test
    fun `check caches a success and keeps annotating every later file`() {
        var callCount = 0
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"code":"ok","message":"compatible"}""",
            exitValue = 0,
        )

        val checker = createChecker(binaryPath = { callCount++; "/fake/baft" })

        repeat(3) { assertTrue(checker.check() is CompatibilityResult.Success) }
        assertEquals(1, callCount)
    }

    @Test
    fun `check retries after a failure`() {
        var callCount = 0
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":false,"code":"version_mismatch","message":"mismatch"}""",
            exitValue = 1,
        )

        val checker = createChecker(binaryPath = { callCount++; "/fake/baft" })

        repeat(3) { checker.check() }
        assertEquals(3, callCount)
    }

    @Test
    fun `reset allows re-running the check`() {
        var callCount = 0
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"code":"ok","message":"compatible"}""",
            exitValue = 0,
        )

        val checker = createChecker(binaryPath = { callCount++; "/fake/baft" })

        checker.check()
        checker.reset()
        checker.check()

        assertEquals(2, callCount)
    }

    @Test
    fun `check reports the running IDE, not just the family`() {
        mockFactory.nextProcess = MockProcessResult(
            stdout = """{"compatible":true,"code":"ok","message":"compatible"}""",
            exitValue = 0,
        )

        createChecker(integrationId = { "intellij-ultimate" }).check()

        assertContains(mockFactory.commands.last(), "--integration=intellij-ultimate")
        assertContains(mockFactory.commands.last(), "--plugin-version=0.2.1")
    }

    // --- Reinstall ---

    @Test
    fun `reinstall targets the running IDE and calls onSuccess when CLI exits 0`() {
        mockFactory.nextProcess = MockProcessResult(exitValue = 0)

        createChecker(integrationId = { "goland" }).reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(1, capturedSuccessCallbacks.size)
        assertEquals(0, capturedErrorCallbacks.size)
        assertContains(mockFactory.commands.last(), "--integration=goland")
    }

    @Test
    fun `reinstall calls onError when CLI exits non-zero`() {
        mockFactory.nextProcess = MockProcessResult(stderr = "no supported integrations detected", exitValue = 1)

        createChecker().reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(0, capturedSuccessCallbacks.size)
        assertEquals(listOf("no supported integrations detected"), capturedErrorCallbacks)
    }

    @Test
    fun `reinstall calls onError with default message when stderr is empty`() {
        mockFactory.nextProcess = MockProcessResult(exitValue = 1)

        createChecker().reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals("Reinstall failed", capturedErrorCallbacks[0])
    }

    @Test
    fun `reinstall calls onError when binary not found`() {
        createChecker(binaryPath = { throw java.io.IOException("not found") }).reinstall(
            onSuccess = { capturedSuccessCallbacks.add(Unit) },
            onError = { capturedErrorCallbacks.add(it) },
        )

        assertEquals(0, capturedSuccessCallbacks.size)
        assertTrue(capturedErrorCallbacks[0].contains("Could not run reinstall"))
    }

    private fun createChecker(
        binaryPath: () -> String = { "/fake/baft" },
        integrationId: () -> String = { "jetbrains" },
    ): BaftCompatibilityChecker {
        return BaftCompatibilityChecker(
            binaryPath = binaryPath,
            pluginVersion = { "0.2.1" },
            onVersionMismatch = { msg, ev, pv -> capturedMismatchMessages.add(Triple(msg, ev, pv)) },
            onFailure = { msg -> capturedFailureMessages.add(msg) },
            gson = gson,
            integrationId = integrationId,
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
    val commands = mutableListOf<List<String>>()

    override fun start(vararg command: String): java.lang.Process {
        commands.add(command.toList())
        return MockProcess(nextProcess ?: MockProcessResult())
    }
}

class MockProcess(private val result: MockProcessResult) : java.lang.Process() {
    private val stdoutStream = ByteArrayInputStream(result.stdout.toByteArray(Charsets.UTF_8))
    private val stderrStream = ByteArrayInputStream(result.stderr.toByteArray(Charsets.UTF_8))

    override fun getInputStream(): java.io.InputStream = stdoutStream
    override fun getOutputStream(): java.io.OutputStream = ByteArrayOutputStream()
    override fun getErrorStream(): java.io.InputStream = stderrStream
    override fun waitFor(): Int = result.exitValue
    override fun waitFor(timeout: Long, unit: java.util.concurrent.TimeUnit): Boolean = true
    override fun destroy() {}
    override fun isAlive() = false
    override fun exitValue(): Int = result.exitValue
}
