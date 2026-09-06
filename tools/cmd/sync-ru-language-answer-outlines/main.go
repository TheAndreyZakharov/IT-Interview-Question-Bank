// Command sync-ru-language-answer-outlines synchronizes Russian answer outlines
// for every terminal section of one topic. It only refreshes an outline when
// that file contains no completed answer heading, so it cannot replace
// handwritten answers.
package main

import (
	"bufio"
	"flag"
	"fmt"
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

func main() {
	repo := flag.String("repo", ".", "path to the question-bank repository")
	topic := flag.Int("topic", 4, "top-level topic number to synchronize")
	write := flag.Bool("write", false, "create and move outlines; without this flag only check")
	flag.Parse()
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fatal(err)
	}
	if *topic < 1 {
		fatal(fmt.Errorf("topic must be positive"))
	}
	source, err := sourceTopicFile(filepath.Join(absRepo, sourceRootRelative), *topic)
	if err != nil {
		fatal(err)
	}
	sections, err := readSections(source, *topic)
	if err != nil {
		fatal(err)
	}
	plans, err := buildPlans(sections, *topic)
	if err != nil {
		fatal(err)
	}
	mode := "check"
	if *write {
		mode = "write"
	}
	fmt.Printf("Mode: %s\nRepository: %s\n", mode, absRepo)
	changed, err := apply(plans, sections, filepath.Join(absRepo, targetRootRelative), *write)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Answer outlines: %d to create or refresh\n", changed)
	if !*write {
		fmt.Println("No files were changed. Run again with --write after reviewing the check.")
	}
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

func readSections(path string, topic int) ([]*section, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var sections []*section
	var current *section
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for s.Scan() {
		line := strings.TrimSuffix(s.Text(), "\r")
		if m := headingRE.FindStringSubmatch(line); len(m) != 0 {
			if !strings.HasPrefix(m[2], strconv.Itoa(topic)) || (len(m[2]) > len(strconv.Itoa(topic)) && m[2][len(strconv.Itoa(topic))] != '.') {
				return nil, fmt.Errorf("heading %s does not belong to topic %d", m[2], topic)
			}
			current = &section{code: m[2], level: len(m[1]), heading: strings.TrimSpace(line)}
			sections = append(sections, current)
			continue
		}
		if current != nil && questionRE.MatchString(line) {
			current.questions = append(current.questions, line)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return sections, nil
}

func buildPlans(sections []*section, topic int) ([]*section, error) {
	byCode := map[string]*section{}
	hasChild := map[string]bool{}
	for _, item := range sections {
		byCode[item.code] = item
		parts := strings.Split(item.code, ".")
		if len(parts) > 1 {
			hasChild[strings.Join(parts[:len(parts)-1], ".")] = true
		}
	}
	var plans []*section
	for _, item := range sections {
		if item.code == strconv.Itoa(topic) || hasChild[item.code] {
			continue
		}
		if len(item.questions) == 0 {
			return nil, fmt.Errorf("terminal section %s has no ID-bearing questions", item.code)
		}
		parts := strings.Split(item.code, ".")
		for d := 1; d < len(parts); d++ {
			if _, ok := byCode[strings.Join(parts[:d], ".")]; !ok {
				return nil, fmt.Errorf("section %s is missing a parent", item.code)
			}
		}
		plans = append(plans, item)
	}
	sort.Slice(plans, func(i, j int) bool { return compare(plans[i].code, plans[j].code) < 0 })
	if len(plans) == 0 {
		return nil, fmt.Errorf("topic %d has no terminal sections", topic)
	}
	return plans, nil
}

func apply(plans, all []*section, root string, write bool) (int, error) {
	changed := 0
	for _, plan := range plans {
		path := planPath(plan, all, root)
		expected := outline(plan, all)
		existing, err := os.ReadFile(path)
		if err == nil {
			if string(existing) == expected {
				continue
			}
			if strings.Contains(string(existing), "- **") {
				return 0, fmt.Errorf("refusing to refresh outline containing answers: %s", path)
			}
			changed++
			if write {
				if err := os.WriteFile(path, []byte(expected), 0o644); err != nil {
					return 0, err
				}
			}
			continue
		}
		if !os.IsNotExist(err) {
			return 0, err
		}
		changed++
		if write {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return 0, err
			}
			if err := os.WriteFile(path, []byte(expected), 0o644); err != nil {
				return 0, err
			}
		}
	}
	return changed, nil
}

func planPath(item *section, all []*section, root string) string {
	byCode := map[string]*section{}
	for _, p := range all {
		byCode[p.code] = p
	}
	parts := strings.Split(item.code, ".")
	dirs := []string{root}
	for d := 1; d < len(parts); d++ {
		code := strings.Join(parts[:d], ".")
		if parent, ok := byCode[code]; ok {
			dirs = append(dirs, safe(parent.heading))
		}
	}
	return filepath.Join(append(dirs, safe(item.heading)+".md")...)
}

func outline(item *section, all []*section) string {
	byCode := map[string]*section{}
	for _, p := range all {
		byCode[p.code] = p
	}
	parts := strings.Split(item.code, ".")
	root, ok := byCode[parts[0]]
	if !ok {
		return ""
	}
	headings := []string{root.heading}
	for d := 2; d <= len(parts); d++ {
		if parent, ok := byCode[strings.Join(parts[:d], ".")]; ok {
			headings = append(headings, parent.heading)
		}
	}
	return strings.Join(headings, "\n\n") + "\n\n" + strings.Join(item.questions, "\n") + "\n"
}

func inRange(code, first, last string) bool {
	return compare(code, first) >= 0 && compare(code, last) <= 0
}
func compare(a, b string) int {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		ai, _ := strconv.Atoi(ap[i])
		bi, _ := strconv.Atoi(bp[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	if len(ap) < len(bp) {
		return -1
	}
	if len(ap) > len(bp) {
		return 1
	}
	return 0
}
func safe(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "/", "-"), "\\", "-"), ":", "-")
	return s
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "ERROR:", err); os.Exit(1) }
