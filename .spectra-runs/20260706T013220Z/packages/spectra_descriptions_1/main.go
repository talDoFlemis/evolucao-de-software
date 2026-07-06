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
	number, nonblank, squeeze bool
	ends, tabs, nonprinting  bool
}

type formatter struct {
	opts          options
	out           *bufio.Writer
	line          uint64
	atLineStart   bool
	previousBlank bool
	haveLine      bool
	pendingCR     bool
	err           error
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION]... [FILE]...\nConcatenate FILE(s) to standard output.\n\n", filepath.Base(os.Args[0]))
	fmt.Fprintln(w, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(w, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(w, "  -e                       equivalent to -vE")
	fmt.Fprintln(w, "  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Fprintln(w, "  -n, --number             number all output lines")
	fmt.Fprintln(w, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(w, "  -t                       equivalent to -vT")
	fmt.Fprintln(w, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(w, "  -u                       ignored")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for LF and TAB")
	fmt.Fprintln(w, "      --help               display this help and exit")
	fmt.Fprintln(w, "      --version            output version information and exit")
}

func setOption(o *options, c byte) bool {
	switch c {
	case 'A':
		o.nonprinting, o.ends, o.tabs = true, true, true
	case 'b':
		o.number, o.nonblank = true, true
	case 'e':
		o.nonprinting, o.ends = true, true
	case 'E':
		o.ends = true
	case 'n':
		o.number = true
	case 's':
		o.squeeze = true
	case 't':
		o.nonprinting, o.tabs = true, true
	case 'T':
		o.tabs = true
	case 'u':
	case 'v':
		o.nonprinting = true
	default:
		return false
	}
	return true
}

func parse(args []string) (options, []string, int) {
	var o options
	var files []string
	optionsEnabled := true
	long := map[string]byte{
		"show-all": 'A', "number-nonblank": 'b', "show-ends": 'E',
		"number": 'n', "squeeze-blank": 's', "show-tabs": 'T',
		"show-nonprinting": 'v',
	}
	for _, arg := range args {
		if optionsEnabled && arg == "--" {
			optionsEnabled = false
			continue
		}
		if optionsEnabled && strings.HasPrefix(arg, "--") {
			name := arg[2:]
			if name == "help" {
				usage(os.Stdout)
				return o, nil, 1
			}
			if name == "version" {
				fmt.Fprintln(os.Stdout, "cat (SPECTRA Go translation) 1.0")
				return o, nil, 1
			}
			c, ok := long[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "%s: unrecognized option %q\n", filepath.Base(os.Args[0]), arg)
				return o, nil, 2
			}
			setOption(&o, c)
			continue
		}
		if optionsEnabled && len(arg) > 1 && arg[0] == '-' {
			for i := 1; i < len(arg); i++ {
				if !setOption(&o, arg[i]) {
					fmt.Fprintf(os.Stderr, "%s: invalid option -- %c\n", filepath.Base(os.Args[0]), arg[i])
					return o, nil, 2
				}
			}
			continue
		}
		files = append(files, arg)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	return o, files, 0
}

func (f *formatter) write(p []byte) {
	if f.err == nil {
		_, f.err = f.out.Write(p)
	}
}

func (f *formatter) startLine() {
	if !f.atLineStart || !f.opts.number {
		return
	}
	f.line++
	f.write([]byte(fmt.Sprintf("%6d\t", f.line)))
	f.atLineStart = false
}

func (f *formatter) visible(b byte) {
	if !f.opts.nonprinting {
		if b == '\t' && f.opts.tabs {
			f.write([]byte("^I"))
		} else {
			f.write([]byte{b})
		}
		return
	}
	if b == '\t' && !f.opts.tabs {
		f.write([]byte{'\t'})
	} else if b >= 32 && b < 127 {
		f.write([]byte{b})
	} else if b == 127 {
		f.write([]byte("^?"))
	} else if b < 32 {
		f.write([]byte{'^', b + 64})
	} else {
		f.write([]byte("M-"))
		c := b - 128
		if c >= 32 && c < 127 {
			f.write([]byte{c})
		} else if c == 127 {
			f.write([]byte("^?"))
		} else {
			f.write([]byte{'^', c + 64})
		}
	}
}

func (f *formatter) flushCR() {
	if f.pendingCR {
		f.visible('\r')
		f.pendingCR = false
	}
}

func (f *formatter) byte(b byte) {
	if f.err != nil {
		return
	}
	if f.pendingCR {
		if b == '\n' {
			f.pendingCR = false
			f.newline(true)
			return
		}
		f.flushCR()
	}
	if b == '\n' {
		f.newline(false)
		return
	}
	if f.atLineStart {
		f.startLine()
	}
	f.haveLine = true
	f.previousBlank = false
	if b == '\r' && f.opts.ends {
		f.pendingCR = true
		return
	}
	f.visible(b)
}

func (f *formatter) newline(hadCR bool) {
	empty := !f.haveLine
	if empty && f.opts.squeeze && f.previousBlank {
		return
	}
	if empty && f.opts.number && !f.opts.nonblank {
		f.startLine()
	}
	if hadCR {
		f.write([]byte("^M"))
	}
	if f.opts.ends {
		f.write([]byte{'$'})
	}
	f.write([]byte{'\n'})
	f.previousBlank = empty
	f.haveLine = false
	f.atLineStart = true
}

func copyInput(r io.Reader, f *formatter) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			f.byte(b)
		}
		if f.err != nil {
			return f.err
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
	o, files, status := parse(os.Args[1:])
	if status == 1 {
		return
	}
	if status == 2 {
		usage(os.Stderr)
		os.Exit(1)
	}

	out := bufio.NewWriterSize(os.Stdout, 32*1024)
	f := &formatter{opts: o, out: out, atLineStart: true}
	ok := true
	stdoutInfo, _ := os.Stdout.Stat()

	for _, name := range files {
		var r io.Reader
		var file *os.File
		if name == "-" {
			r = os.Stdin
		} else {
			var err error
			file, err = os.Open(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", filepath.Base(os.Args[0]), name, err)
				ok = false
				continue
			}
			if info, err := file.Stat(); err == nil && stdoutInfo != nil && os.SameFile(info, stdoutInfo) {
				fmt.Fprintf(os.Stderr, "%s: %s: input file is output file\n", filepath.Base(os.Args[0]), name)
				file.Close()
				ok = false
				continue
			}
			r = file
		}

		if err := copyInput(r, f); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", filepath.Base(os.Args[0]), name, err)
			ok = false
		}
		if file != nil {
			if err := file.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", filepath.Base(os.Args[0]), name, err)
				ok = false
			}
		}
	}

	f.flushCR()
	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: write error: %v\n", filepath.Base(os.Args[0]), err)
		ok = false
	}
	if !ok || f.err != nil {
		os.Exit(1)
	}
}
