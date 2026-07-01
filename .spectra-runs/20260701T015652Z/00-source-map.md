# `cat.c` Source Map

## Global State
- `infile`: current input name for diagnostics.
- `input_desc`: active input file descriptor for the current loop iteration.
- `line_buf` / `line_num_*`: mutable line-number formatting buffer for `-n` / `-b`.
- `newlines2`: preserves newline-state across file boundaries, so options like `-s` and numbering behave consistently when processing multiple inputs.
- `pending_cr`: tracks a trailing `\r` before `\n` split across buffer boundaries when `-E` is active.
- `have_read_stdin`: used to close stdin at exit if it was consumed.
- `PROGRAM_NAME`, `AUTHORS`: metadata for help/version output.

## Functions

### `usage(int status)`
- Prints help/version text and exits.
- Uses coreutils localization and shared help/version helpers.

### `next_line_num(void)`
- Advances the mutable line-number buffer in place.
- Handles carry, widening the visible number field when needed.

### `simple_cat(char *buf, idx_t bufsize)`
- Plain read/write loop for unformatted copying.
- Used when no display/numbering options are active and optimized paths are unavailable.

### `write_pending(char *outbuf, char **bpout)`
- Flushes buffered output if any.
- Shared helper for the formatted path.

### `cat(char *inbuf, idx_t insize, char *outbuf, idx_t outsize, bool show_nonprinting, bool show_tabs, bool number, bool number_nonblank, bool show_ends, bool squeeze_blank)`
- Main formatted transformation engine.
- Implements `-v`, `-E`, `-T`, `-n`, `-b`, `-s`, including newline handling, line numbering, tab quoting, and CRLF edge cases.
- Preserves state across buffer refills and across files via `newlines2` and `pending_cr`.

### `copy_cat(void)`
- Fast path using `copy_file_range`.
- Falls back when unsupported or when behavior is suspicious/limited.
- Returns tri-state: success, try slower path, or fatal error.

### `splice_cat(void)`
- Fast path using `splice` via an internal pipe.
- Distinguishes input/output errors and can fall back to read/write.
- Keeps static pipe state across calls.

### `ensure_buf_size(char *buf, idx_t *buf_alloc, idx_t alignment, idx_t size)`
- Reuses or reallocates aligned buffers.
- Centralizes growth logic for input/output buffers.

### `main(int argc, char **argv)`
- Parses options, opens each input, checks self-copy hazards, selects fast path vs formatted path, and manages cleanup.
- Also handles stdout mode, stdin binary mode, and final pending CR flush.

## Simplifiable in Go
- Replace manual buffer mutation with slices and `bytes.Buffer`/`bufio.Writer`.
- Replace line-number buffer arithmetic with formatted integers.
- Replace `copy_file_range` / `splice` optimization chain with a simpler streaming copy unless performance parity is required.
- Replace `fstat`, `lseek`, `fcntl`, and inode/self-copy checks with Go `os.FileInfo`-based equivalents where possible.
- Replace sentinel-byte buffer scanning with explicit index-based loops.

## Translation Risks to Test
- `-n` vs `-b` precedence and numbering across blank lines.
- `-s` squeezing behavior across file boundaries.
- `-E` with CRLF and partial-buffer `\r` at chunk edges.
- `-T`, `-v`, and `-A` quoting semantics for control/non-ASCII bytes.
- Self-copy detection when input and output refer to the same file.
- Mixed fast-path and fallback behavior for regular files, pipes, procfs, and unsupported syscalls.
- Preservation of exact newline/EOF behavior for empty files and files without trailing newline.
- Unicode/byte handling: this program is byte-oriented, not text-oriented.
