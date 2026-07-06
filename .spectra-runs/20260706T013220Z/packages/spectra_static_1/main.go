package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type options struct {
	number, nonblank, squeeze bool
	showEnds, showTabs, showNonprinting bool
}

type formatter struct {
	opts          options
	out           *bufio.Writer
	line          uint64
	atLineStart   bool
	consecutiveLF int
	pendingCR     bool
	err           error
}

func (f *formatter) write(p []byte) {
	if f.err == nil {
		_, f.err = f.out.Write(p)
	}
}

func (f *formatter) prefix() {
	f.line++
	f.write([]byte(fmt.Sprintf("%6d\t", f.line)))
}

func (f *formatter) beginNonempty() {
	if f.atLineStart {
		if f.opts.number {
			f.prefix()
		}
		f.atLineStart = false
	}
	f.consecutiveLF = 0
}

func (f *formatter) renderByte(b byte) {
	if f.opts.showTabs && b == '\t' {
		f.write([]byte("^I"))
		return
	}
	if !f.opts.showNonprinting || (b == '\t' && !f.opts.showTabs) {
		f.write([]byte{b})
		return
	}
	switch {
	case b < 0x20:
		f.write([]byte{'^', b + 0x40})
	case b < 0x7f:
		f.write([]byte{b})
	case b == 0x7f:
		f.write([]byte("^?"))
	case b < 0xa0:
		f.write([]byte{'M', '-', '^', b - 0x80 + 0x40})
	case b < 0xff:
		f.write([]byte{'M', '-', b - 0x80})
	default:
		f.write([]byte("M-^?"))
	}
}

func (f *formatter) newline() {
	if f.opts.squeeze && f.consecutiveLF >= 2 {
		return
	}
	if f.atLineStart && f.opts.number && !f.opts.nonblank {
		f.prefix()
	}
	if f.opts.showEnds {
		f.write([]byte{'$'})
	}
	f.write([]byte{'\n'})
	f.atLineStart = true
	if f.consecutiveLF < 2 {
		f.consecutiveLF++
	}
}

func (f *formatter) consume(b byte) {
	if f.pendingCR {
		if b == '\n' {
			f.write([]byte("^M"))
			f.pendingCR = false
			f.newline()
			return
		}
		f.renderByte('\r')
		f.pendingCR = false
	}
	if b == '\n' {
		f.newline()
		return
	}
	f.beginNonempty()
	if f.opts.showEnds && b == '\r' {
		f.pendingCR = true
		return
	}
	f.renderByte(b)
}

func (f *formatter) finish() {
	if f.pendingCR {
		f.renderByte('\r')
		f.pendingCR = false
	}
}

func parseArgs(args []string) (options, []string, string, error) {
	var o options
	var files []string
	stopped := false
	for _, arg := range args {
		if stopped || arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			stopped = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--number-nonblank": o.number, o.nonblank = true, true
			case "--number": o.number = true
			case "--squeeze-blank": o.squeeze = true
			case "--show-nonprinting": o.showNonprinting = true
			case "--show-ends": o.showEnds = true
			case "--show-tabs": o.showTabs = true
			case "--show-all": o.showNonprinting, o.showEnds, o.showTabs = true, true, true
			case "--help": return o, files, "help", nil
			case "--version": return o, files, "version", nil
			default: return o, nil, "", fmt.Errorf("unrecognized option %q", arg)
			}
			continue
		}
		for _, c := range arg[1:] {
			switch c {
			case 'b': o.number, o.nonblank = true, true
			case 'e': o.showNonprinting, o.showEnds = true, true
			case 'n': o.number = true
			case 's': o.squeeze = true
			case 't': o.showNonprinting, o.showTabs = true, true
			case 'u':
			case 'v': o.showNonprinting = true
			case 'A': o.showNonprinting, o.showEnds, o.showTabs = true, true, true
			case 'E': o.showEnds = true
			case 'T': o.showTabs = true
			default: return o, nil, "", fmt.Errorf("invalid option -- %c", c)
			}
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	return o, files, "", nil
}

func usage() {
	fmt.Print("Usage: cat [OPTION]... [FILE]...\nConcatenate FILE(s) to standard output.\n\n  -A, --show-all\n  -b, --number-nonblank\n  -e\n  -E, --show-ends\n  -n, --number\n  -s, --squeeze-blank\n  -t\n  -T, --show-tabs\n  -u\n  -v, --show-nonprinting\n      --help\n      --version\n")
}

func sameFileDanger(in *os.File, inInfo, outInfo os.FileInfo) bool {
	if !os.SameFile(inInfo, outInfo) {
		return false
	}
	inPos, err1 := in.Seek(0, io.SeekCurrent)
	outPos, err2 := os.Stdout.Seek(0, io.SeekCurrent)
	return err1 == nil && err2 == nil && inPos < outPos
}

func main() {
	o, files, action, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: %v\nTry 'cat --help' for more information.\n", err)
		os.Exit(1)
	}
	if action == "help" {
		usage()
		return
	}
	if action == "version" {
		fmt.Println("cat (SPECTRA Go translation) 1.0")
		return
	}
	outInfo, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: standard output: %v\n", err)
		os.Exit(1)
	}
	formatted := o.number || o.squeeze || o.showEnds || o.showTabs || o.showNonprinting
	writer := bufio.NewWriterSize(os.Stdout, 32*1024)
	f := formatter{opts: o, out: writer, atLineStart: true}
	ok := true
	buf := make([]byte, 32*1024)

	for _, name := range files {
		in := os.Stdin
		closeInput := false
		if name != "-" {
			in, err = os.Open(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
				ok = false
				continue
			}
			closeInput = true
		}
		info, statErr := in.Stat()
		if statErr != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, statErr)
			ok = false
		} else if sameFileDanger(in, info, outInfo) {
			fmt.Fprintf(os.Stderr, "cat: %s: input file is output file\n", name)
			ok = false
		} else {
			for {
				n, readErr := in.Read(buf)
				if n > 0 {
					if formatted {
						for _, b := range buf[:n] { f.consume(b) }
					} else {
						f.write(buf[:n])
					}
				}
				if f.err != nil { break }
				if readErr == io.EOF { break }
				if readErr != nil {
					fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, readErr)
					ok = false
					break
				}
			}
		}
		if closeInput {
			if closeErr := in.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, closeErr)
				ok = false
			}
		}
		if f.err != nil { break }
	}
	if formatted { f.finish() }
	if f.err == nil { f.err = writer.Flush() }
	if f.err != nil {
		fmt.Fprintf(os.Stderr, "cat: write error: %v\n", f.err)
		os.Exit(1)
	}
	if !ok { os.Exit(1) }
	_ = strconv.IntSize
}
