Usage: strata check [flags] [root-dir]

Check packages for architecture violations.

Flags:
  --lang <name>       Language filter (can be repeated): go, typescript, dart, kotlin, rust
  --reporter=<name>   Output reporter: text (default), json, vsce, intellij
  --overlay-stdin     Read an unsaved-file overlay payload from stdin before checking
  --help, -h          Show this help
