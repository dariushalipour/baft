package com.baft.intellij

import com.google.gson.Gson
import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.lang.annotation.AnnotationHolder
import com.intellij.lang.annotation.ExternalAnnotator
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.ApplicationNamesInfo
import com.intellij.openapi.editor.Document
import com.intellij.openapi.extensions.PluginId
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiFile
import java.io.File
import java.util.concurrent.atomic.AtomicReference

data class BaftAnnotatorInfo(val projectRoot: String, val filePath: String, val overlayJson: String?)
data class BaftOverlayFile(val path: String, val content: String)
data class BaftOverlayPayload(val files: List<BaftOverlayFile>)

private val gson = Gson()
private val lastNotification = AtomicReference<String?>(null)

private val checkRunner = BaftCheckRunner(binaryPath = ::findBinary, gson = gson)

private val compatibilityChecker = BaftCompatibilityChecker(
    binaryPath = ::findBinary,
    pluginVersion = ::currentPluginVersion,
    onVersionMismatch = ::notifyVersionMismatch,
    onFailure = ::notifyError,
    gson = gson,
    integrationId = ::currentIntegrationId,
)

private const val BAFT_PLUGIN_ID = "com.baft.intellij"

class BaftAnnotator : ExternalAnnotator<BaftAnnotatorInfo, List<BaftViolation>>() {

    override fun collectInformation(file: PsiFile): BaftAnnotatorInfo? {
        if (!file.isPhysical || file !== file.originalFile) return null
        if (file.viewProvider.getPsi(file.viewProvider.baseLanguage) !== file) return null
        val path = file.virtualFile?.path ?: return null
        // The annotator is registered for every language, so files baft never
        // scans are filtered here instead of spawning a check for them.
        if (!isScannedByBaft(path)) return null
        val root = file.project.basePath ?: return null
        return BaftAnnotatorInfo(root, path, collectOverlayJson(root))
    }

    override fun doAnnotate(info: BaftAnnotatorInfo): List<BaftViolation> {
        if (compatibilityChecker.check() !is CompatibilityResult.Success) return emptyList()

        val result = checkRunner.check(info.projectRoot, info.overlayJson)
        if (result.errors.isNotEmpty()) {
            notifyError("Baft check failed: " + result.errors.joinToString("\n"))
        }
        return result.violations.filter { it.file == info.filePath }
    }

    override fun apply(file: PsiFile, violations: List<BaftViolation>, holder: AnnotationHolder) {
        if (violations.isEmpty()) return
        val doc = FileDocumentManager.getInstance().getDocument(file.virtualFile) ?: return

        for (v in violations) {
            val range = toTextRange(doc, v) ?: continue
            holder.newAnnotation(toHighlightSeverity(v.severity), v.message)
                .range(range)
                .tooltip("[baft] ${v.rule}: ${v.message}")
                .create()
        }
    }

    private fun toTextRange(doc: Document, v: BaftViolation): TextRange? {
        val lineCount = doc.lineCount
        val zeroLine = (v.line - 1).coerceAtLeast(0)
        if (zeroLine >= lineCount) return null

        val lineStart = doc.getLineStartOffset(zeroLine)
        val lineEnd = doc.getLineEndOffset(zeroLine)
        val startCol = (v.column - 1).coerceAtLeast(0)
        val start = (lineStart + startCol).coerceAtMost(lineEnd)

        val end = if (v.lineEnd > 0 && v.lineEnd != v.line) {
            val zeroLineEnd = (v.lineEnd - 1).coerceAtMost(lineCount - 1)
            val endLineStart = doc.getLineStartOffset(zeroLineEnd)
            val endLineEnd = doc.getLineEndOffset(zeroLineEnd)
            if (v.columnEnd > 0) (endLineStart + v.columnEnd - 1).coerceAtMost(endLineEnd)
            else endLineEnd
        } else if (v.columnEnd > 0) {
            (lineStart + v.columnEnd - 1).coerceIn(start, lineEnd)
        } else {
            lineEnd
        }

        return TextRange(start, end.coerceAtLeast(start))
    }

    private fun toHighlightSeverity(severity: String): HighlightSeverity = when (severity) {
        "error" -> HighlightSeverity.ERROR
        "warning" -> HighlightSeverity.WARNING
        else -> HighlightSeverity.WEAK_WARNING
    }
}

private fun currentPluginVersion(): String {
    return PluginManagerCore.getPlugin(PluginId.getId(BAFT_PLUGIN_ID))?.version ?: "unknown"
}

private fun currentIntegrationId(): String =
    jetbrainsIntegrationId(ApplicationNamesInfo.getInstance().fullProductNameWithEdition)

private fun notifyError(message: String) {
    if (lastNotification.getAndSet(message) == message) return
    ApplicationManager.getApplication().invokeLater {
        NotificationGroupManager.getInstance()
            .getNotificationGroup(BAFT_NOTIFICATION_GROUP_ID)
            .createNotification(
                message,
                NotificationType.ERROR,
            )
            .notify(null)
    }
}

private fun notifyVersionMismatch(message: String, expectedVersion: String?, pluginVersion: String?) {
    val detail = if (expectedVersion != null && pluginVersion != null) {
        "Installed: $pluginVersion, Expected: $expectedVersion"
    } else {
        message
    }
    if (lastNotification.getAndSet(detail) == detail) return
    ApplicationManager.getApplication().invokeLater {
        val notification = NotificationGroupManager.getInstance()
            .getNotificationGroup(BAFT_NOTIFICATION_GROUP_ID)
            .createNotification(
                "Baft plugin version mismatch",
                detail,
                NotificationType.WARNING,
            )
        notification.addAction(object : NotificationAction("Reinstall") {
            override fun actionPerformed(e: com.intellij.openapi.actionSystem.AnActionEvent, notification: com.intellij.notification.Notification) {
                runReinstall()
            }
        })
        notification.notify(null)
    }
}

private fun runReinstall() {
    compatibilityChecker.reinstall(
        onSuccess = {
            notify("Baft plugin reinstalled successfully", "Please restart the IDE to activate the updated plugin.", NotificationType.INFORMATION)
        },
        onError = { message -> notify("Baft reinstall failed", message, NotificationType.ERROR) },
    )
}

private fun notify(title: String, content: String, type: NotificationType) {
    ApplicationManager.getApplication().invokeLater {
        NotificationGroupManager.getInstance()
            .getNotificationGroup(BAFT_NOTIFICATION_GROUP_ID)
            .createNotification(title, content, type)
            .notify(null)
    }
}

private fun collectOverlayJson(projectRoot: String): String? {
    val fileDocumentManager = FileDocumentManager.getInstance()
    val rootPath = File(projectRoot).toPath().normalize()
    val files = fileDocumentManager.unsavedDocuments.mapNotNull { document ->
        val virtualFile = fileDocumentManager.getFile(document) ?: return@mapNotNull null
        if (!isScannedByBaft(virtualFile.path)) return@mapNotNull null
        val filePath = File(virtualFile.path).toPath().normalize()
        if (!filePath.startsWith(rootPath)) return@mapNotNull null
        BaftOverlayFile(virtualFile.path, document.text)
    }.distinctBy { it.path }
    if (files.isEmpty()) return null
    return gson.toJson(BaftOverlayPayload(files))
}
