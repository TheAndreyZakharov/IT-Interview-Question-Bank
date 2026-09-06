// Command create-ru-answer-files is a one-time generator for the unanswered
// Russian part of the answer bank. It creates terminal section files from 3.7
// through 20.12 and never opens or modifies answer files from earlier sections.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	sourceRootRelative = "RU/Questions_By_Topic_RU"
	targetRootRelative = "RU/Questions_with_AI_Answers_By_Topic_RU"
	firstSection       = "3.7"
	lastSection        = "20.12"
)

var (
	headingRE  = regexp.MustCompile(`^(#{2,4})\s+(\d+(?:\.\d+){0,2})\b\s*(.*?)\s*$`)
	questionRE = regexp.MustCompile(`^-\s+.+\s+\[id:\s*RU-[0-9]{6}\]\s*$`)
)

type section struct {
	code      string
	level     int
	heading   string
	questions []string
}

type filePlan struct {
	section string
	path    string
	content string
}

func main() {
	repo := flag.String("repo", ".", "path to the question-bank repository")
	flag.Parse()

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fatal(err)
	}

	plans, err := buildPlans(absRepo)
	if err != nil {
		fatal(err)
	}
	if err := ensureTargetsAreNew(plans); err != nil {
		fatal(err)
	}
	if err := writePlans(plans); err != nil {
		fatal(err)
	}

	first := plans[0].section
	last := plans[len(plans)-1].section
	fmt.Printf("Created %d Russian answer files for sections %s through %s.\n", len(plans), first, last)
}

func buildPlans(repo string) ([]filePlan, error) {
	start, err := parseCode(firstSection)
	if err != nil {
		return nil, err
	}
	end, err := parseCode(lastSection)
	if err != nil {
		return nil, err
	}

	var plans []filePlan
	for topic := 3; topic <= 20; topic++ {
		source, err := sourceTopicFile(filepath.Join(repo, sourceRootRelative), topic)
		if err != nil {
			return nil, err
		}
		sections, err := readSections(source, topic, start)
		if err != nil {
			return nil, err
		}
		filePlans, err := plansForSource(sections, filepath.Join(repo, targetRootRelative), start, end)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		plans = append(plans, filePlans...)
	}
	if len(plans) == 0 {
		return nil, errors.New("no terminal sections found in the configured range")
	}

	sort.Slice(plans, func(i, j int) bool {
		return compareCodes(mustParseCode(plans[i].section), mustParseCode(plans[j].section)) < 0
	})
	return plans, nil
}

func sourceTopicFile(root string, topic int) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, fmt.Sprintf("## %d.*.md", topic)))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected exactly one source file for topic %d in %s, found %d", topic, root, len(matches))
	}
	return matches[0], nil
}

