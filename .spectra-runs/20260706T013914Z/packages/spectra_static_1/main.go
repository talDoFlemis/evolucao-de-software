package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
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

type parseResult struct {
	opt     options
	files   []string
	action  string
}

type lineCounter struct {
	buf   [20]byte
	print int
	start int
	end   int
}

func newLineCounter() lineCounter {
	var lc lineCounter
	for i := range lc.buf {
		lc.buf[i] = ' '
	}
	lc.buf[17] = '0'
	lc.buf[18] = '\t'
	lc.print = 12
	lc.start = 17
	lc.end = 17
	return lc
}

func (lc *lineCounter) next() {
	for p := lc.end; p >= lc.start; p-- {
		if lc.buf[p] < '9' {
			lc.buf[p]++
			return
		}
		lc.buf[p] = '0'
	}
	if lc.start > 0 {
		lc.start--
		lc.buf[lc.start] = '1'
	} else {
		lc.buf[0] = '>'
	}
	if lc.start < lc.print {
		lc.print--
	}
}

func (lc *lineCounter) bytes() []byte {
	return lc.buf[lc.print:19]
}

type processor struct {
	w         *bufio.Writer
	opt       options
	lines     lineCounter
	atLineStart bool
	emptyRun  int
	pendingCR bool
}

func newProcessor(w *bufio.Writer, opt options) *processor {
	return &processor{
		w:           w,
		opt:         opt,
		lines:       newLineCounter(),
		atLineStart: true,
	}
}

func (p *processor) emitLineNumber() error {
	p.lines.next()
	_, err := p.w.Write(p.lines.bytes())
	return err
}

func (p *processor) writeByte(b byte) error {
	return p.w.WriteByte(b)
}

func (p *processor) writeString(s string) error {
	_, err := p.w.WriteString(s)
	return err
}

func (p *processor) writeVisible(b byte) error {
	if b >= 32 {
		switch {
		case b < 127:
			return p.writeByte(b)
		case b == 127:
			return p.writeString("^?")
		default:
			if err := p.writeString("M-"); err != nil {
				return err
			}
			c := b - 128
			switch {
			case c >= 32 && c < 127:
				return p.writeByte(c)
			case c == 127:
				return p.writeString("^?")
			default:
				if err := p.writeByte('^'); err != nil {
					return err
				}
				return p.writeByte(c + 64)
			}
		}
	}
	if b == '\t' && !p.opt.showTabs {
		return p.writeByte('\t')
	}
	if err := p.writeByte('^'); err != nil {
		return err
	}
	return p.writeByte(b + 64)
}

func (p *processor) processByte(b byte) error {
	if p.pendingCR {
		if p.opt.showEnds && !p.opt.showNonprinting && b == '\n' {
			if err := p.writeString("^M"); err != nil {
				return err
			}
		} else {
			if err := p.writeByte('\r'); err != nil {
				return err
			}
		}
		p.pendingCR = false
	}

	if b == '\n' {
		if p.atLineStart {
			if p.opt.squeezeBlank && p.emptyRun >= 1 {
				if p.emptyRun < 2 {
					p.emptyRun = 2
				}
				return nil
			}
			if p.opt.number && !p.opt.numberNonblank {
				if err := p.emitLineNumber(); err != nil {
					return err
				}
			}
			if p.opt.showEnds {
				if err := p.writeByte('$'); err != nil {
					return err
				}
			}
			if err := p.writeByte('\n'); err != nil {
				return err
			}
			if p.emptyRun < 2 {
				p.emptyRun++
			}
			return nil
		}
		if p.opt.showEnds {
			if err := p.writeByte('$'); err != nil {
				return err
			}
		}
		if err := p.writeByte('\n'); err != nil {
			return err
		}
		p.atLineStart = true
		p.emptyRun = 0
		return nil
	}

	if p.atLineStart {
		if p.opt.number {
			if err := p.emitLineNumber(); err != nil {
				return err
			}
		}
		p.atLineStart = false
		p.emptyRun = 0
	}

	if p.opt.showNonprinting {
		return p.writeVisible(b)
	}
	if b == '\t' && p.opt.showTabs {
		return p.writeString("^I")
	}
	if p.opt.showEnds && b == '\r' {
		p.pendingCR = true
		return nil
	}
	return p.writeByte(b)
}

