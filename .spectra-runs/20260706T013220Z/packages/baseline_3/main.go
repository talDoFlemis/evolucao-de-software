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
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
}

type catState struct {
	opts            options
	out             *bufio.Writer
	line            uint64
	atLineStart     bool
	previousNewline bool
	pendingCR       bool
	err             error
}

func programName() string {
	return filepath.Base(os.Args[0])
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION]... [FILE]...\n", programName())
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(w, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(w, "  -e                       equivalent to -vE")
	fmt.Fprintln(w, "  -E, --show-ends          display $ at end of each line")
	fmt.Fprintln(w, "  -n, --number             number all output lines")
	fmt.Fprintln(w, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(w, "  -t                       equivalent to -vT")
	fmt.Fprintln(w, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(w, "  -u                       ignored")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Fprintln(w, "      --help               display this help and exit")
	fmt.Fprintln(w, "      --version            output version information and exit")
}

func setOption(o *options, c byte) bool {
	switch c {
	case 'A':
		o.showNonprinting = true
		o.showEnds = true
		o.showTabs = true
	case 'b':
		o.number = true
		o.numberNonblank = true
	case 'e':
		o.showNonprinting = true
		o.showEnds = true
	case 'E':
		o.showEnds = true
	case 'n':
		o.number = true
	case 's':
		o.squeezeBlank = true
	case 't':
		o.showNonprinting = true
		o.showTabs = true
	case 'T':
		o.showTabs = true
	case 'u':
	case 'v':
		o.showNonprinting = true
	default:
		return false
	}
	return true
}

func parseArgs(args []string) (options, []string, int) {
	var o options
	var files []string
	optionsDone := false

	long := map[string]byte{
		"show-all":         'A',
		"number-nonblank":  'b',
		"show-ends":        'E',
		"number":           'n',
		"squeeze-blank":    's',
		"show-tabs":        'T',
		"show-nonprinting": 'v',
	}

	for _, arg := range args {
		if optionsDone || arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			optionsDone = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			switch name {
			case "help":
				usage(os.Stdout)
				return o, nil, 2
			case "version":
				fmt.Printf("%s (Go implementation) 1.0\n", programName())
				return o, nil, 2
			}
			c, ok := long[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '--%s'\n", programName(), name)
				fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", programName())
				return o, nil, 1
			}
			setOption(&o, c)
			continue
		}
		for i := 1; i < len(arg); i++ {
			if !setOption(&o, arg[i]) {
				fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", programName(), arg[i])
				fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", programName())
				return o, nil, 1
			}
		}
	}

	if len(files) == 0 {
		files = []string{"-"}
	}
	return o, files, 0
}

func (s *catState) writeByte(b byte) {
	if s.err == nil {
		s.err = s.out.WriteByte(b)
	}
}

func (s *catState) writeString(v string) {
	if s.err == nil {
		_, s.err = s.out.WriteString(v)
	}
}

func (s *catState) writeNumber() {
	s.line++
	s.writeString(fmt.Sprintf("%6d\t", s.line))
}

func (s *catState) writeNonprinting(b byte) {
	switch {
	case b >= 32 && b < 127:
		s.writeByte(b)
	case b == 127:
		s.writeString("^?")
	case b >= 128:
		s.writeString("M-")
		low := b - 128
		if low < 32 {
			s.writeByte('^')
			s.writeByte(low + 64)
		} else if low == 127 {
			s.writeString("^?")
		} else {
			s.writeByte(low)
		}
	default:
		s.writeByte('^')
		s.writeByte(b + 64)
	}
}

func (s *catState) processByte(b byte) {
	if s.err != nil {
		return
	}

	if s.pendingCR {
		if b == '\n' {
			s.writeString("^M")
		} else {
			s.writeByte('\r')
		}
		s.pendingCR = false
	}

	if b == '\n' {
		if s.opts.squeezeBlank && s.previousNewline {
			return
		}
		if s.atLineStart && s.opts.number && !s.opts.numberNonblank {
			s.writeNumber()
		}
		if s.opts.showEnds {
			s.writeByte('$')
		}
		s.writeByte('\n')
		s.atLineStart = true
		s.previousNewline = true
		return
	}

	s.previousNewline = false
	if s.atLineStart {
		if s.opts.number {
			s.writeNumber()
		}
		s.atLineStart = false
	}

	if b == '\r' && s.opts.showEnds && !s.opts.showNonprinting {
		s.pendingCR = true
		return
	}
	if b == '\t' {
		if s.opts.showTabs {
			s.writeString("^I")
		} else {
			s.writeByte(b)
		}
		return
	}
	if s.opts.showNonprinting {
		s.writeNonprinting(b)
	} else {
		s.writeByte(b)
	}
}

func (s *catState) copy(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			s.processByte(b)
		}
		if s.err != nil {
			return s.err
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func main() {
	opts, files, parseStatus := parseArgs(os.Args[1:])
	if parseStatus != 0 {
		if parseStatus == 1 {
			os.Exit(1)
		}
		return
	}

	state := catState{
		opts:        opts,
		out:         bufio.NewWriterSize(os.Stdout, 32*1024),
		atLineStart: true,
	}
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
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", programName(), name, err)
				ok = false
				continue
			}
			r = f
		}

		err := state.copy(r)
		if f != nil {
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", programName(), name, err)
			ok = false
			if state.err != nil {
				break
			}
		}
	}

	if state.pendingCR {
		state.writeByte('\r')
	}
	if err := state.out.Flush(); err != nil && state.err == nil {
		state.err = err
	}
	if state.err != nil {
		fmt.Fprintf(os.Stderr, "%s: write error: %v\n", programName(), state.err)
		ok = false
	}
	if !ok {
		os.Exit(1)
	}
}
