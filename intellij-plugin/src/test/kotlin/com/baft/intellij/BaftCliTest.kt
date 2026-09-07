package com.baft.intellij

import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class BaftCliTest {

    @Test
    fun `only files baft scans trigger a check`() {
        for (path in listOf("/repo/a.go", "/repo/a.tsx", "/repo/a.pyi", "/repo/BAFT.md")) {
            assertTrue(isScannedByBaft(path), path)
        }
        for (path in listOf("/repo/CHANGELOG.md", "/repo/a.json", "/repo/NOTBAFT.md")) {
            assertFalse(isScannedByBaft(path), path)
        }
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