func readSections(path string, topic int, start []int) ([]*section, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sections := make([]*section, 0)
	byCode := make(map[string]*section)
	var current *section
	// Topic 3 shares its source file with already completed sections. Their
	// contents are skipped as raw text: no heading or question before 3.7 is
	// parsed, validated, retained, or used to build output.
	skippingCompletedPart := topic == 3
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if skippingCompletedPart {
			if len(sections) == 0 {
				if !isRootHeading(line, topic) {
					continue
				}
				current = &section{code: strconv.Itoa(topic), level: 2, heading: strings.TrimSpace(line)}
				sections = append(sections, current)
				byCode[current.code] = current
				continue
			}
			if !isExactStartHeading(line, formatCode(start)) {
				continue
			}
			skippingCompletedPart = false
		}
		if match := headingRE.FindStringSubmatch(line); len(match) != 0 {
			level := len(match[1])
			code := match[2]
			parts, parseErr := parseCode(code)
			if parseErr != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, parseErr)
			}
			if parts[0] != topic {
				return nil, fmt.Errorf("line %d: heading %q belongs to another topic", lineNumber, code)
			}
			if _, exists := byCode[code]; exists {
				return nil, fmt.Errorf("line %d: duplicate section %s", lineNumber, code)
			}
			current = &section{code: code, level: level, heading: strings.TrimSpace(line)}
			sections = append(sections, current)
			byCode[code] = current
			continue
		}
		if current != nil && questionRE.MatchString(line) {
			current.questions = append(current.questions, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if skippingCompletedPart {
		return nil, fmt.Errorf("could not find start heading ### %s", formatCode(start))
	}
	if len(sections) == 0 {
		return nil, errors.New("no numbered headings found")
	}
	if sections[0].code != strconv.Itoa(topic) || sections[0].level != 2 {
		return nil, fmt.Errorf("first heading must be ## %d", topic)
	}
	return sections, nil
}

func isRootHeading(line string, topic int) bool {
	prefix := "## " + strconv.Itoa(topic)
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	if len(line) == len(prefix) {
		return true
	}
	next := line[len(prefix)]
	return next == '.' || next == ' ' || next == '\t'
}

func isExactStartHeading(line, code string) bool {
	prefix := "### " + code
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	if len(line) == len(prefix) {
		return true
	}
	next := line[len(prefix)]
	return next == ' ' || next == '\t'
}

func plansForSource(sections []*section, targetRoot string, start, end []int) ([]filePlan, error) {
	byCode := make(map[string]*section, len(sections))
	hasChild := make(map[string]bool, len(sections))
	for _, item := range sections {
		byCode[item.code] = item
		parts := mustParseCode(item.code)
		if len(parts) > 1 {
			parent := formatCode(parts[:len(parts)-1])
			hasChild[parent] = true
		}
	}

	plans := make([]filePlan, 0)
	for _, item := range sections {
		parts := mustParseCode(item.code)
		if compareCodes(parts, start) < 0 || compareCodes(parts, end) > 0 || hasChild[item.code] {
			continue
		}
		if len(item.questions) == 0 {
			return nil, fmt.Errorf("terminal section %s has no questions", item.code)
		}

		headings := make([]string, 0, len(parts))
		directories := make([]string, 0, len(parts)-1)
		for depth := 1; depth <= len(parts); depth++ {
			code := formatCode(parts[:depth])
			ancestor, exists := byCode[code]
			if !exists {
				return nil, fmt.Errorf("section %s is missing parent heading %s", item.code, code)
			}
			headings = append(headings, ancestor.heading)
			if depth < len(parts) {
				directories = append(directories, safePathPart(ancestor.heading))
			}
		}

		filename := safePathPart(item.heading) + ".md"
		path := filepath.Join(append([]string{targetRoot}, append(directories, filename)...)...)
		content := strings.Join(headings, "\n\n") + "\n\n" + strings.Join(item.questions, "\n") + "\n"
		plans = append(plans, filePlan{section: item.code, path: path, content: content})
	}
	return plans, nil
}

func ensureTargetsAreNew(plans []filePlan) error {
	seen := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		if _, duplicate := seen[plan.path]; duplicate {
			return fmt.Errorf("two sections resolve to the same target path: %s", plan.path)
		}
		seen[plan.path] = struct{}{}
		if _, err := os.Lstat(plan.path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file for section %s: %s", plan.section, plan.path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect target for section %s: %w", plan.section, err)
		}
	}
	return nil
}

func writePlans(plans []filePlan) error {
	for _, plan := range plans {
		if err := os.MkdirAll(filepath.Dir(plan.path), 0o755); err != nil {
			return fmt.Errorf("create directory for section %s: %w", plan.section, err)
		}
		file, err := os.OpenFile(plan.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create file for section %s: %w", plan.section, err)
		}
		if _, err := io.WriteString(file, plan.content); err != nil {
			file.Close()
			return fmt.Errorf("write file for section %s: %w", plan.section, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close file for section %s: %w", plan.section, err)
		}
	}
	return nil
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, ":", "-")
	return value
}

func parseCode(code string) ([]int, error) {
	pieces := strings.Split(code, ".")
	if len(pieces) == 0 || len(pieces) > 3 {
		return nil, fmt.Errorf("invalid section code %q", code)
	}
	result := make([]int, len(pieces))
	for i, piece := range pieces {
		value, err := strconv.Atoi(piece)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("invalid section code %q", code)
		}
		result[i] = value
	}
	return result, nil
}

func mustParseCode(code string) []int {
	parts, err := parseCode(code)
	if err != nil {
		panic(err)
	}
	return parts
}

func formatCode(parts []int) string {
	text := make([]string, len(parts))
	for i, part := range parts {
		text[i] = strconv.Itoa(part)
	}
	return strings.Join(text, ".")
}

func compareCodes(left, right []int) int {
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for i := 0; i < length; i++ {
		leftPart, rightPart := 0, 0
		if i < len(left) {
			leftPart = left[i]
		}
		if i < len(right) {
			rightPart = right[i]
		}
		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}
