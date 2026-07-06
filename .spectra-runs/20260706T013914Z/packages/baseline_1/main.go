package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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
	lineNumber            int64
	atLineStart           bool
	consecutiveBlankLines int
	pendingCR             bool
}

func main() {
	opts, files, special, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: %v\n", err)
		os.Exit(1)
	}

	if special == "help" {
		printHelp()
		return
	}
	if special == "version" {
		fmt.Println("cat (go translation)")
		return
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	if !needsTransform(opts) {
		if !runSimple(files) {
			os.Exit(1)
		}
		return
	}

	w := bufio.NewWriterSize(os.Stdout, 32*1024)
	st := &state{atLineStart: true}
	ok := true
	for _, name := range files {
		if err := processFile(name, opts, st, w); err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			ok = false
		}
	}
	if st.pendingCR {
		if err := w.WriteByte('\r'); err != nil {
			fmt.Fprintf(os.Stderr, "cat: standard output: %v\n", err)
			os.Exit(1)
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "cat: standard output: %v\n", err)
		os.Exit(1)
	}
	if !ok {
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, string, error) {
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
		if len(arg) > 1 && arg[1] == '-' {
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
				return opts, nil, "help", nil
			case "--version":
				return opts, nil, "version", nil
			default:
				return opts, nil, "", fmt.Errorf("unrecognized option '%s'", arg)
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
				// Ignored for GNU compatibility.
			case 'v':
				opts.showNonprinting = true
			default:
				return opts, nil, "", fmt.Errorf("invalid option -- '%c'", arg[j])
			}
		}
	}
	return opts, files, "", nil
}

func needsTransform(opts options) bool {
	return opts.showNonprinting || opts.showTabs || opts.showEnds || opts.number || opts.squeezeBlank
}

func runSimple(files []string) bool {
	buf := make([]byte, 32*1024)
	ok := true
	for _, name := range files {
		var r io.Reader
		var f *os.File
		if name == "-" {
			r = os.Stdin
		} else {
			var err error
			f, err = os.Open(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
				ok = false
				continue
			}
			r = f
		}
		if _, err := io.CopyBuffer(os.Stdout, r, buf); err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			ok = false
		}
		if f != nil {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
				ok = false
			}
		}
	}
	return ok
}

func processFile(name string, opts options, st *state, w *bufio.Writer) error {
	if name == "-" {
		return processReader(os.Stdin, opts, st, w)
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := processReader(f, opts, st, w); err != nil {
		return err
	}
	return f.Close()
}

func processReader(r io.Reader, opts options, st *state, w *bufio.Writer) error {
	br := bufio.NewReaderSize(r, 32*1024)
	for {
		b, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := processByte(b, opts, st, w); err != nil {
			return err
		}
	}
}

func processByte(b byte, opts options, st *state, w *bufio.Writer) error {
	if st.pendingCR {
		if b == '\n' {
			if _, err := w.WriteString("^M"); err != nil {
				return err
			}
			st.pendingCR = false
			return writeNewline(opts, st, w)
		}
		if err := w.WriteByte('\r'); err != nil {
			return err
		}
		st.pendingCR = false
	}

	if b == '\n' {
		return writeNewline(opts, st, w)
	}

	if st.atLineStart {
		if opts.number {
			if err := writeLineNumber(st, w); err != nil {
				return err
			}
		}
		st.atLineStart = false
		st.consecutiveBlankLines = 0
	}

	if opts.showNonprinting {
		return writeQuotedByte(b, opts.showTabs, w)
	}

	if b == '\t' && opts.showTabs {
		_, err := w.WriteString("^I")
		return err
	}
	if b == '\r' && opts.showEnds {
		st.pendingCR = true
		return nil
	}
	return w.WriteByte(b)
}

func writeNewline(opts options, st *state, w *bufio.Writer) error {
	blank := st.atLineStart
	if blank {
		if opts.squeezeBlank && st.consecutiveBlankLines >= 1 {
			st.consecutiveBlankLines++
			return nil
		}
		if opts.number && !opts.numberNonblank {
			if err := writeLineNumber(st, w); err != nil {
				return err
			}
		}
		st.consecutiveBlankLines++
	} else {
		st.consecutiveBlankLines = 0
	}
	if opts.showEnds {
		if err := w.WriteByte('$'); err != nil {
			return err
		}
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	st.atLineStart = true
	return nil
}

func writeLineNumber(st *state, w *bufio.Writer) error {
	st.lineNumber++
	s := strconv.FormatInt(st.lineNumber, 10)
	for i := len(s); i < 6; i++ {
		if err := w.WriteByte(' '); err != nil {
			return err
		}
	}
	if _, err := w.WriteString(s); err != nil {
		return err
	}
	return w.WriteByte('\t')
}

func writeQuotedByte(b byte, showTabs bool, w *bufio.Writer) error {
	switch {
	case b >= 32 && b < 127:
		return w.WriteByte(b)
	case b == 127:
		_, err := w.WriteString("^?")
		return err
	case b >= 128:
		if _, err := w.WriteString("M-"); err != nil {
			return err
		}
		c := b - 128
		if c >= 32 && c < 127 {
			return w.WriteByte(c)
		}
		if c == 127 {
			_, err := w.WriteString("^?")
			return err
		}
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte(c + 64)
	case b == '\t' && !showTabs:
		return w.WriteByte('\t')
	default:
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte(b + 64)
	}
}

func printHelp() {
	fmt.Print("Usage: cat [OPTION]... [FILE]...\n")
	fmt.Print("Concatenate FILE(s) to standard output.\n\n")
	fmt.Print("  -A, --show-all           equivalent to -vET\n")
	fmt.Print("  -b, --number-nonblank    number nonempty output lines, overrides -n\n")
	fmt.Print("  -e                       equivalent to -vE\n")
	fmt.Print("  -E, --show-ends          display $ at end of each line\n")
	fmt.Print("  -n, --number             number all output lines\n")
	fmt.Print("  -s, --squeeze-blank      suppress repeated empty output lines\n")
	fmt.Print("  -t                       equivalent to -vT\n")
	fmt.Print("  -T, --show-tabs          display TAB characters as ^I\n")
	fmt.Print("  -u                       ignored\n")
	fmt.Print("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB\n")
}
