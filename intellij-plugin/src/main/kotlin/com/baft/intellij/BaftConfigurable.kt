package com.baft.intellij

import com.intellij.openapi.options.Configurable
import com.intellij.openapi.options.SearchableConfigurable
import com.intellij.openapi.ui.ComboBox
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.util.ui.FormBuilder
import java.awt.BorderLayout
import javax.swing.JComponent
import javax.swing.JPanel

class BaftConfigurable : SearchableConfigurable, Configurable.NoScroll {
    private var panel: JPanel? = null
    private var paletteCombo: ComboBox<String>? = null
    private var binaryField: TextFieldWithBrowseButton? = null

    override fun getId(): String = "com.baft.intellij.BaftConfigurable"

    override fun getDisplayName(): String = "🧶 Baft"

    override fun createComponent(): JComponent {
        val combo = ComboBox(arrayOf("vibrant", "muted", "mono", "none"))
        paletteCombo = combo
        val binary = TextFieldWithBrowseButton()
        binary.toolTipText = "Path to the baft executable. Leave empty to use baft from PATH."
        binaryField = binary

        val content = FormBuilder.createFormBuilder()
            .addLabeledComponent("Baft executable:", binary)
            .addLabeledComponent("Formatter color palette:", combo)
            .panel

        return JPanel(BorderLayout()).also {
            it.add(content, BorderLayout.NORTH)
            panel = it
        }
    }

    private fun selectedPalette(): String = paletteCombo?.selectedItem as? String ?: "vibrant"

    private fun enteredBinaryPath(): String = binaryField?.text?.trim() ?: ""

    override fun isModified(): Boolean {
        val settings = BaftSettings.getInstance()
        return selectedPalette() != settings.formatColorPalette || enteredBinaryPath() != settings.binaryPath
    }

    override fun apply() {
        val settings = BaftSettings.getInstance()
        settings.formatColorPalette = selectedPalette()
        settings.binaryPath = enteredBinaryPath()
    }

    override fun reset() {
        val settings = BaftSettings.getInstance()
        paletteCombo?.selectedItem = settings.formatColorPalette
        binaryField?.text = settings.binaryPath
    }

    override fun disposeUIResources() {
        panel = null
        paletteCombo = null
        binaryField = null
    }
}
