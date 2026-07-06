# Static Specification for Translating `cat.c` to Go

## Program Input/Output Contract

* **Inputs**:
  * Command-line arguments (`os.Args`).
  * Standard Input (`os.Stdin`), read if no file arguments are provided, or if the argument is `"-"`.
  * Target files specified as path strings.
* **Outputs**:
  * Standard Output (`os.Stdout`) receives the concatenated and formatted byte stream.
  * Standard Error (`os.Stderr`) receives diagnostic messages and errors.
* **Exit Status**:
  * `0` (Success): If all input sources are successfully read, processed, and written to `os.Stdout`.
  * `1` (Failure): If any error occurs, including:
    * Failure to open, read, or close an input file.
    * Failure to write to `os.Stdout`.
    * A cycle is detected: an input file is identical to the output file (matching device and inode numbers) and the read position is behind the write position.

---

## Option Parsing Contract

The program options map to boolean configuration variables:

| Long Option | Short Option | Enabled Config Variables |
| :--- | :--- | :--- |
| `--number-nonblank` | `-b` | `Number = true`, `NumberNonblank = true` |
| `--number` | `-n` | `Number = true` |
| `--squeeze-blank` | `-s` | `SqueezeBlank = true` |
| `--show-nonprinting` | `-v` | `ShowNonprinting = true` |
| `--show-ends` | `-E` | `ShowEnds = true` |
| `--show-tabs` | `-T` | `ShowTabs = true` |
| `--show-all` | `-A` | `ShowNonprinting = true`, `ShowEnds = true`, `ShowTabs = true` |
| (None) | `-e` | `ShowNonprinting = true`, `ShowEnds = true` |
| (None) | `-t` | `ShowNonprinting = true`, `ShowTabs = true` |
| (None) | `-u` | *Ignored* (Go translation handles streaming/buffering natively) |

### Priority & Overrides
* `NumberNonblank` (`-b`) takes precedence over `Number` (`-n`) when determining whether to number empty lines.
* Option parsing must handle `--help` and `--version` by printing standard information to `stdout` and exiting with status `0`.
* Any unrecognized option must print usage information to `stderr` and exit with status `1`.

---

## Preconditions and Postconditions for `simple_cat` Behavior

* **Trigger Condition**: Activated when all formatting options are false:
  ```go
  !Number && !ShowEnds && !ShowNonprinting && !ShowTabs && !SqueezeBlank
  ```
* **Preconditions**:
  * `input` is an open, readable `io.Reader`.
  * `output` is an open, writable `io.Writer`.
* **Postconditions**:
  * All bytes read from `input` are written to `output` byte-for-byte unmodified.
  * Returns `nil` if the entire stream is successfully copied.
  * Returns a descriptive `error` immediately if a read or write operation fails.

---

## Preconditions and Postconditions for Formatted `cat` Behavior

* **Trigger Condition**: Activated if any formatting option is true:
  ```go
  Number || ShowEnds || ShowNonprinting || ShowTabs || SqueezeBlank
  ```
* **Persistent State**: The following variables must be preserved across multiple files:
  * `lineCounter` (int): The current line number, initialized to `1`.
  * `newlines` (int): Track consecutive empty lines. Initialized to `0` (start of a line).
  * `pendingCR` (bool): Set to `true` if a carriage return (`\r`) is pending at the end of the current buffer or file.
* **Preconditions**:
  * `input` is an open, readable `io.Reader`.
  * `output` is an open, writable `io.Writer`.
  * Global state variables (`lineCounter`, `newlines`, `pendingCR`) are passed in with their current accumulated values.
* **Postconditions**:
  * The input stream is processed to EOF and transformed according to the formatting rules.
  * Returns `nil` on success, or a descriptive `error` on read/write failure.
  * The mutated state variables are returned or persisted for the next file.

---

## Invariants for Formatting Options

For an input byte stream, the following transformations must be applied invariant-complying:

### 1. Line Numbering (`Number`, `NumberNonblank`)
* When beginning a new line (i.e. after writing a newline or starting the first file):
  * **If `NumberNonblank` is true**: If the next character is not a newline (`\n`), write `fmt.Sprintf("%6d\t", lineCounter)` and increment `lineCounter`. If the line is empty (only a newline), output the newline without a number prefix.
  * **If `Number` is true and `NumberNonblank` is false**: Write `fmt.Sprintf("%6d\t", lineCounter)` for every line (including empty ones) and increment `lineCounter`.

### 2. Squeeze Blank (`SqueezeBlank`)
* If `SqueezeBlank` is true, consecutive empty lines are capped at one.
* **State Mapping**: If consecutive newlines $\ge 2$, discard any subsequent adjacent newlines until a non-newline byte is encountered.

### 3. Show Tabs (`ShowTabs`)
* If `ShowTabs` is true, write `^I` for every tab byte (`\t`, ASCII 9). Otherwise, write `\t` literally.

### 4. Show Ends (`ShowEnds`)
* If `ShowEnds` is true, write `$` immediately before every newline character (`\n`).
* If `ShowNonprinting` is false:
  * A carriage return-newline sequence (`\r\n`) is output as `^M$`.
  * A carriage return (`\r`) at the end of a buffer or stream must be deferred (`pendingCR = true`). If the subsequent byte is not `\n`, output the carriage return as `\r`.

### 5. Show Nonprinting (`ShowNonprinting`)
If `ShowNonprinting` is true, transform each byte `b` as follows:
* **`b < 32`**:
  * If `b == 9` (`\t`) and `ShowTabs` is false: output `\t` literally.
  * If `b == 10` (`\n`): output `\n` literally.
  * Otherwise: output `^` followed by `b + 64`.
* **`32 <= b < 127`**: output `b` literally.
* **`b == 127`**: output `^?`.
* **`b >= 128`**: output `M-` followed by:
  * If `b >= 160`:
    * If `b < 255`: output the byte `b - 128` literally.
    * If `b == 255`: output `^?`.
  * If `128 <= b < 160`: output `^` followed by the byte `b - 128 + 64`.

---

## Explicit Equivalences

* **`-A` (show-all)** $\equiv$ `-vET` (sets `ShowNonprinting = true`, `ShowEnds = true`, `ShowTabs = true`)
* **`-e`** $\equiv$ `-vE` (sets `ShowNonprinting = true`, `ShowEnds = true`)
* **`-t`** $\equiv$ `-vT` (sets `ShowNonprinting = true`, `ShowTabs = true`)
* **`-u` (ignored)**: Does not modify behavior. Unbuffered/streaming I/O is standard in the Go implementation.