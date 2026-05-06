plugins {
    kotlin("multiplatform") version "2.1.0"
    id("com.android.library") version "8.2.0"
}

group = "com.example.app"
version = "1.0.0"

kotlin {
    jvm()
    androidTarget()
    iosArm64()
    iosSimulatorArm64()

    sourceSets {
        commonMain.dependencies {
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.1")
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
        }
        jvmMain.dependencies {
            implementation("com.squareup.okhttp3:okhttp:4.12.0")
        }
    }
}

android {
    namespace = "com.example.app"
    compileSdk = 34
}
