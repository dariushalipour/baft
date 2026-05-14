package com.baft.intellij

import com.intellij.formatting.service.AsyncDocumentFormattingService
import com.intellij.formatting.service.AsyncFormattingRequest
import com.intellij.formatting.service.FormattingService
import com.intellij.openapi.diagnostic.Logger
import com.intellij.psi.PsiFile
import java.io.IOException
import java.util.concurrent.atomic.AtomicReference

private val formatterLog = Logger.getInstance(BaftFormattingService::class.java)
private const val BAFT_FORMATTER_NAME = "BAFT"

class BaftFormattingService : AsyncDocumentFormattingService() {

    override fun getFeatures(): Set<FormattingService.Feature> = setOf(FormattingService.Feature.AD_HOC_FORMATTING)

    override fun canFormat(file: PsiFile): Boolean = file.name == "BAFT.md"

    override fun createFormattingTask(request: AsyncFormattingRequest): FormattingTask {
        return object : FormattingTask {
            private val runningProcess = AtomicReference<Process?>(null)

            override fun run() {
                if (request.canChangeWhitespaceOnly()) {
                    request.onTextReady(request.documentText)
                    return
                }

                val filePath = request.getIOFile()?.path
                    ?: request.getContext().getVirtualFile()?.path
                    ?: request.getContext().getContainingFile().virtualFile?.path
                    ?: "BAFT.md"

                val process = try {
                    val command = listOf(
                        findBinary(),
                        "restyle",
                        "--stdin",
                        "--path=$filePath",
                        "--color-palette=${BaftSettings.getInstance().formatColorPalette}",
                    )
                    val pb = ProcessBuilder(command)
                    pb.environment()["PATH"] = augmentedPath()
                    pb.start()
                } catch (e: IOException) {
                    request.onError("BAFT: binary not found in PATH", e.message ?: "")
                    return
                }

                runningProcess.set(process)

                try {
                    process.outputStream.bufferedWriter().use { writer ->
                        writer.write(request.documentText)
                    }

                    val stdoutText = process.inputStream.bufferedReader().readText()
                    val stderrText = process.errorStream.bufferedReader().readText().trim()

                    if (stderrText.isNotBlank()) {
                        formatterLog.info("baft: $stderrText")
                    }

                    process.waitFor()
                    if (process.exitValue() != 0) {
                        request.onError(
                            "BAFT restyle failed",
                            stderrText.ifBlank { "Formatter exited with code ${process.exitValue()}" },
                            process.exitValue(),
                        )
                        return
                    }

                    request.onTextReady(stdoutText)
                } catch (e: Exception) {
                    request.onError("BAFT restyle failed", e.message ?: "unknown error")
                } finally {
                    runningProcess.compareAndSet(process, null)
                }
            }

            override fun cancel(): Boolean {
                runningProcess.getAndSet(null)?.destroyForcibly()
                return true
            }
        }
    }

    override fun getNotificationGroupId(): String = BAFT_NOTIFICATION_GROUP_ID

    override fun getName(): String = BAFT_FORMATTER_NAME
}
