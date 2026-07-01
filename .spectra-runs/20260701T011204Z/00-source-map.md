# Source Map: `cat.c`

## Global State
- `infile`: current input name for diagnostics and error messages.
- `input_desc`: active input file descriptor for the current file/stdin.
- `line_buf` + `line_num_print/start/end`: mutable line-number formatter state for `-n`/`-b`.
- `newlines2`: carries newline state across files so numbering/squeezing behaves continuously.
- `pending_cr`: defers a `\r` when `\r\n` crosses buffer boundaries and `-E` is active.
- External state from support libs:
  - `program_name`, localization helpers, `close_stdout`, `errno`-based error reporting.
  - `xset_binary_mode`, `fdadvise`, `io_blksize`, `copy_file_range`, `splice`, etc.

## Functions
- `usage(status)`: prints help/version text and exits.
- `next_line_num()`: increments the shared line-number buffer in-place.
- `simple_cat(buf, bufsize)`: plain read/write copy loop for one file.
- `write_pending(outbuf, bpout)`: flushes buffered output bytes.
- `cat(...)`: format-aware streaming path for `-n/-b/-s/-v/-E/-T`.
- `copy_cat()`: fast path using `copy_file_range` when possible.
- `splice_cat()`: fast path using `splice`/pipe buffering when possible.
- `ensure_buf_size(...)`: allocate/reuse aligned buffers across inputs.
- `main(argc, argv)`: option parsing, file iteration, path selection, cleanup, exit status.

## C/Coreutils Details to Simplify in Go
- Replace descriptor-centric control flow with `io.Reader`/`io.Writer` loops.
- Collapse `copy_cat`/`splice_cat`/`simple_cat` into a small set of Go copy strategies, or just use buffered copy unless benchmarking requires fast paths.
- Replace mutable global line-number buffer with a small formatter struct.
- Replace `FIONREAD`, `splice`, and `copy_file_range` heuristics with portable Go code unless platform-specific optimization is needed.
- Binary mode toggles and `fdadvise` are mostly Unix/C concerns; Go can usually ignore them.
- Manual aligned buffer reuse can be simplified to normal byte slices unless performance requires reuse.

## Translation Risks To Test
- `-n` vs `-b` precedence and line numbering across multiple files/stdin.
- `-s` squeezing repeated blank lines, including file boundaries.
- `-E`, `-T`, `-v`, and `-A` output formatting, especially `\r\n` handling.
- Buffer-boundary edge cases for newline sentinel logic and `pending_cr`.
- Self-copy detection when input and output are the same file.
- Fallback behavior when fast paths fail or are unsupported.
- Error propagation and exit status when one file fails but others succeed.
- Stdout/stderr flushing and interleaving of output with diagnostics.
