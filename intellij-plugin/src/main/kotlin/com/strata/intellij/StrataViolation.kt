package com.strata.intellij

data class StrataViolation(
    val rule: String = "",
    val severity: String = "",
    val source: String = "",
    val message: String = "",
    val file: String = "",
    val line: Int = 0,
    val column: Int = 0,
    val columnEnd: Int = 0,
    val lineEnd: Int = 0,
)
