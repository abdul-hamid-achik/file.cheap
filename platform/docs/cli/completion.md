# completion

Generate a shell completion script for fcheap commands, arguments, and flags.

## Usage

```bash
fcheap completion [bash|zsh|fish|powershell]
```

Exactly one supported shell name is required.

## Bash

Load completion for the current shell:

```bash
source <(fcheap completion bash)
```

For a persistent system installation, write the generated script to a directory
loaded by your Bash completion setup. This location varies by operating system
and package manager.

## Zsh

Write the completion function into the first configured completion directory:

```zsh
fcheap completion zsh > "${fpath[1]}/_fcheap"
```

Start a new shell or rebuild the completion cache if your setup requires it.

## Fish

```fish
fcheap completion fish > ~/.config/fish/completions/fcheap.fish
```

Fish loads scripts from that directory automatically in future sessions.

## PowerShell

Generate the script and load it into the current session:

```powershell
fcheap completion powershell | Out-String | Invoke-Expression
```

To make it persistent, save the generated script and source it from your
PowerShell profile.

## Output

The command writes only the generated shell script to stdout, so it can be
piped, sourced, or redirected. It does not change your shell configuration by
itself.
