package com.baft.intellij

import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class BaftCliTest {

    @Test
    fun `only files that can change a check trigger one`() {
        val scanned = listOf(
            "/repo/a.go", "/repo/a.tsx", "/repo/a.pyi", "/repo/a.csproj", "/repo/BAFT.md",
            "/repo/go.mod", "/repo/package.json", "/repo/build.gradle.kts",
            "/repo/tsconfig.json", "/repo/tsconfig.build.json", "/repo/.baftignore", "/repo/.gitignore",
        )
        for (path in scanned) {
            assertTrue(isScannedByBaft(path), path)
        }
        for (path in listOf("/repo/CHANGELOG.md", "/repo/README.md", "/repo/a.json", "/repo/NOTBAFT.md")) {
            assertFalse(isScannedByBaft(path), path)
        }
    }

    @Test
    fun `the configured executable is used verbatim`() {
        assertEquals("/opt/bin/baft", resolveBinary("  /opt/bin/baft  "))
    }

    @Test
    fun `an unset executable falls back to a PATH lookup`() {
        assertTrue(resolveBinary("  ").endsWith("baft") || resolveBinary("").endsWith("baft.exe"))
    }

    @Test
    fun `a repeated notification is shown once`() {
        val deduper = NotificationDeduper()

        assertTrue(deduper.isNew("Baft check failed"))
        assertFalse(deduper.isNew("Baft check failed"))
        assertTrue(deduper.isNew(versionMismatchDetail("mismatch", "0.4.0", "0.3.1")))
        assertFalse(deduper.isNew("Installed: 0.3.1, Expected: 0.4.0"))
        assertTrue(deduper.isNew("Baft check failed"))

        deduper.reset()
        assertTrue(deduper.isNew("Baft check failed"))
    }

    @Test
    fun `a mismatch without both versions falls back to the CLI message`() {
        assertEquals("mismatch", versionMismatchDetail("mismatch", null, "0.3.1"))
    }

    @Test
    fun `product names map to the CLI integration ids`() {
        assertEquals("goland", jetbrainsIntegrationId("GoLand"))
        assertEquals("intellij-community", jetbrainsIntegrationId("IntelliJ IDEA Community Edition"))
        assertEquals("intellij-ultimate", jetbrainsIntegrationId("IntelliJ IDEA Ultimate"))
        assertEquals("rustrover", jetbrainsIntegrationId("RustRover"))
        assertEquals("jetbrains", jetbrainsIntegrationId("Some Future IDE"))
    }
}
