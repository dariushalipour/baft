package com.strata.intellij

import com.intellij.openapi.editor.Document
import com.intellij.openapi.fileEditor.FileDocumentManagerListener
import com.intellij.openapi.project.ProjectManager
import com.intellij.psi.PsiDocumentManager
import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer

class StrataSaveListener : FileDocumentManagerListener {
    override fun beforeDocumentSaving(document: Document) {
        for (project in ProjectManager.getInstance().openProjects) {
            val psiFile = PsiDocumentManager.getInstance(project).getPsiFile(document) ?: continue
            DaemonCodeAnalyzer.getInstance(project).restart(psiFile)
        }
    }
}
