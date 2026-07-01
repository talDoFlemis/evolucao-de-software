package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	number         bool
	numberNonBlank bool
	squeezeBlank   bool
	showEnds       bool
	showNonPrint   bool
	showTabs       bool
	showHelp       bool
	showVersion    bool
}

type state struct {
	lineNumber    int
	atLineStart   bool
	consecNewline int
	pendingCR     bool
}

func main() {
	opts, files, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if opts.showHelp {
		printUsage()
		return
	}
	if opts.showVersion {
		printVersion()
		return
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	if err := run(files, opts, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	parsingOpts := true

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if parsingOpts && arg == "--" {
			parsingOpts = false
			continue
		}
		if parsingOpts && strings.HasPrefix(arg, "--") && len(arg) > 2 {
			switch arg {
			case "--show-all":
				opts.showNonPrint = true
				opts.showEnds = true
				opts.showTabs = true
			case "--number-nonblank":
				opts.number = true
				opts.numberNonBlank = true
			case "--number":
				opts.number = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-nonprinting":
				opts.showNonPrint = true
			case "--show-ends":
				opts.showEnds = true
			case "--show-tabs":
				opts.showTabs = true
			case "--help":
				opts.showHelp = true
			case "--version":
				opts.showVersion = true
			default:
				return options{}, nil, fmt.Errorf("unrecognized option: %s", arg)
			}
			continue
		}
		if parsingOpts && strings.HasPrefix(arg, "-") && arg != "-" {
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'A':
					opts.showNonPrint = true
					opts.showEnds = true
					opts.showTabs = true
				case 'b':
					opts.number = true
					opts.numberNonBlank = true
				case 'e':
					opts.showEnds = true
					opts.showNonPrint = true
				case 'E':
					opts.showEnds = true
				case 'n':
					opts.number = true
				case 's':
					opts.squeezeBlank = true
				case 't':
					opts.showTabs = true
					opts.showNonPrint = true
				case 'u':
				case 'v':
					opts.showNonPrint = true
				case 'T':
					opts.showTabs = true
				default:
					return options{}, nil, fmt.Errorf("unrecognized option: -%c", arg[j])
				}
			}
			continue
		}
		files = append(files, arg)
	}

	if opts.numberNonBlank {
		opts.number = true
	}
	return opts, files, nil
}

func run(files []string, opts options, out io.Writer) error {
	if !opts.number && !opts.showEnds && !opts.squeezeBlank && !opts.showNonPrint && !opts.showTabs {
		return fastCopy(files, out)
	}

	st := state{lineNumber: 1, atLineStart: true}
	for _, name := range files {
		if err := processFile(name, opts, out, &st); err != nil {
			return err
		}
	}
	if st.pendingCR {
		if _, err := io.WriteString(out, "\r"); err != nil {
			return fmt.Errorf("write error: %w", err)
		}
	}
	return nil
}

