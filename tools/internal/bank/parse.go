package bank

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	headingRE           = regexp.MustCompile(`^#{1,4}\s+(\d+(?:\.\d+){0,3})\b`)
	topicNumberRE       = regexp.MustCompile(`^##\s+(\d+)\b`)
	questionLineRE      = regexp.MustCompile(`^-\s+(.*)$`)
	answerHeadRE        = regexp.MustCompile(`^-{1,2}\s+\*\*(.*?)\*\*(?:\s+(.*))?$`)
	answerFileSectionRE = regexp.MustCompile(`^#{2,4}\s+(\d+(?:\.\d+){0,3})\b`)
	idSuffixRE          = regexp.MustCompile(`\s+\[id:\s*([A-Z]{2}-[0-9]{6})\]\s*$`)
	idMarkerRE          = regexp.MustCompile(`(?i)\[id:`)
	answerMarkRE        = regexp.MustCompile(`(?i)^\*{0,2}(ответ|answer):?\*{0,2}\s*$`)
)

type rawLine struct {
	Text   string
	Ending string
	Number int
}

type lineReader struct {
	r      *bufio.Reader
	number int
	done   bool
}

func newLineReader(file *os.File) *lineReader {
	return &lineReader{r: bufio.NewReaderSize(file, 256*1024)}
}

func (r *lineReader) Next() (rawLine, error) {
	if r.done {
		return rawLine{}, io.EOF
	}
	r.number++
	raw, err := r.r.ReadString('\n')
	if len(raw) == 0 && err == io.EOF {
		r.done = true
		return rawLine{}, io.EOF
	}
	ending := ""
	text := raw
	if strings.HasSuffix(text, "\n") {
		ending = "\n"
		text = strings.TrimSuffix(text, "\n")
		if strings.HasSuffix(text, "\r") {
			ending = "\r\n"
			text = strings.TrimSuffix(text, "\r")
		}
	}
	if err == io.EOF {
		r.done = true
	} else if err != nil {
		return rawLine{}, err
	}
	return rawLine{Text: text, Ending: ending, Number: r.number}, nil
}

func writeRawLine(w io.Writer, text, ending string) error {
	if _, err := io.WriteString(w, text); err != nil {
		return err
	}
	_, err := io.WriteString(w, ending)
	return err
}

func headingCode(line string) string {
	m := headingRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return ""
	}
	return m[1]
}

func parseID(text string, language Language) (base, id string, hasID bool, err error) {
	if !idMarkerRE.MatchString(text) {
		return strings.TrimSpace(text), "", false, nil
	}
	m := idSuffixRE.FindStringSubmatch(text)
	if len(m) == 0 {
		return "", "", false, fmt.Errorf("malformed ID marker in %q", text)
	}
	if !strings.HasPrefix(m[1], language.IDPrefix()+"-") {
		return "", "", false, fmt.Errorf("ID %q belongs to another language, expected %s-*", m[1], language)
	}
	number, convErr := strconv.Atoi(strings.TrimPrefix(m[1], language.IDPrefix()+"-"))
	if convErr != nil || number < 1 {
		return "", "", false, fmt.Errorf("invalid ID %q", m[1])
	}
	return strings.TrimSpace(strings.TrimSuffix(text, m[0])), m[1], true, nil
}

func formatID(language Language, number int) string {
	return fmt.Sprintf("%s-%06d", language.IDPrefix(), number)
}

func canonicalText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "`", "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

func sectionKey(section, text string) string {
	return section + "\x00" + canonicalText(text)
}

func locationKey(file string, line int) string {
	return fmt.Sprintf("%s\x00%d", file, line)
}

func parseSourceQuestion(line string, language Language) (text, id, prefix string, ok bool, err error) {
	m := questionLineRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return "", "", "", false, nil
	}
	base, parsedID, hasID, parseErr := parseID(m[1], language)
	if parseErr != nil {
		return "", "", "", false, parseErr
	}
	if strings.TrimSpace(base) == "" {
		return "", "", "", false, errors.New("empty question text")
	}
	if !hasID {
		parsedID = ""
	}
	return base, parsedID, "", true, nil
}

func parseAnswerQuestion(line string, language Language) (text, id string, hasID, ok bool, err error) {
	m := answerHeadRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return "", "", false, false, nil
	}
	answerText := m[1]
	if strings.TrimSpace(m[2]) != "" {
		answerText += " " + strings.TrimSpace(m[2])
	}
	base, parsedID, parsed, parseErr := parseID(answerText, language)
	if parseErr != nil {
		return "", "", false, false, parseErr
	}
	if strings.TrimSpace(base) == "" {
		return "", "", false, false, errors.New("empty answer question heading")
	}
	return base, parsedID, parsed, true, nil
}

func naturalTopicFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	type named struct {
		path string
		name string
		num  int
	}
	files := make([]named, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		num := 0
		if m := topicNumberRE.FindStringSubmatch(entry.Name()); len(m) == 2 {
			num, _ = strconv.Atoi(m[1])
		}
		files = append(files, named{path: filepath.Join(root, entry.Name()), name: entry.Name(), num: num})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].num != files[j].num {
			return files[i].num < files[j].num
		}
		return files[i].name < files[j].name
	})
	result := make([]string, len(files))
	for i := range files {
		result[i] = files[i].path
	}
	return result, nil
}

func markdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func answerFileSection(file string) string {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	m := answerFileSectionRE.FindStringSubmatch(name)
	if len(m) == 0 {
		return ""
	}
	return m[1]
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}
	previous := make([]int, len(rb)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, ca := range ra {
		current := make([]int, len(rb)+1)
		current[0] = i + 1
		for j, cb := range rb {
			cost := 0
			if ca != cb {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(rb)]
}

func min(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
