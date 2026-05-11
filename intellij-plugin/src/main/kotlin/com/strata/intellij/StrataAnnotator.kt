package com.strata.intellij

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import com.google.gson.reflect.TypeToken
import com.intellij.lang.annotation.AnnotationHolder
import com.intellij.lang.annotation.ExternalAnnotator
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.editor.Document
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiFile
import java.io.IOException
import java.io.File
import java.util.concurrent.atomic.AtomicReference

data class StrataAnnotatorInfo(val projectRoot: String, val filePath: String, val overlayJson: String?)
data class StrataOverlayFile(val path: String, val content: String)
data class StrataOverlayPayload(val files: List<StrataOverlayFile>)

private val log = Logger.getInstance(StrataAnnotator::class.java)
private val gson = Gson()
private val violationType = object : TypeToken<List<StrataViolation>>() {}.type
private val runningProcess = AtomicReference<Process?>(null)

class StrataAnnotator : ExternalAnnotator<StrataAnnotatorInfo, List<StrataViolation>>() {

    override fun collectInformation(file: PsiFile): StrataAnnotatorInfo? {
        if (!file.isPhysical || file !== file.originalFile) return null
        if (file.viewProvider.getPsi(file.viewProvider.baseLanguage) !== file) return null
        val root = file.project.basePath ?: return null
        val path = file.virtualFile?.path ?: return null
        return StrataAnnotatorInfo(root, path, collectOverlayJson(root))
    }

    override fun doAnnotate(info: StrataAnnotatorInfo): List<StrataViolation> {
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
            ApplicationManager.getApplication().invokeLater {
                NotificationGroupManager.getInstance()
                    .getNotificationGroup("STRATA")
                    .createNotification(
                        "STRATA: binary not found in PATH",
                        NotificationType.ERROR,
                    )
                    .notify(null)
            }
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

        stderr.forEachLine { log.info("strata: $it") }

        process.waitFor()
        runningProcess.compareAndSet(process, null)

        return try {
            val all: List<StrataViolation> = gson.fromJson(stdoutText.trim(), violationType) ?: emptyList()
            all.filter { it.file == info.filePath }
        } catch (e: JsonSyntaxException) {
            log.warn("STRATA: failed to parse output: $stdoutText")
            emptyList()
        }
    }

    override fun apply(file: PsiFile, violations: List<StrataViolation>, holder: AnnotationHolder) {
        if (violations.isEmpty()) return
        val doc = FileDocumentManager.getInstance().getDocument(file.virtualFile) ?: return

        for (v in violations) {
            val range = toTextRange(doc, v) ?: continue
            holder.newAnnotation(toHighlightSeverity(v.severity), v.message)
                .range(range)
                .tooltip("[strata] ${v.rule}: ${v.message}")
                .create()
        }
    }

    private fun toTextRange(doc: Document, v: StrataViolation): TextRange? {
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

private fun collectOverlayJson(projectRoot: String): String? {
    val fileDocumentManager = FileDocumentManager.getInstance()
    val rootPath = File(projectRoot).toPath().normalize()
    val files = fileDocumentManager.unsavedDocuments.mapNotNull { document ->
        val virtualFile = fileDocumentManager.getFile(document) ?: return@mapNotNull null
        val filePath = File(virtualFile.path).toPath().normalize()
        if (!filePath.startsWith(rootPath)) return@mapNotNull null
        StrataOverlayFile(virtualFile.path, document.text)
    }.distinctBy { it.path }
    if (files.isEmpty()) return null
    return gson.toJson(StrataOverlayPayload(files))
}

private fun findBinary(): String {
    val os = System.getProperty("os.name").lowercase()
    val isWin = os.contains("win")
    val name = if (isWin) "strata.exe" else "strata"
    val sep = if (isWin) ";" else ":"
    return augmentedPath().split(sep)
        .map { java.io.File(it, name) }
        .firstOrNull { it.canExecute() }
        ?.absolutePath ?: name
}

private fun augmentedPath(): String {
    val current = System.getenv("PATH") ?: ""
    val home = System.getProperty("user.home") ?: return current
    val os = System.getProperty("os.name").lowercase()
    val extras = when {
        os.contains("win") -> listOf(
            "$home\\go\\bin",
            "$home\\AppData\\Local\\Programs\\Go\\bin",
            "C:\\Go\\bin",
            "C:\\Program Files\\Go\\bin",
        )
        os.contains("mac") -> listOf(
            "$home/go/bin",
            "$home/.local/bin",
            "/usr/local/go/bin",
            "/opt/homebrew/bin",
            "/usr/local/bin",
        )
        else -> listOf( // Linux
            "$home/go/bin",
            "$home/.local/bin",
            "/usr/local/go/bin",
            "/usr/local/bin",
            "/snap/bin",
        )
    }
    val separator = if (os.contains("win")) ";" else ":"
    val parts = current.split(separator).toMutableList()
    for (extra in extras.reversed()) {
        if (extra !in parts) parts.add(0, extra)
    }
    return parts.joinToString(separator)
}
