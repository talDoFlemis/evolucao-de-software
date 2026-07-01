package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	showAll         bool
	numberNonblank   bool
	showEnds        bool
	number          bool
	squeezeBlank    bool
	showTabs        bool
	showNonprinting bool
	showHelp       bool
	showVersion    bool
}

type processor struct {
	w           *bufio.Writer
	opts        options
	lineNum     int64
	atLineStart bool
	blankRun    int
	pendingCR   bool
	writeErr    error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	opts, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if opts.showHelp {
		printHelp(stdout)
		return 0
	}
	if opts.showVersion {
		fmt.Fprintln(stdout, "cat (Go translation)")
		return 0
	}

	proc := &processor{
		w:           bufio.NewWriter(stdout),
		opts:        opts,
		lineNum:     1,
		atLineStart: true,
	}
	defer func() {
		if flushErr := proc.w.Flush(); flushErr != nil && proc.writeErr == nil {
			proc.writeErr = flushErr
		}
	}()

	if len(files) == 0 {
		files = []string{"-"}
	}

	status := 0
	for _, name := range files {
		if err := proc.processFile(name); err != nil {
			fmt.Fprintf(stderr, "cat: %s: %v\n", name, err)
			status = 1
		}
	}
	if proc.writeErr != nil {
		fmt.Fprintf(stderr, "cat: %v\n", proc.writeErr)
		status = 1
	}
	return status
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	parsingOpts := true

	for _, arg := range args {
		if parsingOpts && arg == "--" {
			parsingOpts = false
			continue
		}
		if parsingOpts && strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				opts.showAll = true
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case "--number-nonblank":
				opts.number = true
				opts.numberNonblank = true
			case "--show-ends":
				opts.showEnds = true
			case "--number":
				opts.number = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-tabs":
				opts.showTabs = true
			case "--show-nonprinting":
				opts.showNonprinting = true
			case "--help":
				opts.showHelp = true
			case "--version":
				opts.showVersion = true
			default:
				return opts, nil, fmt.Errorf("cat: unrecognized option %q", arg)
			}
			continue
		}
		if parsingOpts && strings.HasPrefix(arg, "-") && arg != "-" {
			for _, r := range arg[1:] {
				switch r {
				case 'A':
					opts.showAll = true
					opts.showNonprinting = true
					opts.showEnds = true
					opts.showTabs = true
				case 'b':
					opts.number = true
					opts.numberNonblank = true
				case 'e':
					opts.showNonprinting = true
					opts.showEnds = true
				case 'E':
					opts.showEnds = true
				case 'n':
					opts.number = true
				case 's':
					opts.squeezeBlank = true
				case 't':
					opts.showNonprinting = true
					opts.showTabs = true
				case 'T':
					opts.showTabs = true
				case 'u':
					// Ignored, like GNU cat.
				case 'v':
					opts.showNonprinting = true
				case 'h':
					opts.showHelp = true
				case 'V':
					opts.showVersion = true
				default:
					return opts, nil, fmt.Errorf("cat: invalid option -%c", r)
				}
			}
			continue
		}
		files = append(files, arg)
	}

	return opts, files, nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: cat [OPTION]... [FILE]...")
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
	fmt.Fprintln(w, "  -u                       (ignored)")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Fprintln(w, "      --help               display this help and exit")
	fmt.Fprintln(w, "      --version            output version information and exit")
}

func (p *processor) processFile(name string) error {
	var r io.Reader
	if name == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	return p.processReader(r)
}

func (p *processor) processReader(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if procErr := p.processBytes(buf[:n]); procErr != nil {
				return procErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if p.pendingCR {
		if err := p.writeByte('\r'); err != nil {
			return err
		}
		p.pendingCR = false
	}
	return p.writeErr
}

func (p *processor) processBytes(bs []byte) error {
	for _, b := range bs {
		if p.writeErr != nil {
			return p.writeErr
		}

		if p.pendingCR {
			if b == '\n' {
				if err := p.writeString("^M"); err != nil {
					return err
				}
				p.pendingCR = false
				if err := p.handleNewline(); err != nil {
					return err
				}
				continue
			}
			if err := p.writeByte('\r'); err != nil {
				return err
			}
			p.pendingCR = false
		}

		if b == '\n' {
			if err := p.handleNewline(); err != nil {
				return err
			}
			continue
		}

		if p.atLineStart {
			if err := p.maybeWriteLineNumber(); err != nil {
				return err
			}
		}
		p.atLineStart = false
		p.blankRun = 0

		if !p.opts.showNonprinting && p.opts.showEnds && b == '\r' {
			p.pendingCR = true
			continue
		}

		if err := p.writeString(p.renderByte(b)); err != nil {
			return err
		}
	}
	return p.writeErr
}

func (p *processor) handleNewline() error {
	blankLine := p.atLineStart && !p.pendingCR
	if blankLine {
		if p.opts.squeezeBlank && p.blankRun >= 1 {
			p.blankRun = 2
			return nil
		}
		if err := p.maybeWriteBlankLineNumber(); err != nil {
			return err
		}
		if p.opts.showEnds {
			if err := p.writeByte('$'); err != nil {
				return err
			}
		}
		if err := p.writeByte('\n'); err != nil {
			return err
		}
		p.blankRun = 1
		p.atLineStart = true
		return nil
	}

	if p.opts.showEnds {
		if err := p.writeByte('$'); err != nil {
			return err
		}
	}
	if err := p.writeByte('\n'); err != nil {
		return err
	}
	p.blankRun = 0
	p.atLineStart = true
	return nil
}

func (p *processor) maybeWriteLineNumber() error {
	if !p.opts.number {
		return nil
	}
	return p.writeLineNumber()
}

func (p *processor) maybeWriteBlankLineNumber() error {
	if !p.opts.number || p.opts.numberNonblank {
		return nil
	}
	return p.writeLineNumber()
}

func (p *processor) writeLineNumber() error {
	if p.writeErr != nil {
		return p.writeErr
	}
	if _, err := fmt.Fprintf(p.w, "%6d\t", p.lineNum); err != nil {
		p.writeErr = err
		return err
	}
	p.lineNum++
	return nil
}

func (p *processor) renderByte(b byte) string {
	if p.opts.showNonprinting {
		if b == '\t' && !p.opts.showTabs {
			return "\t"
		}
		if b == '\n' {
			return "\n"
		}
		if b >= 32 && b < 127 {
			return string([]byte{b})
		}
		if b == 127 {
			return "^?"
		}
		if b >= 128 {
			r := b - 128
			if r >= 32 {
				if r < 127 {
					return "M-" + string([]byte{r})
				}
				return "M-^?"
			}
			return "M-^" + string([]byte{r + 64})
		}
		return "^" + string([]byte{b + 64})
	}

	if b == '\t' && p.opts.showTabs {
		return "^I"
	}
	return string([]byte{b})
}

func (p *processor) writeString(s string) error {
	if p.writeErr != nil {
		return p.writeErr
	}
	if _, err := p.w.WriteString(s); err != nil {
		p.writeErr = err
		return err
	}
	return nil
}

func (p *processor) writeByte(b byte) error {
	if p.writeErr != nil {
		return p.writeErr
	}
	if err := p.w.WriteByte(b); err != nil {
		p.writeErr = err
		return err
	}
	return nil
}
