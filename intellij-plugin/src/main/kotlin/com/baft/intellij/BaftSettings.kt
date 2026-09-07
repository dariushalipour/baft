package com.baft.intellij

import com.intellij.openapi.components.BaseState
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.SimplePersistentStateComponent
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.openapi.components.service

// Application-level state: settings live in the IDE config directory, never in
// project files, so a checked-out repository cannot point Baft at its own binary.
@Service(Service.Level.APP)
@State(name = "BaftSettings", storages = [Storage("baft.xml")])
class BaftSettings : SimplePersistentStateComponent<BaftSettings.State>(State()) {

    class State : BaseState() {
        var formatColorPalette by string("vibrant")
        var binaryPath by string("")
    }

    var formatColorPalette: String
        get() = state.formatColorPalette ?: "vibrant"
        set(value) {
            state.formatColorPalette = value
        }

    var binaryPath: String
        get() = state.binaryPath ?: ""
        set(value) {
            state.binaryPath = value
        }

    companion object {
        fun getInstance(): BaftSettings = service()
    }
}
