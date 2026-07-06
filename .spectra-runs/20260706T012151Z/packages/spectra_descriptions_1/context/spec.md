Here is the natural-language description of the `cat` program to guide the Go translation process:

# SPECTRA Go Translation Guide: `cat`

### 1. User-Visible Behavior
At a user-visible level, `cat` concatenates the content of one or more input files (or standard input if `-` or no files are specified) and writes the sequential stream of bytes to standard output. It supports various options to format the output (such as numbering lines, squeezing multiple empty lines, and displaying non-printing characters).

---

### 2. Supported Options and Output Changes
When format-oriented options are active, the stream is processed character-by-character:

*   **`-b`, `--number-nonblank`**: Prefixes non-empty output lines with a right-aligned line number (width 6, followed by a tab). Overrides `-n`.
*   **`-n`, `--number`**: Prefixes all output lines (empty or non-empty) with a right-aligned line number (width 6, followed by a tab).
*   **`-s`, `--squeeze-blank`**: Suppresses repeated adjacent empty output lines, replacing multiple consecutive empty lines with a single empty line.
*   **`-v`, `--show-nonprinting`**: Converts non-printing characters to visible representations using carets (`^`) and meta-notations (`M-`), except for Line Feed (`\n`) and Tab (`\t`).
    *   Control characters $0 \le C < 32$ (except `\n`, `\t`) are printed as `^` followed by the character code $+ 64$ (e.g., `\0` becomes `^@`).
    *   Delete ($127$) is printed as `^?`.
    *   Characters $C > 127$ are prefixed with `M-`. If $C - 128$ is a control character, it uses caret notation (e.g., $128$ becomes `M-^@`, $255$ becomes `M-^?`). Otherwise, it outputs the raw character $C - 128$.
*   **`-T`, `--show-tabs`**: Converts Tab (`\t`) characters into `^I`.
*   **`-E`, `--show-ends`**: Displays a `$` character at the end of each line (immediately preceding the newline character).
*   **`-e`**: Equivalent to `-vE`.
*   **`-t`**: Equivalent to `-vT`.
*   **`-A`, `--show-all`**: Equivalent to `-vET`.
*   **`-u`**: Ignored. (In Go, output is buffered or unbuffered depending on standard I/O structure, but behavior remains functionally unbuffered/line-buffered where needed).

---

### 3. Persisted State Across Input Files
To maintain correct output formatting across multiple files in a single execution, the following states **must** persist across input file boundaries:

1.  **Line Counter (`line_buf` and associated pointers)**: The current line number must not reset when transitioning to the next input file.
2.  **Consecutive Newline Count (`newlines2`)**: Tracks consecutive newlines (used by `-s` to squeeze empty lines and `-n`/`-b` to identify line starts). An empty line at the end of File A and an empty line at the start of File B must be treated as two consecutive empty lines.
3.  **Pending Carriage Return (`pending_cr`)**: A boolean flag indicating that a carriage return (`\r`) was read at the very end of a buffer/file, and its representation (`^M` or `\r`) is pending the evaluation of the next character (which could be the first character of the next file).
4.  **Standard Input Flag (`have_read_stdin`)**: Tracks whether standard input has been read during the execution block to handle cleanup/close operations.

---

### 4. C-Specific Optimizations Not Required in Go
The Go translation can skip these platform-specific and C-specific optimizations without losing semantic equivalence:

*   **`copy_file_range` and `splice` System Calls**: The C implementation bypasses standard buffers and uses zero-copy system calls for plain copying when no formatting options are selected. In Go, utilizing `io.Copy` or `io.CopyBuffer` is sufficient, as Go's runtime library already optimizes data transfer internally (e.g., via `ReadFrom` / `WriteTo` implementations).
*   **Buffer Sentinel Trick**: The C code appends a sentinel newline (`\n`) to the end of the read buffer to avoid out-of-bounds checks in the character-scanning loop. In Go, safe slice boundary checks make this manual pointer arithmetic and sentinel technique obsolete and dangerous; standard loops over byte slices should be used instead.
*   **Custom Page-Aligned Allocations**: Aligning I/O buffers to memory page boundaries using custom memory allocators (`ensure_buf_size`, `xalignalloc`) is handled transparently by Go's runtime and standard library allocation strategies.
*   **`FIONREAD` ioctl Check**: C checks if input bytes are immediately available to write buffered output before blocking on a read. Standard Go buffered writers (`bufio.Writer`) do not require this optimization for semantic correctness.

---

### 5. Translation Edge Cases and Pitfalls
Pay close attention to these scenarios during translation:

*   **Cross-Buffer / Cross-File `\r\n` Splits**: When `-E` (or any option implying it) is active, a carriage return (`\r`) followed by a newline (`\n`) is formatted as `^M$`. If the `\r` falls exactly on the last byte of a file/buffer and the `\n` is the first byte of the next, it must be properly concatenated. Ensure `pending_cr` logic is preserved.
*   **Same Input and Output File**: The program checks if an input file is the same physical file as the output (`SAME_INODE(istat_buf, ostat_buf)`) to prevent infinite recursive writes. In Go, retrieve `FileInfo` and compare the underlying OS-specific status structure (e.g., `syscall.Stat_t` device and inode numbers on Unix-like systems).
*   **Sequential empty lines split across files**: If File 1 ends with an empty line and File 2 starts with one, the `-s` flag must squeeze them. Ensure `newlines` is not reset to `0` or `-1` between files.
*   **Line Number Formatting formatting match**: The C code formats the line number as a 6-digit right-aligned number followed by a tab (conceptually `%6d\t`). Ensure the Go equivalent formats using `fmt.Sprintf("%6d\t", lineNum)` or manual fast formatting to match standard `cat` output formatting exactly.