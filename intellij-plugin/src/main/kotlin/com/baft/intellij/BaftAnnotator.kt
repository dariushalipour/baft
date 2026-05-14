package com.baft.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import com.google.gson.reflect.TypeToken
import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.lang.annotation.AnnotationHolder
import com.intellij.lang.annotation.ExternalAnnotator
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.editor.Document
import com.intellij.openapi.extensions.PluginId
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiFile
import java.io.File
import java.io.IOException
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

data class BaftAnnotatorInfo(val projectRoot: String, val filePath: String, val overlayJson: String?)
data class BaftOverlayFile(val path: String, val content: String)
data class BaftOverlayPayload(val files: List<BaftOverlayFile>)
data class BaftCompatibilityReport(val compatible: Boolean, val message: String, val warning: String?)

private val log = Logger.getInstance(BaftAnnotator::class.java)
private val gson = Gson()
private val violationType = object : TypeToken<List<BaftViolation>>() {}.type
private val runningProcess = AtomicReference<Process?>(null)
private val compatibilityChecked = AtomicBoolean(false)
private val compatibilityFailure = AtomicReference<String?>(null)
private val lastNotification = AtomicReference<String?>(null)

private const val BAFT_PLUGIN_ID = "com.baft.intellij"
private const val BAFT_PROTOCOL_VERSION = 3

class BaftAnnotator : ExternalAnnotator<BaftAnnotatorInfo, List<BaftViolation>>() {

    override fun collectInformation(file: PsiFile): BaftAnnotatorInfo? {
        if (!file.isPhysical || file !== file.originalFile) return null
        if (file.viewProvider.getPsi(file.viewProvider.baseLanguage) !== file) return null
        val root = file.project.basePath ?: return null
        val path = file.virtualFile?.path ?: return null
        return BaftAnnotatorInfo(root, path, collectOverlayJson(root))
    }

    override fun doAnnotate(info: BaftAnnotatorInfo): List<BaftViolation> {
        ensureCompatible()?.let { message ->
            notifyError(message)
            return emptyList()
        }

        runningProcess.getAndSet(null)?.destroyForcibly()

        val process = try {
            val command = mutableListOf(findBinary(), "check", "--reporter=intellij")
            if (info.overlayJson != null) {
                command.add("--overlay-stdin")
            }
            command.add(".")
            val pb = ProcessBuilder(command)
                .directory(File(info.projectRoot))
            pb.environment()["PATH"] = augmentedPath()
            pb.start()
        } catch (e: IOException) {
            notifyError("BAFT: binary not found in PATH")
            return emptyList()
        }

        runningProcess.set(process)

        process.outputStream.bufferedWriter().use { writer ->
	        if (info.overlayJson != null) {
	            writer.write(info.overlayJson)
	        }
	    }

        val stderr = process.errorStream.bufferedReader()
        val stdoutText = process.inputStream.bufferedReader().readText()

        stderr.forEachLine { log.info("baft: $it") }

        process.waitFor()
        runningProcess.compareAndSet(process, null)

        return try {
            val all: List<BaftViolation> = gson.fromJson(stdoutText.trim(), violationType) ?: emptyList()
            all.filter { it.file == info.filePath }
        } catch (e: JsonSyntaxException) {
            log.warn("BAFT: failed to parse output: $stdoutText")
            emptyList()
        }
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

private fun ensureCompatible(): String? {
    if (compatibilityChecked.get()) return compatibilityFailure.get()

    synchronized(compatibilityChecked) {
        if (compatibilityChecked.get()) return compatibilityFailure.get()

        val process = try {
            val pb = ProcessBuilder(
                findBinary(),
                "integrate",
                "--verify-compatible",
                "--integration=jetbrains",
                "--plugin-version=${currentPluginVersion()}",
                "--protocol=$BAFT_PROTOCOL_VERSION",
            )
            pb.environment()["PATH"] = augmentedPath()
            pb.start()
        } catch (e: IOException) {
            compatibilityFailure.set("BAFT: binary not found in PATH")
            compatibilityChecked.set(true)
            return compatibilityFailure.get()
        }

        val stdoutText = process.inputStream.bufferedReader().readText().trim()
        val stderrText = process.errorStream.bufferedReader().readText().trim()
        process.waitFor()

        val report = try {
            if (stdoutText.isBlank()) null else gson.fromJson(stdoutText, BaftCompatibilityReport::class.java)
        } catch (_: JsonSyntaxException) {
            null
        }

        if (!report?.warning.isNullOrBlank()) {
            log.warn("BAFT: ${report?.warning}")
        }

        val failure = when {
            process.exitValue() == 0 && report?.compatible == true -> null
            !report?.message.isNullOrBlank() -> report?.message
            stderrText.isNotBlank() -> stderrText
            else -> "BAFT compatibility check failed"
        }

        compatibilityFailure.set(failure)
        compatibilityChecked.set(true)
        return failure
    }
}

private fun currentPluginVersion(): String {
    return PluginManagerCore.getPlugin(PluginId.getId(BAFT_PLUGIN_ID))?.version ?: "unknown"
}

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

private fun collectOverlayJson(projectRoot: String): String? {
    val fileDocumentManager = FileDocumentManager.getInstance()
    val rootPath = File(projectRoot).toPath().normalize()
    val files = fileDocumentManager.unsavedDocuments.mapNotNull { document ->
        val virtualFile = fileDocumentManager.getFile(document) ?: return@mapNotNull null
        val filePath = File(virtualFile.path).toPath().normalize()
        if (!filePath.startsWith(rootPath)) return@mapNotNull null
        BaftOverlayFile(virtualFile.path, document.text)
    }.distinctBy { it.path }
    if (files.isEmpty()) return null
    return gson.toJson(BaftOverlayPayload(files))
}
