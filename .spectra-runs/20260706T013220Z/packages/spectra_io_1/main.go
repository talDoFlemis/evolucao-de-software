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

type processor struct {
	o                    options
	w                    *bufio.Writer
	line                  uint64
	atLineStart, prevBlank bool
	pendingCR             bool
}

func (p *processor) numberLine() error {
	p.line++
	_, err := fmt.Fprintf(p.w, "%6d\t", p.line)
	return err
}

func (p *processor) writeVisible(b byte) error {
	if !p.o.nonprinting {
		if b == '\t' && p.o.tabs {
			_, err := p.w.WriteString("^I")
			return err
		}
		return p.w.WriteByte(b)
	}
	if b == '\t' && !p.o.tabs {
		return p.w.WriteByte(b)
	}
	if b >= 32 && b < 127 {
		return p.w.WriteByte(b)
	}
	if b == 127 {
		_, err := p.w.WriteString("^?")
		return err
	}
	if b >= 128 {
		if _, err := p.w.WriteString("M-"); err != nil {
			return err
		}
		c := b - 128
		if c >= 32 && c < 127 {
			return p.w.WriteByte(c)
		}
		if c == 127 {
			_, err := p.w.WriteString("^?")
			return err
		}
		if err := p.w.WriteByte('^'); err != nil {
			return err
		}
		return p.w.WriteByte(c + 64)
	}
	if err := p.w.WriteByte('^'); err != nil {
		return err
	}
	return p.w.WriteByte(b + 64)
}

func (p *processor) content(b byte) error {
	if p.atLineStart {
		if p.o.number && (!p.o.nonblank || b != '\n') {
			if err := p.numberLine(); err != nil {
				return err
			}
		}
		p.atLineStart = false
	}
	return p.writeVisible(b)
}

func (p *processor) newline(crlf bool) error {
	blank := p.atLineStart
	if blank && p.o.squeeze && p.prevBlank {
		return nil
	}
	if p.atLineStart && p.o.number && !p.o.nonblank {
		if err := p.numberLine(); err != nil {
			return err
		}
	}
	if crlf {
		if p.o.nonprinting {
			if _, err := p.w.WriteString("^M"); err != nil {
				return err
			}
		} else if p.o.ends {
			if _, err := p.w.WriteString("^M"); err != nil {
				return err
			}
		} else if err := p.w.WriteByte('\r'); err != nil {
			return err
		}
	}
	if p.o.ends {
		if err := p.w.WriteByte('$'); err != nil {
			return err
		}
	}
	if err := p.w.WriteByte('\n'); err != nil {
		return err
	}
	p.prevBlank = blank
	p.atLineStart = true
	return nil
}

func (p *processor) byte(b byte) error {
	if p.pendingCR {
		p.pendingCR = false
		if b == '\n' {
			return p.newline(true)
		}
		if err := p.content('\r'); err != nil {
			return err
		}
	}
	if b == '\r' && p.o.ends && !p.o.nonprinting {
		p.pendingCR = true
		return nil
	}
	if b == '\n' {
		return p.newline(false)
	}
	return p.content(b)
}

func (p *processor) copy(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			if e := p.byte(b); e != nil {
				return e
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

func usage() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\nConcatenate FILE(s) to standard output.\n", filepath.Base(os.Args[0]))
}

func parse(args []string) (options, []string, bool) {
	var o options
	var files []string
	parsing := true
	long := map[string]byte{"number-nonblank": 'b', "number": 'n', "squeeze-blank": 's', "show-nonprinting": 'v', "show-ends": 'E', "show-tabs": 'T', "show-all": 'A'}
	set := func(c byte) bool {
		switch c {
		case 'b': o.number, o.nonblank = true, true
		case 'e': o.ends, o.nonprinting = true, true
		case 'n': o.number = true
		case 's': o.squeeze = true
		case 't': o.tabs, o.nonprinting = true, true
		case 'u':
		case 'v': o.nonprinting = true
		case 'A': o.nonprinting, o.ends, o.tabs = true, true, true
		case 'E': o.ends = true
		case 'T': o.tabs = true
		default: return false
		}
		return true
	}
	for _, a := range args {
		if parsing && a == "--" {
			parsing = false
		} else if parsing && strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			if name == "help" { usage(); os.Exit(0) }
			if name == "version" { fmt.Println("cat (Go translation)"); os.Exit(0) }
			c, ok := long[name]
			if !ok || !set(c) { fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", filepath.Base(os.Args[0]), a); return o, nil, false }
		} else if parsing && len(a) > 1 && a[0] == '-' {
			for i := 1; i < len(a); i++ {
				if !set(a[i]) { fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", filepath.Base(os.Args[0]), a[i]); return o, nil, false }
			}
		} else {
			files = append(files, a)
		}
	}
	if len(files) == 0 { files = []string{"-"} }
	return o, files, true
}

func main() {
	o, files, ok := parse(os.Args[1:])
	if !ok { os.Exit(1) }
	w := bufio.NewWriter(os.Stdout)
	p := processor{o: o, w: w, atLineStart: true}
	status := 0
	name := filepath.Base(os.Args[0])
	for _, file := range files {
		var r io.Reader
		var f *os.File
		if file == "-" {
			r = os.Stdin
		} else {
			var err error
			f, err = os.Open(file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, file, err)
				status = 1
				continue
			}
			r = f
		}
		if err := p.copy(r); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, file, err)
			status = 1
		}
		if f != nil { _ = f.Close() }
	}
	if p.pendingCR { _ = p.content('\r') }
	if err := w.Flush(); err != nil { fmt.Fprintf(os.Stderr, "%s: write error: %v\n", name, err); status = 1 }
	os.Exit(status)
}
