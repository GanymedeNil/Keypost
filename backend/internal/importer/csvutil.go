package importer

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
)

// detectDelimiter picks the field separator (comma, semicolon or tab) of a
// CSV file from its first line, ignoring separators inside quotes.
func detectDelimiter(sample []byte) rune {
	if i := bytes.IndexByte(sample, '\n'); i >= 0 {
		sample = sample[:i]
	}
	counts := map[rune]int{}
	inQuotes := false
	for _, b := range sample {
		switch {
		case b == '"':
			inQuotes = !inQuotes
		case inQuotes:
		case b == ',' || b == ';' || b == '\t':
			counts[rune(b)]++
		}
	}
	best, n := ',', 0
	for _, d := range []rune{',', ';', '\t'} {
		if counts[d] > n {
			best, n = d, counts[d]
		}
	}
	return best
}

// newCSVReader returns a tolerant reader (BOM, any common delimiter, ragged
// rows, stray quotes).
func newCSVReader(r io.Reader) *csv.Reader {
	br := skipBOM(r)
	sample, _ := br.Peek(64 << 10)
	cr := csv.NewReader(br)
	cr.Comma = detectDelimiter(sample)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	return cr
}

// readCSV reads every record of a CSV stream.
func readCSV(r io.Reader) ([][]string, error) {
	cr := newCSVReader(r)
	var rows [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			// a malformed line is skipped, anything else ends the file
			if pe, ok := err.(*csv.ParseError); ok && pe.Err != io.EOF {
				continue
			}
			return rows, err
		}
		if blankRecord(rec) {
			continue
		}
		rows = append(rows, rec)
	}
}

func blankRecord(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// normHeader lowercases a header cell and turns separators into underscores.
func normHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
	return strings.NewReplacer(" ", "_", "-", "_", ".", "_").Replace(h)
}
