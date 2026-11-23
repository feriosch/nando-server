package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// maxLinesRead is the most lines of a CSV one messenger_get_file reads: a
// month of a bank's movements in one answer, while a year comes in parts.
const maxLinesRead = 300

// csvAnswerJSON is what messenger_get_file answers about a CSV.
type csvAnswerJSON struct {
	// Encoding is what the file was written in: utf-8, or shift_jis, as
	// Japanese banks write their CSVs. The text is always UTF-8.
	Encoding  string `json:"encoding"`
	LineCount int    `json:"lineCount"`
	FromLine  int    `json:"fromLine"`
	ToLine    int    `json:"toLine"`
	// Text is the lines read, joined by \n.
	Text string `json:"text"`
}

// isPDF tells a PDF from the other files Nando keeps, which are CSVs, by the
// signature a PDF starts with.
func isPDF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF-"))
}

// readCSV answers the lines of the CSV in data that lines asks for, such as
// "301-600", or the first ones when it is empty.
func readCSV(data []byte, lines string) (csvAnswerJSON, error) {
	text, encoding, err := csvText(data)
	if err != nil {
		return csvAnswerJSON{}, err
	}
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	all := strings.Split(text, "\n")
	if text == "" {
		all = nil
	}
	first, last, err := lineRange(lines, len(all))
	if err != nil {
		return csvAnswerJSON{}, err
	}
	answer := csvAnswerJSON{Encoding: encoding, LineCount: len(all), FromLine: first, ToLine: last}
	if len(all) > 0 {
		answer.Text = strings.Join(all[first-1:last], "\n")
	}
	return answer, nil
}

// csvText is data as UTF-8 text, without a BOM, and the encoding it was in:
// UTF-8 when it is valid UTF-8, else Shift_JIS (Windows-31J).
func csvText(data []byte) (text, encoding string, err error) {
	if bytes.IndexByte(data, 0) >= 0 {
		return "", "", fmt.Errorf("the file can't be read: it isn't a CSV or a PDF")
	}
	if utf8.Valid(data) {
		return strings.TrimPrefix(string(data), "\uFEFF"), "utf-8", nil
	}
	decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(data)
	if err != nil || bytes.ContainsRune(decoded, utf8.RuneError) {
		return "", "", fmt.Errorf("the file can't be read: it is neither UTF-8 nor Shift_JIS text")
	}
	return string(decoded), "shift_jis", nil
}

// lineRange is the lines of a CSV of count lines that lines asks for, such as
// "301-600", or the first ones when it is empty: never more than
// maxLinesRead, so a longer range is cut short. An empty CSV has no lines to
// ask for, and answers none.
func lineRange(lines string, count int) (first, last int, err error) {
	lines = strings.TrimSpace(lines)
	first, last = 1, count
	if lines != "" {
		from, to, isRange := strings.Cut(lines, "-")
		if !isRange {
			to = from
		}
		var errFrom, errTo error
		first, errFrom = strconv.Atoi(strings.TrimSpace(from))
		last, errTo = strconv.Atoi(strings.TrimSpace(to))
		if errFrom != nil || errTo != nil {
			return 0, 0, fmt.Errorf("lines %q isn't a line or a range of lines, such as 3 or 301-600", lines)
		}
		if first < 1 || first > last || last > count {
			return 0, 0, fmt.Errorf("lines %q isn't in the CSV, which has %d lines", lines, count)
		}
	}
	if count == 0 {
		return 0, 0, nil
	}
	return first, min(last, first+maxLinesRead-1), nil
}
