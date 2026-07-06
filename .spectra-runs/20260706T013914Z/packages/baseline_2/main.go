package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	number           bool
	numberNonblank   bool
	squeezeBlank     bool
	showEnds         bool
	showNonprinting  bool
	showTabs         bool
}

type state struct {
	lineNo           int
	atLineStart      bool
	consecutiveEmpty int
	pendingCR        bool
}

type writer struct {
	w   *bufio.Writer
	out *os.File
}

func newWriter(out *os.File) *writer {
	return &writer{w: bufio.NewWriterSize(out, 32768), out: out}
}

func (w *writer) writeByte(b byte) error {
	return w.w.WriteByte(b)
}

func (w *writer) writeString(s string) error {
	_, err := w.w.WriteString(s)
	return err
}

func (w *writer) flush() error {
	return w.w.Flush()
}

func usage() {
	fmt.Fprintf(os.Stdout, "Usage: %s [OPTION]... [FILE]...\n", programName())
	fmt.Fprintln(os.Stdout, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(os.Stdout, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(os.Stdout, "  -e                       equivalent to -vE")
	fmt.Fprintln(os.Stdout, "  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Fprintln(os.Stdout, "  -n, --number             number all output lines")
	fmt.Fprintln(os.Stdout, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(os.Stdout, "  -t                       equivalent to -vT")
	fmt.Fprintln(os.Stdout, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(os.Stdout, "  -u                       ignored")
	fmt.Fprintln(os.Stdout, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
}

func version() {
	fmt.Fprintf(os.Stdout, "%s (go implementation)\n", programName())
}

func programName() string {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return "cat"
	}
	return os.Args[0]
}

func parseArgs(args []string) (options, []string, int) {
	var opts options
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
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
				usage()
				return opts, nil, 0
			case "--version":
				version()
				return opts, nil, 0
			default:
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", programName(), arg)
				return opts, nil, 1
			}
			continue
		}
		for j := 1; j < len(arg); j++ {
			switch arg[j] {
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
				// Ignored.
			case 'v':
				opts.showNonprinting = true
			case '-':
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", programName(), arg)
				return opts, nil, 1
			default:
				fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", programName(), arg[j])
				return opts, nil, 1
			}
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	return opts, files, 2
}

func writeLineNumber(w *writer, st *state) error {
	_, err := fmt.Fprintf(w.w, "%6d\t", st.lineNo)
	if err != nil {
		return err
	}
	st.lineNo++
	return nil
}

func writeQuotedByte(w *writer, b byte, showTabs bool) error {
	if b >= 32 {
		if b < 127 {
			return w.writeByte(b)
		}
		if b == 127 {
			return w.writeString("^?")
		}
		if err := w.writeString("M-"); err != nil {
			return err
		}
		if b >= 160 {
			if b < 255 {
				return w.writeByte(b - 128)
			}
			return w.writeString("^?")
		}
		if err := w.writeByte('^'); err != nil {
			return err
		}
		return w.writeByte(b - 64)
	}
	if b == '\t' && !showTabs {
		return w.writeByte('\t')
	}
	if err := w.writeByte('^'); err != nil {
		return err
	}
	return w.writeByte(b + 64)
}

func processByte(w *writer, st *state, opts options, b byte) error {
	if st.pendingCR && b != '\n' {
		if err := w.writeByte('\r'); err != nil {
			return err
		}
		st.pendingCR = false
	}

	if b == '\n' {
		if st.atLineStart {
			if opts.squeezeBlank && st.consecutiveEmpty >= 1 {
				st.consecutiveEmpty++
				return nil
			}
			if opts.number && !opts.numberNonblank {
				if err := writeLineNumber(w, st); err != nil {
					return err
				}
			}
			st.consecutiveEmpty++
		} else {
			st.consecutiveEmpty = 0
		}
		if opts.showEnds {
			if st.pendingCR {
				if err := w.writeString("^M"); err != nil {
					return err
				}
				st.pendingCR = false
			}
			if err := w.writeByte('$'); err != nil {
				return err
			}
		}
		if err := w.writeByte('\n'); err != nil {
			return err
		}
		st.atLineStart = true
		return nil
	}

	if st.atLineStart {
		if opts.number {
			if err := writeLineNumber(w, st); err != nil {
				return err
			}
		}
		st.atLineStart = false
		st.consecutiveEmpty = 0
	}

	if opts.showNonprinting {
		return writeQuotedByte(w, b, opts.showTabs)
	}
	if b == '\t' && opts.showTabs {
		return w.writeString("^I")
	}
	if b == '\r' && opts.showEnds {
		st.pendingCR = true
		return nil
	}
	return w.writeByte(b)
}

func catFormatted(r io.Reader, w *writer, st *state, opts options) error {
	buf := make([]byte, 32768)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if err := processByte(w, st, opts, b); err != nil {
					return err
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func catSimple(r io.Reader, w *writer) error {
	buf := make([]byte, 32768)
	_, err := io.CopyBuffer(w.w, r, buf)
	return err
}

func main() {
	opts, files, mode := parseArgs(os.Args[1:])
	if mode != 2 {
		os.Exit(mode)
	}

	formatted := opts.number || opts.showEnds || opts.showNonprinting || opts.showTabs || opts.squeezeBlank
	st := state{lineNo: 1, atLineStart: true}
	w := newWriter(os.Stdout)
	ok := true

	for _, name := range files {
		var f *os.File
		if name == "-" {
			f = os.Stdin
		} else {
			var err error
			f, err = os.Open(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", programName(), name, err)
				ok = false
				continue
			}
		}

		var err error
		if formatted {
			err = catFormatted(f, w, &st, opts)
		} else {
			err = catSimple(f, w)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", programName(), name, err)
			ok = false
		}

		if f != os.Stdin {
			if cerr := f.Close(); cerr != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", programName(), name, cerr)
				ok = false
			}
		}
	}

	if st.pendingCR {
		if err := w.writeByte('\r'); err != nil {
			fmt.Fprintf(os.Stderr, "%s: standard output: %v\n", programName(), err)
			ok = false
		}
	}
	if err := w.flush(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: standard output: %v\n", programName(), err)
		ok = false
	}

	if !ok {
		os.Exit(1)
	}
}
