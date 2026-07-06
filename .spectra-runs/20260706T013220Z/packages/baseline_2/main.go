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
	number, nonblank, squeeze, ends, nonprinting, tabs bool
}

type catState struct {
	opts         options
	out          *bufio.Writer
	line         uint64
	lineStart    bool
	previousBlank bool
	pendingCR    bool
}

func usage(name string, out io.Writer) {
	fmt.Fprintf(out, `Usage: %s [OPTION]... [FILE]...
Concatenate FILE(s) to standard output.

  -A, --show-all           equivalent to -vET
  -b, --number-nonblank    number nonempty output lines, overrides -n
  -e                       equivalent to -vE
  -E, --show-ends          display $ or ^M$ at end of each line
  -n, --number             number all output lines
  -s, --squeeze-blank      suppress repeated empty output lines
  -t                       equivalent to -vT
  -T, --show-tabs          display TAB characters as ^I
  -u                       ignored
  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB
      --help               display this help and exit
      --version            output version information and exit
`, name)
}

func applyOption(o *options, c byte) bool {
	switch c {
	case 'A': o.nonprinting, o.ends, o.tabs = true, true, true
	case 'b': o.number, o.nonblank = true, true
	case 'e': o.nonprinting, o.ends = true, true
	case 'E': o.ends = true
	case 'n': o.number = true
	case 's': o.squeeze = true
	case 't': o.nonprinting, o.tabs = true, true
	case 'T': o.tabs = true
	case 'u':
	case 'v': o.nonprinting = true
	default: return false
	}
	return true
}

func parse(args []string, name string) (options, []string, int) {
	var o options
	var files []string
	parsing := true
	long := map[string]byte{
		"--show-all": 'A', "--number-nonblank": 'b', "--show-ends": 'E',
		"--number": 'n', "--squeeze-blank": 's', "--show-tabs": 'T',
		"--show-nonprinting": 'v',
	}
	for _, arg := range args {
		if parsing && arg == "--" { parsing = false; continue }
		if parsing && arg == "--help" { usage(name, os.Stdout); return o, nil, 2 }
		if parsing && arg == "--version" { fmt.Printf("%s (Go implementation) 1.0\n", name); return o, nil, 2 }
		if parsing && strings.HasPrefix(arg, "--") {
			if c, ok := long[arg]; ok { applyOption(&o, c); continue }
			fmt.Fprintf(os.Stderr, "%s: unrecognized option %q\n", name, arg)
			fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", name)
			return o, nil, 1
		}
		if parsing && len(arg) > 1 && arg[0] == '-' {
			for i := 1; i < len(arg); i++ {
				if !applyOption(&o, arg[i]) {
					fmt.Fprintf(os.Stderr, "%s: invalid option -- %q\n", name, arg[i])
					fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", name)
					return o, nil, 1
				}
			}
			continue
		}
		files = append(files, arg)
	}
	if len(files) == 0 { files = []string{"-"} }
	return o, files, 0
}

func (s *catState) prefix() error {
	s.line++
	_, err := fmt.Fprintf(s.out, "%6d\t", s.line)
	return err
}

func (s *catState) writeVisible(b byte) error {
	if !s.opts.nonprinting {
		if b == '\t' && s.opts.tabs { _, err := s.out.WriteString("^I"); return err }
		return s.out.WriteByte(b)
	}
	if b >= 32 && b < 127 { return s.out.WriteByte(b) }
	if b == '\t' && !s.opts.tabs { return s.out.WriteByte(b) }
	if b == 127 { _, err := s.out.WriteString("^?"); return err }
	if b < 32 { return s.out.WriteByte('^') /* second byte below */ }
	if _, err := s.out.WriteString("M-"); err != nil { return err }
	b -= 128
	if b < 32 { if err := s.out.WriteByte('^'); err != nil { return err }; return s.out.WriteByte(b + 64) }
	if b == 127 { _, err := s.out.WriteString("^?"); return err }
	return s.out.WriteByte(b)
}

func (s *catState) process(b byte) error {
	if s.pendingCR {
		s.pendingCR = false
		if b == '\n' { if _, err := s.out.WriteString("^M"); err != nil { return err } } else {
			if err := s.out.WriteByte('\r'); err != nil { return err }
		}
	}
	if b == '\n' {
		blank := s.lineStart
		if s.opts.squeeze && blank && s.previousBlank { return nil }
		if s.opts.number && !s.opts.nonblank && s.lineStart { if err := s.prefix(); err != nil { return err } }
		if s.opts.ends { if err := s.out.WriteByte('$'); err != nil { return err } }
		if err := s.out.WriteByte('\n'); err != nil { return err }
		s.previousBlank, s.lineStart = blank, true
		return nil
	}
	if s.lineStart && s.opts.number { if err := s.prefix(); err != nil { return err } }
	s.lineStart = false
	if b == '\r' && s.opts.ends && !s.opts.nonprinting { s.pendingCR = true; return nil }
	if s.opts.nonprinting && b < 32 {
		if b == '\t' && !s.opts.tabs { return s.out.WriteByte(b) }
		if err := s.out.WriteByte('^'); err != nil { return err }
		return s.out.WriteByte(b + 64)
	}
	return s.writeVisible(b)
}

func (s *catState) copy(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] { if e := s.process(b); e != nil { return e } }
		if err == io.EOF { return nil }
		if err != nil { return err }
	}
}

func main() {
	name := filepath.Base(os.Args[0])
	o, files, status := parse(os.Args[1:], name)
	if status != 0 { if status == 1 { os.Exit(1) }; return }
	s := catState{opts: o, out: bufio.NewWriterSize(os.Stdout, 32*1024), lineStart: true}
	ok := true
	for _, file := range files {
		var r io.Reader = os.Stdin
		var f *os.File
		if file != "-" {
			var err error
			f, err = os.Open(file)
			if err != nil { fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, file, err); ok = false; continue }
			r = f
		}
		if err := s.copy(r); err != nil { fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, file, err); ok = false }
		if f != nil { if err := f.Close(); err != nil { fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, file, err); ok = false } }
	}
	if s.pendingCR { if err := s.out.WriteByte('\r'); err != nil { ok = false } }
	if err := s.out.Flush(); err != nil { fmt.Fprintf(os.Stderr, "%s: write error: %v\n", name, err); ok = false }
	if !ok { os.Exit(1) }
}
