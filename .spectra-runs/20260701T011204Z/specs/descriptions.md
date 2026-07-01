## User-Visible Behavior

`cat` concatenates input files to standard output, or copies standard input when no files are given. By default it preserves bytes exactly and streams data as it reads it.

It also has a formatted mode that can number lines, show line endings, reveal tabs, and render nonprinting characters with caret/M- notation.

## Option Effects

- `-A`, `--show-all`: enable `-vET`
- `-b`, `--number-nonblank`: number only nonempty output lines; overrides `-n`
- `-e`: enable `-vE`
- `-E`, `--show-ends`: print `$` before each newline; also prints `^M$` for CRLF when applicable
- `-n`, `--number`: number all output lines
- `-s`, `--squeeze-blank`: collapse repeated empty output lines to one
- `-t`: enable `-vT`
- `-T`, `--show-tabs`: show tab as `^I`
- `-u`: ignored
- `-v`, `--show-nonprinting`: render nonprinting bytes as `^X` / `M-` forms, except tab and newline

## State That Must Persist Across Input Files

- Current line number counter
- Whether the previous input ended with a pending `\r` before `\n` handling in `-E` mode
- Blank-line squeeze state across file boundaries
- Any line-numbering state that determines whether the next line begins a new numbered line

## C-Specific Optimizations Not Needed for Go Semantic Parity

- `copy_file_range` fast path
- `splice` fast path
- `FIONREAD` polling optimization
- Block-size tuning, page alignment, and buffer reuse optimizations
- `O_BINARY` / `xset_binary_mode` handling, except where platform semantics require it
- Self-copy detection via inode/device/lseek checks, unless you want the same safety behavior

## Edge Cases Most Likely to Be Mistranslated

- CRLF handling with `-E`, especially when `\r` appears at buffer boundaries
- `-s` blank-line squeezing across file boundaries
- `-b` vs `-n` precedence
- Line-number formatting and carry behavior after many lines
- `-v` rendering of bytes >= 128, DEL, and control bytes
- Distinguishing literal tabs/newlines from quoted output
- Exact treatment of empty final input and inputs without trailing newline
- Error behavior when opening, reading, writing, or closing files fails
