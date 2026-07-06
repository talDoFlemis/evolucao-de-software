package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	number, nonblank, squeeze, ends, nonprinting, tabs bool
}

type processor struct {
	o        options
	w        *bufio.Writer
	line     uint64
	atStart  bool
	newlines int
	pending  bool
	err      error
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: cat [OPTION]... [FILE]...")
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
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
}

func setOption(o *options, c byte) bool {
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

func parse(args []string) (options, []string, int) {
	var o options
	var files []string
	parsing := true
	long := map[string]byte{"show-all":'A', "number-nonblank":'b', "show-ends":'E', "number":'n', "squeeze-blank":'s', "show-tabs":'T', "show-nonprinting":'v'}
	for _, a := range args {
		if parsing && a == "--" { parsing = false; continue }
		if parsing && strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			if name == "help" { usage(os.Stdout); return o, nil, 2 }
			if name == "version" { fmt.Fprintln(os.Stdout, "cat (Go implementation) 1.0"); return o, nil, 2 }
			c, ok := long[name]
			if !ok { fmt.Fprintf(os.Stderr, "cat: unrecognized option '--%s'\n", name); return o, nil, 1 }
			setOption(&o, c)
		} else if parsing && len(a) > 1 && a[0] == '-' {
			for i := 1; i < len(a); i++ {
				if !setOption(&o, a[i]) { fmt.Fprintf(os.Stderr, "cat: invalid option -- '%c'\n", a[i]); return o, nil, 1 }
			}
		} else { files = append(files, a) }
	}
	return o, files, 0
}

func (p *processor) writeByte(b byte) { if p.err == nil { p.err = p.w.WriteByte(b) } }
func (p *processor) writeString(s string) { if p.err == nil { _, p.err = p.w.WriteString(s) } }
func (p *processor) numberLine() { p.line++; p.writeString(fmt.Sprintf("%6d\t", p.line)) }

func (p *processor) visible(b byte) {
	if b >= 32 && b < 127 { p.writeByte(b); return }
	if b == 127 { p.writeString("^?"); return }
	if b >= 128 {
		p.writeString("M-"); b -= 128
		if b >= 32 && b < 127 { p.writeByte(b) } else if b == 127 { p.writeString("^?") } else { p.writeByte('^'); p.writeByte(b + 64) }
		return
	}
	p.writeByte('^'); p.writeByte(b + 64)
}

func (p *processor) process(b byte) {
	if p.pending {
		p.pending = false
		if b == '\n' { p.writeString("^M"); p.processNewline(); return }
		p.writeByte('\r')
	}
	if b == '\r' && p.o.ends && !p.o.nonprinting { p.pending = true; return }
	if b == '\n' { p.processNewline(); return }
	p.newlines = 0
	if p.atStart {
		if p.o.number { p.numberLine() }
		p.atStart = false
	}
	if p.o.nonprinting {
		if b == '\t' && !p.o.tabs { p.writeByte(b) } else { p.visible(b) }
	} else if b == '\t' && p.o.tabs { p.writeString("^I") } else { p.writeByte(b) }
}

func (p *processor) processNewline() {
	p.newlines++
	if p.o.squeeze && p.newlines > 1 { p.atStart = true; return }
	if p.atStart && p.o.number && !p.o.nonblank { p.numberLine() }
	if p.o.ends { p.writeByte('$') }
	p.writeByte('\n')
	p.atStart = true
}

func (p *processor) copy(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] { p.process(b); if p.err != nil { return p.err } }
		if err == io.EOF { return nil }
		if err != nil { return err }
	}
}

func main() {
	o, files, status := parse(os.Args[1:])
	if status == 2 { return }
	if status != 0 { usage(os.Stderr); os.Exit(status) }
	if len(files) == 0 { files = []string{"-"} }
	p := &processor{o:o, w:bufio.NewWriterSize(os.Stdout, 32*1024), atStart:true}
	ok := true
	for _, name := range files {
		var r io.Reader
		var f *os.File
		if name == "-" { r = os.Stdin } else {
			var err error
			f, err = os.Open(name)
			if err != nil { fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err); ok = false; continue }
			r = f
		}
		if err := p.copy(r); err != nil { fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err); ok = false }
		if f != nil { if err := f.Close(); err != nil { fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err); ok = false } }
	}
	if p.pending { p.writeByte('\r') }
	if err := p.w.Flush(); err != nil || p.err != nil { fmt.Fprintln(os.Stderr, "cat: write error"); ok = false }
	if !ok { os.Exit(1) }
}
