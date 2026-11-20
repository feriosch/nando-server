package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// pdfTimeout bounds each poppler program Nando runs on a PDF, so a PDF that
// makes one hang can't hold the run's Tool call.
const pdfTimeout = 30 * time.Second

// errUnreadablePDF is a PDF poppler can't read, such as a broken one.
var errUnreadablePDF = errors.New("the PDF can't be read")

// pdf is a PDF on disk, for poppler's programs (ADR 0008), which run apart
// from Nando so that a PDF that crashes one can't crash the server.
type pdf struct {
	path string
}

// openPDF writes data to a temporary file. close removes it.
func openPDF(data []byte) (pdf, error) {
	f, err := os.CreateTemp("", "nando-*.pdf")
	if err != nil {
		return pdf{}, err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return pdf{}, err
	}
	return pdf{path: f.Name()}, nil
}

func (p pdf) close() {
	_ = os.Remove(p.path)
}

// run runs a poppler program on the PDF with args before its path and after
// it, and returns what it printed.
func (p pdf) run(ctx context.Context, program string, args []string, after ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, append(append(args, p.path), after...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s isn't installed: the server needs poppler-utils", program)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("the PDF took too long to read: try fewer pages at a time, with pages")
		}
		return nil, errUnreadablePDF
	}
	return out.Bytes(), nil
}

// pagesLine is the line of pdfinfo's output that counts the pages.
var pagesLine = regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`)

// pageCount is how many pages the PDF has.
func (p pdf) pageCount(ctx context.Context) (int, error) {
	out, err := p.run(ctx, "pdfinfo", nil)
	if err != nil {
		return 0, err
	}
	match := pagesLine.FindSubmatch(out)
	if match == nil {
		return 0, errUnreadablePDF
	}
	return strconv.Atoi(string(match[1]))
}

// text is the text of pages first to last, one string per page, each without
// the spaces and line breaks around it.
func (p pdf) text(ctx context.Context, first, last int) ([]string, error) {
	out, err := p.run(ctx, "pdftotext", []string{"-q", "-enc", "UTF-8", "-f", strconv.Itoa(first), "-l", strconv.Itoa(last)}, "-")
	if err != nil {
		return nil, err
	}
	// pdftotext ends every page with a form feed.
	pages := strings.Split(string(out), "\f")
	texts := make([]string, last-first+1)
	for i := range texts {
		if i < len(pages) {
			texts[i] = strings.TrimSpace(pages[i])
		}
	}
	return texts, nil
}

// maxPageSide is the longest side, in pixels, of a page drawn as an image:
// as for photos, enough to read it, at about 1,500 tokens.
const maxPageSide = 1280

// images draws pages first to last as JPEGs.
func (p pdf) images(ctx context.Context, first, last int) ([][]byte, error) {
	dir, err := os.MkdirTemp("", "nando-pages-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if _, err := p.run(ctx, "pdftoppm", []string{"-q", "-jpeg", "-scale-to", strconv.Itoa(maxPageSide),
		"-f", strconv.Itoa(first), "-l", strconv.Itoa(last)}, filepath.Join(dir, "page")); err != nil {
		return nil, err
	}
	// pdftoppm names each page page-<n>.jpg, with n zero-padded to the
	// width of the last page's number, so their names sort in page order.
	names, err := filepath.Glob(filepath.Join(dir, "page-*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	images := make([][]byte, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		images = append(images, data)
	}
	return images, nil
}
