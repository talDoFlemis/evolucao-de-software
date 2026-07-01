package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

type options struct {
	showAll         bool
	numberNonblank  bool
	showEnds        bool
	numberAll       bool
	squeezeBlank    bool
	showTabs        bool
	showNonprinting bool
	ignoreU         bool
	help            bool
	version         bool
}

type state struct {
	lineNum  int
	blankRun int
}

func main() {
	opts, files, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if opts.help {
		printUsage(os.Stdout)
		return
	}
	if opts.version {
		fmt.Fprintln(os.Stdout, "cat (Go translation)")
		return
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	stdoutInfo, _ := os.Stdout.Stat()
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	st := state{lineNum: 1}
	var currentLine []byte
	ok := true

	for _, name := range files {
		if name == "-" {
			if err := processReader(os.Stdin, name, opts, stdoutInfo, &st, &currentLine, out); err != nil {
				ok = false
			}
			continue
		}

		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			ok = false
			continue
		}

		func() {
			defer f.Close()
			if err := processReader(f, name, opts, stdoutInfo, &st, &currentLine, out); err != nil {
				ok = false
			}
		}()
	}

	if len(currentLine) > 0 {
		if err := emitLine(out, currentLine, false, opts, &st); err != nil {
			fmt.Fprintln(os.Stderr, err)
			ok = false
		}
		currentLine = currentLine[:0]
	}

	if err := out.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		ok = false
	}

	if !ok {
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	endOfOptions := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if endOfOptions || arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			endOfOptions = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			switch name {
			case "show-all":
				opts.showAll = true
			case "number-nonblank":
				opts.numberNonblank = true
				opts.numberAll = true
			case "show-ends":
				opts.showEnds = true
			case "number":
				opts.numberAll = true
			case "squeeze-blank":
				opts.squeezeBlank = true
			case "show-tabs":
				opts.showTabs = true
			case "show-nonprinting":
				opts.showNonprinting = true
			case "help":
				opts.help = true
			case "version":
				opts.version = true
			default:
				return opts, nil, fmt.Errorf("unknown option --%s", name)
			}
			continue
		}

		for j := 1; j < len(arg); j++ {
			switch arg[j] {
			case 'A':
				opts.showAll = true
			case 'b':
				opts.numberNonblank = true
				opts.numberAll = true
			case 'e':
				opts.showEnds = true
				opts.showNonprinting = true
			case 'E':
				opts.showEnds = true
			case 'n':
				opts.numberAll = true
			case 's':
				opts.squeezeBlank = true
			case 't':
				opts.showTabs = true
				opts.showNonprinting = true
			case 'T':
				opts.showTabs = true
			case 'u':
				opts.ignoreU = true
			case 'v':
				opts.showNonprinting = true
			case 'h':
				opts.help = true
			case 'V':
				opts.version = true
			default:
				return opts, nil, fmt.Errorf("unknown option -%c", arg[j])
			}
		}
	}

	if opts.showAll {
		opts.showNonprinting = true
		opts.showEnds = true
		opts.showTabs = true
	}
	if opts.help || opts.version {
		return opts, nil, nil
	}
	return opts, files, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: cat [OPTION]... [FILE]...")
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(w, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(w, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(w, "  -e                       equivalent to -vE")
	fmt.Fprintln(w, "  -E, --show-ends          display $ before each newline")
	fmt.Fprintln(w, "  -n, --number             number all output lines")
	fmt.Fprintln(w, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(w, "  -t                       equivalent to -vT")
	fmt.Fprintln(w, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(w, "  -u                       ignored")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for TAB and LF")
	fmt.Fprintln(w, "  -h, --help               display this help and exit")
	fmt.Fprintln(w, "  -V, --version            output version information and exit")
}

func processReader(r io.Reader, name string, opts options, stdoutInfo os.FileInfo, st *state, currentLine *[]byte, out *bufio.Writer) error {
	if stdoutInfo != nil {
		if inInfo, err := fileInfo(r); err == nil && sameFile(stdoutInfo, inInfo) {
			fmt.Fprintf(os.Stderr, "%s: input file is output file\n", name)
			return errors.New("self copy")
		}
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					if err := emitLine(out, *currentLine, true, opts, st); err != nil {
						return err
					}
					*currentLine = (*currentLine)[:0]
				} else {
					*currentLine = append(*currentLine, b)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			fmt.Fprintln(os.Stderr, err)
			return err
		}
	}
}

func emitLine(out *bufio.Writer, line []byte, terminated bool, opts options, st *state) error {
	blank := len(line) == 0
	if blank {
		if opts.squeezeBlank && st.blankRun > 0 {
			st.blankRun++
			return nil
		}
		st.blankRun++
	} else {
		st.blankRun = 0
	}

	if opts.numberNonblank {
		if !blank {
			if _, err := fmt.Fprintf(out, "%6d\t", st.lineNum); err != nil {
				return err
			}
			st.lineNum++
		}
	} else if opts.numberAll {
		if _, err := fmt.Fprintf(out, "%6d\t", st.lineNum); err != nil {
			return err
		}
		st.lineNum++
	}

	if len(line) > 0 {
		for i, b := range line {
			last := terminated && i == len(line)-1 && b == '\r' && opts.showEnds && !opts.showNonprinting
			if last {
				if _, err := out.WriteString("^M"); err != nil {
					return err
				}
				continue
			}
			if err := writeByte(out, b, opts); err != nil {
				return err
			}
		}
	}

	if terminated && opts.showEnds {
		if err := out.WriteByte('$'); err != nil {
			return err
		}
	}
	if terminated {
		if err := out.WriteByte('\n'); err != nil {
			return err
		}
	}
	return nil
}

func writeByte(out *bufio.Writer, b byte, opts options) error {
	if opts.showNonprinting {
		switch {
		case b >= 128:
			if err := out.WriteByte('M'); err != nil {
				return err
			}
			if err := out.WriteByte('-'); err != nil {
				return err
			}
			if b >= 160 {
				if b < 255 {
					return out.WriteByte(b - 128)
				}
				if err := out.WriteByte('^'); err != nil {
					return err
				}
				return out.WriteByte('?')
			}
			if err := out.WriteByte('^'); err != nil {
				return err
			}
			return out.WriteByte(b - 128 + 64)
		case b == '\t':
			if opts.showTabs {
				if _, err := out.WriteString("^I"); err != nil {
					return err
				}
				return nil
			}
			return out.WriteByte(b)
		case b == '\n':
			return nil
		case b == 127:
			if _, err := out.WriteString("^?"); err != nil {
				return err
			}
			return nil
		case b < 32:
			if err := out.WriteByte('^'); err != nil {
				return err
			}
			return out.WriteByte(b + 64)
		default:
			return out.WriteByte(b)
		}
	}

	if b == '\t' && opts.showTabs {
		if _, err := out.WriteString("^I"); err != nil {
			return err
		}
		return nil
	}
	return out.WriteByte(b)
}

func fileInfo(r io.Reader) (os.FileInfo, error) {
	if f, ok := r.(*os.File); ok {
		return f.Stat()
	}
	return nil, errors.New("no file info")
}

func sameFile(a, b os.FileInfo) bool {
	sa, okA := a.Sys().(*syscall.Stat_t)
	sb, okB := b.Sys().(*syscall.Stat_t)
	return okA && okB && sa.Dev == sb.Dev && sa.Ino == sb.Ino
}
