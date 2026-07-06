## User-visible behavior

The program concatenates the named input files in order and writes their contents to standard output. A filename of `-` means standard input at that position. If no files are given, it copies standard input to standard output.

Without formatting options, output is just the raw byte stream. With formatting options, output may add line numbers, show line endings, show tabs, render nonprinting bytes visibly, and collapse runs of blank lines.

## Option effects

- `-n`, `--number`: prefix every output line with an incrementing line number.
- `-b`, `--number-nonblank`: prefix only nonempty output lines with line numbers; this overrides `-n` for blank lines.
- `-s`, `--squeeze-blank`: when multiple empty lines occur in a row, output only one empty line.
- `-E`, `--show-ends`: append `$` before each output newline. For CRLF input, show the `\r` as `^M` before `$`.
- `-T`, `--show-tabs`: render each tab as `^I`.
- `-v`, `--show-nonprinting`: render nonprinting bytes using caret notation and high-bit bytes using `M-` notation, but leave newline and tab unchanged unless another option changes them.
- `-e`: equivalent to `-vE`.
- `-t`: equivalent to `-vT`.
- `-A`, `--show-all`: equivalent to `-vET`.
- `-u`: ignored; no semantic effect.

## State that must persist across input files

- The current line number counter must continue across file boundaries.
- The “newline run” state used by `-n`, `-b`, and especially `-s` must continue across file boundaries, so a blank-line run can span multiple files.
- A pending trailing `\r` that was deferred while checking for a following `\n` must survive across file boundaries and be flushed later if no `\n` follows.
- Whether standard input was ever read matters only for final cleanup, not output semantics.

## C-specific optimizations not needed for Go semantic parity

- `copy_file_range`, `splice`, `ioctl(FIONREAD)`, pipe-size tuning, `fadvise`, block-size selection, page alignment, sentinel newlines, and manual buffer growth strategies are performance-only.
- Separate `simple_cat` vs optimized copy paths are not required if Go preserves the same visible output.
- Binary-mode toggling and low-level descriptor flags are platform-specific implementation details unless the Go target must match them explicitly.

## Likely mistranslation edge cases

- Blank-line squeezing across file boundaries.
- Interaction of `-b` and `-n`: `-b` still enables numbering but suppresses numbers on empty lines.
- Line numbering at the start of a file and immediately after newlines.
- Correct handling of CRLF when `--show-ends` is enabled, especially when `\r` and `\n` fall in different reads or different files.
- `-v` does not transform newline, and does not transform tab unless `-T` or `-t` is active.
- High-bit byte rendering under `-v` must be byte-oriented, not Unicode/rune-oriented.
- Output for byte `127` is `^?`; bytes `128..255` use `M-` forms with the same control-byte rules.
- Final pending `\r` must be emitted if it was not consumed as part of a shown CRLF ending.