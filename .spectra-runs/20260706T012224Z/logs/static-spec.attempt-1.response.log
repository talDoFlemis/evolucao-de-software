# Static Specifications for translating `cat.c` to Go

## Program Input/Output Contract

### Inputs
* **Command-line arguments (`os.Args`)**: Parsed to extract options and positional file paths.
* **Standard Input (`os.Stdin`)**: Read if no file paths are provided, or if the file path is `"-"`.
* **Files**: Opened in read-only mode (`os.O_RDONLY`).

### Outputs
* **Standard Output (`os.Stdout`)**: The concatenated (and potentially formatted) byte streams of all input sources.
* **Standard Error (`os.Stderr`)**: Diagnostic messages on open, read, write, or close failures.

### Exit Code
* **`0` (Success)**: If all files were read and written to `stdout` successfully.
* **`1` (Failure)**: If any file open/read error occurred, or if writing to `stdout` failed.

### Constraints / Safety Checks
* **Self-Copy Prevention**: Before reading a file, check if it is a regular file and `stdout` is also a regular file. If they have the same device and inode (or Go equivalent: `os.SameFile(istat, ostat)`), the program must print an error (`"input file is output file"`), skip the file, and ensure a non-zero exit status.

---

## Option Parsing Contract

### Command-line Option Mapping
* **`-b` / `--number-nonblank`**: Sets `number = true` and `number_nonblank = true`.
* **`-e`**: Sets `show_ends = true` and `show_nonprinting = true`.
* **`-E` / `--show-ends`**: Sets `show_ends = true`.
* **`-n` / `--number`**: Sets `number = true`.
* **`-s` / `--squeeze-blank`**: Sets `squeeze_blank = true`.
* **`-t`**: Sets `show_tabs = true` and `show_nonprinting = true`.
* **`-T` / `--show-tabs`**: Sets `show_tabs = true`.
* **`-u`**: Ignored (Go stdout is buffered standardly; matching unbuffered behavior).
* **`-v` / `--show-nonprinting`**: Sets `show_nonprinting = true`.
* **`-A` / `--show-all`**: Sets `show_nonprinting = true`, `show_ends = true`, and `show_tabs = true`.

### Override Precedence
* If both `-b` and `-n` are specified, `-b` overrides `-n` (`number_nonblank = true` wins).

---

## Preconditions and Postconditions for `simple_cat` Behavior

### Trigger Condition
`!(number || show_ends || show_nonprinting || show_tabs || squeeze_blank)`

### Preconditions
* The input file descriptor/stream is valid and readable.
* `os.Stdout` is writable.

### Postconditions
* Verbatim transfer of all bytes from the input source to `os.Stdout`.
* In Go, this maps directly to `io.Copy(os.Stdout, inputFile)`.
* Returns `true` if EOF is reached with zero read/write errors; `false` otherwise.

---

## Preconditions and Postconditions for Formatted `cat` Behavior

### Trigger Condition
`number || show_ends || show_nonprinting || show_tabs || squeeze_blank`

### Preconditions
* The input file descriptor/stream is valid and readable.
* `os.Stdout` is writable.
* State variables $N$ (consecutive newlines, initialized to `0`) and $CR_{pending}$ (pending CR flag, initialized to `false`) are preserved across multiple file reads.

### Postconditions
* Processed and formatted bytes are written sequentially to `os.Stdout`.
* State variables $N$ and $CR_{pending}$ are updated dynamically.
* Returns `true` if EOF is reached with zero read/write errors; `false` otherwise.
* **Termination Postcondition**: If $CR_{pending} == \text{true}$ at the end of the final file, a trailing `\r` is written to `os.Stdout`.

---

## Invariants for Formatting Behavior

### Squeeze Blank (`squeeze_blank`)
Let $N$ be the consecutive newline counter:
* $N$ is initialized to $0$ at program start, or persists from the previous file.
* Upon reading `\n`:
  * Increment $N$. If $N > 2$, set $N = 2$.
  * If `squeeze_blank` is true and $N == 2$, discard the current `\n` (do not output it, its line number, or its line end).
  * Else, print `\n`.
* Upon reading any non-`\n` character:
  * Reset $N = -1$ (so that the next `\n` sets $N = 0$).

### Line Numbering (`number`, `number_nonblank`)
Let $L_c$ be the line number counter (initialized to 1):
* A line number prefix is formatted as a 6-character right-aligned integer followed by a tab (e.g., `fmt.Sprintf("%6d\t", L_c)`).
* **If `number` is true and `number_nonblank` is false**:
  * Print the line number prefix (and increment $L_c$) at the beginning of the stream and immediately after every non-squeezed output `\n`.
* **If `number_nonblank` is true**:
  * Print the line number prefix (and increment $L_c$) only immediately before writing the first non-`\n` character of a line.

### Show Tabs (`show_tabs`)
* If `show_tabs` is true:
  * Output `^I` for every tab (`\t`) character.
* If `show_tabs` is false:
  * Output `\t` character as-is.

### Show Ends (`show_ends`)
* If `show_ends` is true:
  * Output `$` immediately before every output `\n`.
  * If a carriage return (`\r`) is followed by `\n` in the input stream, output `^M$`.
    * If `\r` is the last byte of an input block, set $CR_{pending} = \text{true}$ to defer processing until the next byte is read.

### Show Nonprinting (`show_nonprinting`)
If `show_nonprinting` is true, convert each input byte $c \in [0, 255]$:
* **For $c \ge 128$**:
  * Write `M-`.
  * Let $c' = c - 128$.
  * If $c' \ge 32$:
    * If $c' < 127$: output $c'$ as-is.
    * If $c' == 127$: output `^?`.
  * If $c' < 32$: output `^` followed by $c' + 64$.
* **For $c < 128$**:
  * If $c \ge 32$:
    * If $c < 127$: output $c$ as-is.
    * If $c == 127$: output `^?`.
  * If $c < 32$:
    * If $c == \text{TAB}$ (`\t`) and `show_tabs` is false: output `\t`.
    * If $c == \text{LFD}$ (`\n`): handled by line/ends logic (ends inner loop).
    * Else: output `^` followed by $c + 64$.

---

## Explicit Equivalences

* **`-A` (equivalent to `-vET`)**:
  * `show_nonprinting = true`
  * `show_ends = true`
  * `show_tabs = true`
* **`-e` (equivalent to `-vE`)**:
  * `show_nonprinting = true`
  * `show_ends = true`
* **`-t` (equivalent to `-vT`)**:
  * `show_nonprinting = true`
  * `show_tabs = true`
* **`-u` (ignored)**:
  * No-op. Output streaming does not perform custom internal buffering.