package com.baft.intellij

import com.intellij.openapi.components.BaseState
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.SimplePersistentStateComponent
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.openapi.components.service

@Service(Service.Level.APP)
@State(name = "BaftSettings", storages = [Storage("baft.xml")])
class BaftSettings : SimplePersistentStateComponent<BaftSettings.State>(State()) {

    class State : BaseState() {
        var formatColorPalette by string("vibrant")
    }

    var formatColorPalette: String
        get() = state.formatColorPalette ?: "vibrant"
        set(value) {
            state.formatColorPalette = value
        }

    companion object {
        fun getInstance(): BaftSettings = service()
    }
}