func (p *processor) process(r io.Reader) error {
	buf := make([]byte, 32768)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if werr := p.processByte(b); werr != nil {
					return werr
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

func simpleCopy(w *bufio.Writer, r io.Reader) error {
	buf := make([]byte, 32768)
	_, err := io.CopyBuffer(w, r, buf)
	return err
}

func usage(out io.Writer) {
	fmt.Fprintln(out, "Usage: cat [OPTION]... [FILE]...")
	fmt.Fprintln(out, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(out, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(out, "  -e                       equivalent to -vE")
	fmt.Fprintln(out, "  -E, --show-ends          display $ at end of each line")
	fmt.Fprintln(out, "  -n, --number             number all output lines")
	fmt.Fprintln(out, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(out, "  -t                       equivalent to -vT")
	fmt.Fprintln(out, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(out, "  -u                       ignored")
	fmt.Fprintln(out, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Fprintln(out, "      --help     display this help and exit")
	fmt.Fprintln(out, "      --version  output version information and exit")
}

func version(out io.Writer) {
	fmt.Fprintln(out, "cat (go translation) 1.0")
}

func parseOptions(args []string) (parseResult, error) {
	res := parseResult{}
	filesStart := len(args)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			filesStart = i + 1
			break
		}
		if arg == "-" || len(arg) == 0 || arg[0] != '-' {
			filesStart = i
			break
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				res.opt.showNonprinting = true
				res.opt.showEnds = true
				res.opt.showTabs = true
			case "--number-nonblank":
				res.opt.number = true
				res.opt.numberNonblank = true
			case "--number":
				res.opt.number = true
			case "--squeeze-blank":
				res.opt.squeezeBlank = true
			case "--show-nonprinting":
				res.opt.showNonprinting = true
			case "--show-ends":
				res.opt.showEnds = true
			case "--show-tabs":
				res.opt.showTabs = true
			case "--help":
				res.action = "help"
			case "--version":
				res.action = "version"
			default:
				return res, fmt.Errorf("invalid option -- %s", arg[2:])
			}
			continue
		}
		for j := 1; j < len(arg); j++ {
			switch arg[j] {
			case 'A':
				res.opt.showNonprinting = true
				res.opt.showEnds = true
				res.opt.showTabs = true
			case 'b':
				res.opt.number = true
				res.opt.numberNonblank = true
			case 'e':
				res.opt.showNonprinting = true
				res.opt.showEnds = true
			case 'E':
				res.opt.showEnds = true
			case 'n':
				res.opt.number = true
			case 's':
				res.opt.squeezeBlank = true
			case 't':
				res.opt.showNonprinting = true
				res.opt.showTabs = true
			case 'T':
				res.opt.showTabs = true
			case 'u':
			case 'v':
				res.opt.showNonprinting = true
			default:
				return res, fmt.Errorf("invalid option -- %c", arg[j])
			}
		}
	}
	if filesStart == len(args) {
		res.files = []string{"-"}
	} else if filesStart < len(args) {
		res.files = args[filesStart:]
	} else {
		res.files = []string{"-"}
	}
	if len(res.files) == 0 {
		res.files = []string{"-"}
	}
	return res, nil
}

func reportError(name string, err error) {
	fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
}

func isFormatted(opt options) bool {
	return opt.number || opt.showEnds || opt.showNonprinting || opt.showTabs || opt.squeezeBlank
}

func main() {
	res, err := parseOptions(os.Args[1:])
	if err != nil {
		reportError("option", err)
		usage(os.Stderr)
		os.Exit(1)
	}
	if res.action == "help" {
		usage(os.Stdout)
		return
	}
	if res.action == "version" {
		version(os.Stdout)
		return
	}

	w := bufio.NewWriterSize(os.Stdout, 32768)
	formatted := isFormatted(res.opt)
	proc := newProcessor(w, res.opt)
	ok := true

	for _, name := range res.files {
		var f *os.File
		if name == "-" {
			f = os.Stdin
		} else {
			f, err = os.Open(name)
			if err != nil {
				reportError(name, err)
				ok = false
				continue
			}
		}

		if _, err := f.Stat(); err != nil {
			reportError(name, err)
			ok = false
			if f != os.Stdin {
				_ = f.Close()
			}
			continue
		}

		if formatted {
			err = proc.process(f)
		} else {
			err = simpleCopy(w, f)
		}

		if flushErr := w.Flush(); flushErr != nil {
			reportError("write error", flushErr)
			if f != os.Stdin {
				_ = f.Close()
			}
			os.Exit(1)
		}

		if f != os.Stdin {
			if closeErr := f.Close(); closeErr != nil {
				reportError(name, closeErr)
				ok = false
			}
		}

		if err != nil {
			reportError(name, err)
			ok = false
		}
	}

	if formatted && proc.pendingCR {
		if err := proc.writeByte('\r'); err != nil {
			reportError("write error", err)
			os.Exit(1)
		}
	}
	if err := w.Flush(); err != nil {
		reportError("write error", err)
		os.Exit(1)
	}
	if !ok {
		os.Exit(1)
	}
}
