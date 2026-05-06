package com.example.app.presentation

import com.example.app.shared.ConsoleLogger
import com.example.app.shared.Logger

object Main {
    @JvmStatic
    fun main(args: Array<String>) {
        val logger: Logger = ConsoleLogger()
        logger.info("Application starting", mapOf("version" to "1.0.0"))
        logger.info("Application started successfully")
    }
}
