Usage: baft <command> [arguments]

Commands:
  check [flags] [root-dir]   Check packages for architecture violations
  draft [root-dir]           Generate BAFT.md from current dependency reality
  manual                     Print the BAFT.md manual for working in BAFT-governed code

Flags:
  --version, -v               Print version
  --help, -h                  Show this help

Run 'baft <command> --help' for more information about a command.

Exit codes:
  0  No violations
  1  Violations found or error
