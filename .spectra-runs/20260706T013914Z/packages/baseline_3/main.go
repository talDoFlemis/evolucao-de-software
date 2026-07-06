package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type options struct {
	showNonprinting bool
	showTabs        bool
	showEnds        bool
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
}

type state struct {
	lineNo                 int
	lineStart              bool
	currentLineHasContent  bool
	consecutiveBlankLines  int
	pendingCR              bool
}

func main() {
	opts, files, code, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(code)
	}
	if code != 0 {
		os.Exit(code)
	}

	writer := bufio.NewWriterSize(os.Stdout, 32768)
	defer writer.Flush()

	st := &state{lineNo: 1, lineStart: true}
	ok := true

	if len(files) == 0 {
		files = []string{"-"}
	}

	for _, name := range files {
		if err := processFile(name, opts, st, writer); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			ok = false
		}
	}

	if st.pendingCR {
		if _, err := writer.Write([]byte{'\r'}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		st.pendingCR = false
	}

	if err := writer.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if !ok {
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, int, error) {
	var opts options
	var files []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) == 0 || arg == "-" || arg[0] != '-' {
			files = append(files, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--number-nonblank":
				opts.number = true
				opts.numberNonblank = true
			case "--number":
				opts.number = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-nonprinting":
				opts.showNonprinting = true
			case "--show-ends":
				opts.showEnds = true
			case "--show-tabs":
				opts.showTabs = true
			case "--show-all":
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case "--help":
				printUsage(os.Stdout)
				return opts, nil, 0, nil
			case "--version":
				fmt.Fprintln(os.Stdout, "cat (go implementation)")
				return opts, nil, 0, nil
			default:
				return opts, nil, 1, fmt.Errorf("unrecognized option %q", arg)
			}
			continue
		}

		for _, c := range arg[1:] {
			switch c {
			case 'A':
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case 'b':
				opts.number = true
				opts.numberNonblank = true
			case 'e':
				opts.showNonprinting = true
				opts.showEnds = true
			case 'E':
				opts.showEnds = true
			case 'n':
				opts.number = true
			case 's':
				opts.squeezeBlank = true
			case 't':
				opts.showNonprinting = true
				opts.showTabs = true
			case 'T':
				opts.showTabs = true
			case 'u':
				// Ignored for GNU cat compatibility.
			case 'v':
				opts.showNonprinting = true
			default:
				return opts, nil, 1, fmt.Errorf("invalid option -- %q", string(c))
			}
		}
	}

	return opts, files, 0, nil
}

func printUsage(w io.Writer) {
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(w, "Usage: %s [OPTION]... [FILE]...\n", prog)
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(w, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(w, "  -e                       equivalent to -vE")
	fmt.Fprintln(w, "  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Fprintln(w, "  -n, --number             number all output lines")
	fmt.Fprintln(w, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(w, "  -t                       equivalent to -vT")
	fmt.Fprintln(w, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(w, "  -u                       ignored")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
}

func processFile(name string, opts options, st *state, w *bufio.Writer) error {
	var r io.Reader
	var f *os.File
	if name == "-" {
		r = os.Stdin
	} else {
		var err error
		f, err = os.Open(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		defer f.Close()
		r = f
	}

	reader := bufio.NewReaderSize(r, 32768)
	buf := make([]byte, 32768)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if err := processChunk(buf[:n], opts, st, w); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if name == "-" {
				return err
			}
			return fmt.Errorf("%s: %w", name, err)
		}
	}
}

func processChunk(data []byte, opts options, st *state, w *bufio.Writer) error {
	for _, b := range data {
		if st.pendingCR && (!opts.showNonprinting || b != '\n') {
			if err := writeByte(w, '\r'); err != nil {
				return err
			}
			st.pendingCR = false
		}

		if b == '\n' {
			blank := !st.currentLineHasContent
			if blank {
				st.consecutiveBlankLines++
				if opts.squeezeBlank && st.consecutiveBlankLines > 1 {
					st.lineStart = true
					st.currentLineHasContent = false
					st.pendingCR = false
					continue
				}
			} else {
				st.consecutiveBlankLines = 0
			}

			if st.lineStart && opts.number && !opts.numberNonblank {
				if err := writeLineNumber(w, st); err != nil {
					return err
				}
			}
			if st.pendingCR {
				if _, err := w.Write([]byte{'^', 'M'}); err != nil {
					return err
				}
				st.pendingCR = false
			}
			if opts.showEnds {
				if err := writeByte(w, '$'); err != nil {
					return err
				}
			}
			if err := writeByte(w, '\n'); err != nil {
				return err
			}
			st.lineStart = true
			st.currentLineHasContent = false
			continue
		}

		if st.lineStart && opts.number {
			if !opts.numberNonblank || b != '\n' {
				if err := writeLineNumber(w, st); err != nil {
					return err
				}
			}
		}
		st.lineStart = false
		st.currentLineHasContent = true
		st.consecutiveBlankLines = 0

		if !opts.showNonprinting && opts.showEnds && b == '\r' {
			st.pendingCR = true
			continue
		}

		if err := writeTransformedByte(w, b, opts); err != nil {
			return err
		}
	}
	return nil
}

func writeLineNumber(w *bufio.Writer, st *state) error {
	if _, err := fmt.Fprintf(w, "%6d\t", st.lineNo); err != nil {
		return err
	}
	st.lineNo++
	st.lineStart = false
	return nil
}

func writeTransformedByte(w *bufio.Writer, b byte, opts options) error {
	if opts.showNonprinting {
		switch {
		case b == '\t' && !opts.showTabs:
			return writeByte(w, '\t')
		case b >= 32 && b < 127:
			return writeByte(w, b)
		case b == 127:
			_, err := w.Write([]byte{'^', '?'})
			return err
		case b < 32:
			_, err := w.Write([]byte{'^', b + 64})
			return err
		case b < 128+32:
			_, err := w.Write([]byte{'M', '-', '^', b - 128 + 64})
			return err
		case b < 255:
			_, err := w.Write([]byte{'M', '-', b - 128})
			return err
		default:
			_, err := w.Write([]byte{'M', '-', '^', '?'})
			return err
		}
	}

	if b == '\t' && opts.showTabs {
		_, err := w.Write([]byte{'^', 'I'})
		return err
	}
	return writeByte(w, b)
}

func writeByte(w *bufio.Writer, b byte) error {
	return w.WriteByte(b)
}
