package com.baft.intellij

import com.intellij.openapi.options.Configurable
import com.intellij.openapi.options.SearchableConfigurable
import com.intellij.openapi.ui.ComboBox
import com.intellij.util.ui.FormBuilder
import java.awt.BorderLayout
import javax.swing.JComponent
import javax.swing.JPanel

class BaftConfigurable : SearchableConfigurable, Configurable.NoScroll {
    private var panel: JPanel? = null
    private var paletteCombo: ComboBox<String>? = null

    override fun getId(): String = "com.baft.intellij.BaftConfigurable"

    override fun getDisplayName(): String = "🧶 Baft"

    override fun createComponent(): JComponent {
        val combo = ComboBox(arrayOf("vibrant", "muted", "mono", "none"))
        paletteCombo = combo

        val content = FormBuilder.createFormBuilder()
            .addLabeledComponent("Formatter color palette:", combo)
            .panel

        return JPanel(BorderLayout()).also {
            it.add(content, BorderLayout.NORTH)
            panel = it
        }
    }

    override fun isModified(): Boolean {
        return (paletteCombo?.selectedItem as? String ?: "vibrant") != BaftSettings.getInstance().formatColorPalette
    }

    override fun apply() {
        BaftSettings.getInstance().formatColorPalette = paletteCombo?.selectedItem as? String ?: "vibrant"
    }

    override fun reset() {
        paletteCombo?.selectedItem = BaftSettings.getInstance().formatColorPalette
    }

    override fun disposeUIResources() {
        panel = null
        paletteCombo = null
    }
}
