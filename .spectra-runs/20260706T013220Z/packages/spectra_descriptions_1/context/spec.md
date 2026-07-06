## User-visible behavior

Concatenates files in argument order to standard output. `-` means standard input; with no files, input comes from standard input. Data is copied unchanged unless formatting options are enabled. File errors are reported, remaining files are still processed, and any error produces a nonzero exit status.

## Options

- `-n`, `--number`: Prefix every output line, including blank lines, with a right-aligned line number and tab.
- `-b`, `--number-nonblank`: Number only nonempty lines. Overrides `-n`, regardless of option order.
- `-s`, `--squeeze-blank`: Replace each run of multiple empty lines with one empty line.
- `-E`, `--show-ends`: Write `$` before each LF. A CR immediately before LF is shown as `^M$`.
- `-T`, `--show-tabs`: Render TAB as `^I`.
- `-v`, `--show-nonprinting`: Render control and high-bit bytes using `^` and `M-` notation, except LF and TAB.
- `-A`, `--show-all`: Equivalent to `-vET`.
- `-e`: Equivalent to `-vE`.
- `-t`: Equivalent to `-vT`.
- `-u`: Accepted but ignored.
- `--help`, `--version`: Print information and exit.

Transformations are byte-oriented, not Unicode-aware.

## State across input files

Files form one continuous byte stream for formatting purposes:

- Line numbering continues across files.
- Whether output is currently at the start of a line persists.
- Consecutive-newline state persists, so `-s` can squeeze blank lines across a file boundary.
- A pending CR detected at the end of an internal buffer or file persists until the following byte is known.
- Repeated `-` operands reuse the current standard-input position rather than restarting it.

Do not insert separators or final newlines between files.

## C optimizations not required in Go

Semantic parity does not require:

- `copy_file_range` or `splice`
- `FIONREAD` polling and pre-wait flushing
- Page-aligned or reusable buffers
- Filesystem block-size tuning
- Sentinel newlines
- Manual output-buffer expansion calculations
- Pipe-size adjustments
- Sequential-access advice
- The fixed-width mutable decimal counter implementation

A buffered streaming loop is sufficient, provided state is preserved across reads and files and write errors are handled.

## Likely mistranslation edge cases

- Treating each file independently, resetting numbering, line-start, squeezing, or CR state.
- Numbering a final unterminated line incorrectly; it is numbered when its first byte is emitted.
- Emitting a number for an empty input.
- Confusing an empty line with an unterminated empty file.
- Applying `-s` per file instead of across file boundaries.
- Making `-n` override an earlier `-b`; `-b` always wins once present.
- Implementing `-v` over Unicode code points instead of raw bytes.
- Escaping TAB under `-v` alone; TAB remains literal unless `-T` or `-t` is active.
- Mishandling CRLF split across buffers or files with `-E`.
- Adding separators or a trailing newline.
- Stopping after the first input error instead of continuing.
- Copying an input regular file onto the same output file and allowing unbounded self-appending.