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
	number           bool
	numberNonblank   bool
	squeezeBlank     bool
	showEnds         bool
	showNonprinting  bool
	showTabs         bool
}

type processor struct {
	opts        options
	w           *bufio.Writer
	line        int64
	newlines    int
	pendingCR   bool
}

func main() {
	opts, files, err := parseArgs(os.Args[1:])
	if err != nil {
		fprintf(os.Stderr, "%s: %v\n", progName(), err)
		os.Exit(1)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	formatted := opts.number || opts.showEnds || opts.showNonprinting || opts.showTabs || opts.squeezeBlank
	out := bufio.NewWriterSize(os.Stdout, 32*1024)
	ok := true
	p := &processor{opts: opts, w: out}

	for _, name := range files {
		var f *os.File
		if name == "-" {
			f = os.Stdin
		} else {
			var openErr error
			f, openErr = os.Open(name)
			if openErr != nil {
				fprintf(os.Stderr, "%s: %s: %v\n", progName(), name, openErr)
				ok = false
				continue
			}
		}

		if formatted {
			if err := p.process(f); err != nil {
				fprintf(os.Stderr, "%s: %s: %v\n", progName(), name, err)
				ok = false
			}
		} else {
			if _, err := io.Copy(out, f); err != nil {
				fprintf(os.Stderr, "%s: %s: %v\n", progName(), name, err)
				ok = false
			}
		}

		if name != "-" {
			if err := f.Close(); err != nil {
				fprintf(os.Stderr, "%s: %s: %v\n", progName(), name, err)
				ok = false
			}
		}
	}

	if formatted {
		if err := p.finish(); err != nil {
			fprintf(os.Stderr, "%s: %v\n", progName(), err)
			ok = false
		}
	}
	if err := out.Flush(); err != nil {
		fprintf(os.Stderr, "%s: %v\n", progName(), err)
		ok = false
	}
	if !ok {
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	parsingOpts := true

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !parsingOpts || arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			parsingOpts = false
			continue
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
			default:
				return options{}, nil, fmt.Errorf("unrecognized option '%s'", arg)
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
				opts.showEnds = true
				opts.showNonprinting = true
			case 'E':
				opts.showEnds = true
			case 'n':
				opts.number = true
			case 's':
				opts.squeezeBlank = true
			case 't':
				opts.showTabs = true
				opts.showNonprinting = true
			case 'T':
				opts.showTabs = true
			case 'u':
				// Ignored.
			case 'v':
				opts.showNonprinting = true
			default:
				return options{}, nil, fmt.Errorf("invalid option -- '%c'", arg[j])
			}
		}
	}

	return opts, files, nil
}

func (p *processor) process(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if err := p.writeByte(b); err != nil {
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

func (p *processor) finish() error {
	if p.pendingCR {
		p.pendingCR = false
		return p.w.WriteByte('\r')
	}
	return nil
}

func (p *processor) writeByte(b byte) error {
	if p.pendingCR {
		if b == '\n' && p.opts.showEnds && !p.opts.showNonprinting {
			if _, err := p.w.WriteString("^M"); err != nil {
				return err
			}
		} else {
			if err := p.w.WriteByte('\r'); err != nil {
				return err
			}
		}
		p.pendingCR = false
	}

	if b == '\n' {
		return p.writeNewline()
	}

	if p.opts.number && p.newlines >= 0 {
		p.line++
		if _, err := fmt.Fprintf(p.w, "%6d\t", p.line); err != nil {
			return err
		}
	}
	p.newlines = -1

	if p.opts.showNonprinting {
		return p.writeQuoted(b)
	}
	if b == '\t' && p.opts.showTabs {
		_, err := p.w.WriteString("^I")
		return err
	}
	if b == '\r' && p.opts.showEnds {
		p.pendingCR = true
		return nil
	}
	return p.w.WriteByte(b)
}

func (p *processor) writeNewline() error {
	if p.newlines >= 0 {
		p.newlines++
		if p.newlines >= 2 {
			p.newlines = 2
			if p.opts.squeezeBlank {
				return nil
			}
		}
		if p.opts.number && !p.opts.numberNonblank {
			p.line++
			if _, err := fmt.Fprintf(p.w, "%6d\t", p.line); err != nil {
				return err
			}
		}
	} else {
		p.newlines = 0
	}

	if p.opts.showEnds {
		if _, err := p.w.WriteString("$"); err != nil {
			return err
		}
	}
	return p.w.WriteByte('\n')
}

func (p *processor) writeQuoted(b byte) error {
	switch {
	case b >= 32:
		switch {
		case b < 127:
			return p.w.WriteByte(b)
		case b == 127:
			_, err := p.w.WriteString("^?")
			return err
		default:
			if _, err := p.w.WriteString("M-"); err != nil {
				return err
			}
			if b >= 160 {
				if b < 255 {
					return p.w.WriteByte(b - 128)
				}
				_, err := p.w.WriteString("^?")
				return err
			}
			if err := p.w.WriteByte('^'); err != nil {
				return err
			}
			return p.w.WriteByte(b - 64)
		}
	case b == '\t' && !p.opts.showTabs:
		return p.w.WriteByte('\t')
	default:
		if err := p.w.WriteByte('^'); err != nil {
			return err
		}
		return p.w.WriteByte(b + 64)
	}
}

func progName() string {
	return filepath.Base(os.Args[0])
}

func fprintf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
