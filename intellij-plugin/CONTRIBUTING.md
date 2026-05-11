# BAFT — IntelliJ Plugin

Shows BAFT architecture violations as red squiggles and Problems tool window entries by running the BAFT CLI and mapping its output to IntelliJ annotations.

## Prerequisites

- JDK 17+
- `baft` binary available in your `PATH`
- IntelliJ IDEA 2024.1+

## How it works

The plugin uses IntelliJ's `ExternalAnnotator` pipeline to run `baft check --reporter=intellij .` from the project root. When there are unsaved files in the project, it adds `--overlay-stdin` and streams the current in-memory document contents to the CLI so annotations track unsaved edits.

The plugin parses the JSON array on stdout and publishes `ExternalAnnotator` annotations. No architecture logic lives in the plugin — the CLI is the single source of truth.

## Local development

Open the `intellij-plugin/` directory in IntelliJ IDEA. The project uses the standard Gradle IntelliJ Plugin setup.

Run a sandboxed IDE with the plugin loaded:

```bash
./gradlew runIde
```

Open any project containing a `BAFT.md` manifest and edit a file — violations will appear as red squiggles and in the Problems tool window, including unsaved changes.

## Build a distributable ZIP

```bash
./gradlew buildPlugin
```

This produces a `.zip` under `build/distributions/`.

## Install the ZIP manually

In IntelliJ IDEA:

1. Open **Settings → Plugins**
2. Click the gear icon and choose **Install Plugin from Disk…**
3. Select the `.zip` from `build/distributions/`

## Publish to JetBrains Marketplace

Follow the [JetBrains publishing guide](https://plugins.jetbrains.com/docs/intellij/publishing-plugin.html):

```bash
./gradlew publishPlugin
```

Requires a `PUBLISH_TOKEN` environment variable with a JetBrains Marketplace token.
