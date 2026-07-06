# Specification for Go Translation of GNU `cat`

This document details the functional behavior, state requirements, and translation guidance for implementing the C-based GNU `cat` utility in Go.

---

## 1. User-Visible Behavior
`cat` reads the content of one or more input files sequentially and writes them to standard output.
* **Standard Input Handling:** If no files are specified, or if the filename is `-`, `cat` reads from standard input.
* **Diagnostics and Exit Status:** If a file cannot be opened, read, or closed, or if a write error occurs, the program prints an error message to standard error. The program exits with `EXIT_SUCCESS` (0) if all files were successfully processed, and `EXIT_FAILURE` (1) otherwise.
* **Same-File Collision Prevention:** `cat` prevents copying a regular file to itself (e.g., `cat foo >> foo`) by comparing input and output device/i-node configurations and active byte offsets to avoid infinite loops and disk exhaustion.

---

## 2. Option Flags and Output Modifications
If no formatting options are provided, `cat` performs a raw, unbuffered byte copy. When formatting options are enabled, output is modified as follows:

| Option Flag(s) | Long Option | Description |
| :--- | :--- | :--- |
| `-A` | `--show-all` | Equivalent to `-vET`. |
| `-b` | `--number-nonblank` | Numbers only non-empty output lines. Overrides `-n`. |
| `-e` | — | Equivalent to `-vE`. |
| `-E` | `--show-ends` | Appends `$` to the end of each line (directly before the newline character). Translates trailing Carriage Returns (`\r`) followed by line feeds (`\n`) to `^M$`. |
| `-n` | `--number` | Numbers all output lines (starts at 1). |
| `-s` | `--squeeze-blank` | Suppresses consecutive empty output lines so that no more than one consecutive blank line is printed. |
| `-t` | — | Equivalent to `-vT`. |
| `-T` | `--show-tabs` | Renders tab characters (`\t`) as `^I`. |
| `-u` | — | Ignored (output is always unbuffered/flushed immediately in modern implementations). |
| `-v` | `--show-nonprinting` | Converts non-printing bytes using caret (`^`) and meta (`M-`) notation, excluding newlines and tabs (unless `-T` is set). |

### Formatting Specifics:
* **Line Number Formatting:** Line numbers are formatted as right-aligned integers in a 6-character field, followed by a tab character (equivalent to `%6d\t`). The field alignment shifts left once the number exceeds `999999`.
* **Non-Printing Character Encoding (under `-v`):**
  * Control characters (bytes 0–31): Represented as `^` followed by the character code + 64 (e.g., byte 0 is `^@`, byte 1 is `^A`, excluding `\t` and `\n` unless explicitly requested by other flags).
  * Delete character (byte 127): Represented as `^?`.
  * Meta characters (bytes 128–255): Represented as `M-` followed by the representation of the lower 7 bits (e.g., byte 128 is `M-^@`, byte 255 is `M-^?`).

---

## 3. Persistent State Across Input Files
To maintain consistent output formatting across multiple input files, the following state variables must persist across the lifetime of the program execution (they cannot be reset between files):

1. **Line Counter:** The cumulative count of lines printed so far (for `-n` and `-b` numbering).
2. **Consecutive Newline Count (`newlines2`):** Tracks the number of consecutive newlines encountered at the end of the previous file to correctly compute blank-line squeezing (`-s`) and numbering (`-n`/`-b`) when transitioning to the next file.
3. **Pending Carriage Return (`pending_cr`):** Tracks whether a carriage return (`\r`) was read at the very end of an input block/file. If the subsequent file begins with a newline (`\n`), or if the program reaches EOF, this state dictates whether to format the sequence as `^M` or output a raw `\r`.

---

## 4. Non-Required C Optimizations
The original C implementation contains platform-specific and manual performance optimizations that are not required for semantic parity in Go. Go's runtime library and compiler handle these patterns idiomaticly:

* **Manual System-Level Copying (`splice` and `copy_file_range`):** The C implementation manually manages pipes and fallbacks for `splice` and `copy_file_range` system calls. In Go, these platform-level optimizations are handled transparently by the standard library's [io.Copy](https://pkg.go.dev/io#Copy) (via internal interfaces like `writeTo` and `readFrom`).
* **Input Pending Check (`ioctl` with `FIONREAD`):** Used to check for immediately available input to avoid blocking writes. Go's [bufio.Writer](https://pkg.go.dev/bufio#Writer) handles buffer flushing automatically and does not require manual I/O status checks.
* **Buffer Allocation and Alignment:** Page-aligned allocations (`xalignalloc`) and calculated safety margins (e.g., `insize * 4 + outsize + 20`) are unnecessary. Standard slice sizing and standard Go buffering are sufficient.
* **Sentinel Characters:** Modifying the input buffer to place a newline `\n` sentinel at the end of a read block to speed up character scanning loops can be replaced by normal Go range slices or boundary checks.

---

## 5. Mistranslation Risks and Edge Cases
Pay close attention to these scenarios during translation to Go to avoid subtle behavioral mismatches:

* **Same Inode / Cycle Detection:** 
  In Go, you must replicate the C logic that checks if the input file is the same file descriptor as standard output. Ensure you retrieve system-specific attributes (e.g., on Unix/Linux, `syscall.Stat_t`'s `Dev` and `Ino` fields via `FileInfo.Sys()`) and check active file offsets using `Seek` to determine if writing would overwrite active input.
* **Formatting Layout at Scale:** 
  The C version uses a manual byte incrementation routine (`next_line_num()`) over a fixed buffer (`line_buf`) to avoid `sprintf` calls. In Go, while `fmt.Fprintf` is simpler, it must preserve the exact padding constraints (right-justified in 6 spaces, transitioning to left-aligned after 6 digits).
* **Buffer-Boundary Carriage Returns (`pending_cr`):**
  If a block boundary or file transition falls exactly between a carriage return (`\r`) and a newline (`\n`), the parser must carry forward the `pending_cr` state rather than writing a raw `\r` immediately. At EOF, if `pending_cr` remains true, a final `\r` must be flushed to the output.
* **Empty Line Squeezing Boundary Transitions:**
  Ensure the consecutive newline count transitions smoothly across file boundaries. A trailing newline at the end of File A and a leading newline at the beginning of File B must count as two consecutive newlines for `-s` evaluation.