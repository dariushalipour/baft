package com.baft.intellij

import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class BaftCheckRunnerTest {

    private val violationJson =
        """{"rule":"import-not-allowed","severity":"error","message":"nope","file":"/repo/a.go","line":1,"column":1}"""

    @Test
    fun `serves every file of a root from one check`() {
        val runner = testRunner("""{"violations":[$violationJson],"errors":[]}""")

        val first = runner.check("/repo", null)
        val second = runner.check("/repo", null)

        assertEquals(first, second)
        assertEquals(1, spawned.size)
        assertEquals("/repo/a.go", second.violations.single().file)
    }

    @Test
    fun `re-runs when the unsaved buffers change`() {
        val runner = testRunner("""{"violations":[],"errors":[]}""")

        runner.check("/repo", null)
        runner.check("/repo", """{"files":[]}""")

        assertEquals(2, spawned.size)
    }

    @Test
    fun `each root gets its own check`() {
        val runner = testRunner("""{"violations":[],"errors":[]}""")

        runner.check("/repo", null)
        runner.check("/other", null)

        assertEquals(listOf("/repo", "/other"), spawned)
    }

    @Test
    fun `a failing check keeps the last good violations and reports the error`() {
        var payload = """{"violations":[$violationJson],"errors":[]}"""
        val runner = testRunner({ payload })

        runner.check("/repo", null)
        payload = """{"violations":[],"errors":["discovery: boom"]}"""
        val failed = runner.check("/repo", "overlay-changed")

        assertEquals(listOf("discovery: boom"), failed.errors)
        assertEquals("/repo/a.go", failed.violations.single().file)
    }

    @Test
    fun `unreadable output is reported as an error, not as zero violations`() {
        val runner = testRunner("not json")

        val result = runner.check("/repo", null)

        assertTrue(result.errors.single().isNotBlank())
        assertEquals(emptyList(), result.violations)
    }

    private val spawned = mutableListOf<String>()

    private fun testRunner(stdout: String) = testRunner({ stdout })

    private fun testRunner(stdout: () -> String) = BaftCheckRunner(
        binaryPath = { "/fake/baft" },
        environment = { emptyMap() },
        ttlMillis = 60_000,
        start = { builder ->
            spawned.add(builder.directory().path)
            MockProcess(MockProcessResult(stdout = stdout()))
        },
    )
}
