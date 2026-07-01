## User-Visible Behavior

`cat` copies each input file to standard output, in order. If no files are given, it reads from standard input. By default it preserves bytes as-is.

## Supported Options

- `-A`, `--show-all`: same as `-vET`
- `-b`, `--number-nonblank`: number only nonempty output lines, overrides `-n`
- `-e`: same as `-vE`
- `-E`, `--show-ends`: print `$` before each newline; on CRLF input it may print `^M$`
- `-n`, `--number`: number all output lines
- `-s`, `--squeeze-blank`: collapse repeated blank output lines to a single blank line
- `-t`: same as `-vT`
- `-T`, `--show-tabs`: print TAB as `^I`
- `-u`: ignored
- `-v`, `--show-nonprinting`: render nonprinting bytes as `^`/`M-` notation, except TAB and newline

## Persistent State Across Input Files

These values must carry across file boundaries:

- Current line number counter
- Whether the previous file ended with a pending `\r` before a following `\n` (`pending_cr`)
- Whether the program has already read from standard input

For semantic parity, line numbering and blank-line squeezing must continue across files as one stream, not reset per file.

## C-Specific Optimizations Not Required in Go

The Go version does not need to preserve these implementation details:

- `copy_file_range` fast path
- `splice` fast path
- `FIONREAD` ioctl optimization
- Buffer alignment and page-size tuning
- Sentinel newline at end of input buffer
- Manual `full_write` retry loops and low-level descriptor juggling
- `O_BINARY` / platform-specific binary mode toggles

## Edge Cases Most Likely to Be Mistranslated

- CRLF handling with `-E`, especially a `\r` at buffer end followed by `\n` in the next read
- `-b` vs `-n` precedence
- `-s` counting blank lines across file boundaries
- Numbering only at the start of a line, including after empty lines
- `-v` byte-quoting rules for bytes `>= 128`
- Output buffering behavior when options are enabled
- Self-copy detection when input and output refer to the same file
- Special handling of stdin as `-`
- The fact that `-u` is accepted but does nothing
