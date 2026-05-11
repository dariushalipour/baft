#!/bin/bash

cd "$(dirname "$0")"
cd ../vscode-extension && npm install && npm run compile && vsce package && code --install-extension *.vsix
cd ../intellij-plugin && ./gradlew buildPlugin