func fastCopy(files []string, out io.Writer) error {
	buf := make([]byte, 32*1024)
	for _, name := range files {
		var (
			f   *os.File
			err error
		)
		if name == "-" {
			f = os.Stdin
		} else {
			f, err = os.Open(name)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if err := refuseSelfCopy(name, f); err != nil {
			if name != "-" {
				_ = f.Close()
			}
			return err
		}

		if err := copyStream(f, out, buf); err != nil {
			if name != "-" {
				_ = f.Close()
			}
			return err
		}
		if name != "-" {
			if err := f.Close(); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

func copyStream(r io.Reader, out io.Writer, buf []byte) error {
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write error: %w", werr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}
	}
}

func processFile(name string, opts options, out io.Writer, st *state) error {
	var (
		f   *os.File
		err error
	)
	if name == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	defer func() {
		if name != "-" {
			_ = f.Close()
		}
	}()

	if err := refuseSelfCopy(name, f); err != nil {
		return err
	}

	if err := formattedCopy(f, opts, out, st); err != nil {
		return err
	}

	if name == "-" {
		if err := f.Close(); err != nil {
			return fmt.Errorf("standard input: %w", err)
		}
	}
	return nil
}

func refuseSelfCopy(name string, in *os.File) error {
	if name == "-" {
		return nil
	}
	inInfo, err := in.Stat()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	outInfo, err := os.Stdout.Stat()
	if err != nil {
		return fmt.Errorf("stdout: %w", err)
	}
	if !inInfo.Mode().IsRegular() || !outInfo.Mode().IsRegular() {
		return nil
	}
	if os.SameFile(inInfo, outInfo) {
		return fmt.Errorf("%s: input file is output file", name)
	}
	return nil
}

func formattedCopy(f *os.File, opts options, out io.Writer, st *state) error {
	br := bufio.NewReader(f)
	for {
		b, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("%s: %w", fileName(f), err)
		}

		if st.pendingCR {
			if b == '\n' {
				if err := writeString(out, "^M"); err != nil {
					return err
				}
				st.pendingCR = false
			} else {
				if err := writeByte(out, '\r'); err != nil {
					return err
				}
				st.pendingCR = false
				st.atLineStart = false
				st.consecNewline = 0
				// Reprocess b normally after flushing the deferred CR.
			}
		}

		if b == '\n' {
			if st.consecNewline > 0 && opts.squeezeBlank {
				st.consecNewline++
				st.atLineStart = true
				continue
			}
			if opts.number && !opts.numberNonBlank && st.atLineStart {
				if err := emitLineNumber(out, st); err != nil {
					return err
				}
			}
			if opts.showEnds {
				if err := writeByte(out, '$'); err != nil {
					return err
				}
			}
			if err := writeByte(out, '\n'); err != nil {
				return err
			}
			st.atLineStart = true
			st.consecNewline++
			continue
		}

		if st.atLineStart && opts.number {
			if err := emitLineNumber(out, st); err != nil {
				return err
			}
		}
		st.atLineStart = false
		st.consecNewline = 0

		if opts.showNonPrint {
			if err := writeQuoted(out, b, opts.showTabs); err != nil {
				return err
			}
			continue
		}

		if b == '\t' && opts.showTabs {
			if err := writeString(out, "^I"); err != nil {
				return err
			}
			continue
		}

		if b == '\r' && opts.showEnds {
			if next, ok := peekByte(br); ok {
				if next == '\n' {
					if err := writeString(out, "^M"); err != nil {
						return err
					}
					continue
				}
				if err := writeByte(out, '\r'); err != nil {
					return err
				}
				continue
			}
			st.pendingCR = true
			continue
		}

		if err := writeByte(out, b); err != nil {
			return err
		}
	}
}

func emitLineNumber(out io.Writer, st *state) error {
	if _, err := io.WriteString(out, fmt.Sprintf("%6d\t", st.lineNumber)); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	st.lineNumber++
	return nil
}

func writeQuoted(out io.Writer, b byte, showTabs bool) error {
	switch {
	case b == '\t' && !showTabs:
		return writeByte(out, '\t')
	case b == '\n':
		return writeByte(out, '\n')
	case b >= 32 && b < 127:
		return writeByte(out, b)
	case b == 127:
		return writeString(out, "^?")
	case b >= 128:
		if err := writeString(out, "M-"); err != nil {
			return err
		}
		c := b - 128
		if c >= 32 && c < 127 {
			return writeByte(out, c)
		}
		if c == 127 {
			return writeString(out, "^?")
		}
		return writeString(out, fmt.Sprintf("^%c", c+64))
	default:
		return writeString(out, fmt.Sprintf("^%c", b+64))
	}
}

func peekByte(r *bufio.Reader) (byte, bool) {
	b, err := r.Peek(1)
	if err != nil || len(b) == 0 {
		return 0, false
	}
	return b[0], true
}

func writeString(out io.Writer, s string) error {
	if _, err := io.WriteString(out, s); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

func writeByte(out io.Writer, b byte) error {
	var buf [1]byte
	buf[0] = b
	if _, err := out.Write(buf[:]); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

func fileName(f *os.File) string {
	if f == os.Stdin {
		return "standard input"
	}
	if n := f.Name(); n != "" {
		return n
	}
	return "input"
}

func printUsage() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println("  -A, --show-all           equivalent to -vET")
	fmt.Println("  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Println("  -e                       equivalent to -vE")
	fmt.Println("  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Println("  -n, --number             number all output lines")
	fmt.Println("  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Println("  -t                       equivalent to -vT")
	fmt.Println("  -T, --show-tabs          display TAB characters as ^I")
	fmt.Println("  -u                       (ignored)")
	fmt.Println("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
}

func printVersion() {
	fmt.Printf("%s (Go translation)\n", os.Args[0])
}
