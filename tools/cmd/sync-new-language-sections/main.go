package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type languageFiles struct {
	name     string
	topic    string
	complete string
}

var questionID = regexp.MustCompile(`\s+\[id: (?:RU|EN)-\d+\]`)

func main() {
	repo := flag.String("repo", ".", "path to the question-bank repository")
	write := flag.Bool("write", false, "apply the synchronization; without this flag only verify and report")
	flag.Parse()

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fatal(err)
	}

	files := []languageFiles{
		{
			name:     "RU",
			topic:    "RU/Questions_By_Topic_RU/## 4. Языки программирования.md",
			complete: "RU/Complete_Question_Bank_RU.md",
		},
		{
			name:     "EN",
			topic:    "EN/Questions_By_Topic_EN/## 4. Programming Languages.md",
			complete: "EN/Complete_Question_Bank_EN.md",
		},
	}

	mode := "check"
	if *write {
		mode = "write"
	}
	fmt.Printf("Mode: %s\nRepository: %s\n", mode, absRepo)

	for _, language := range files {
		changed, questions, err := syncLanguage(absRepo, language, *write)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", language.name, err))
		}
		fmt.Printf("%s: programming-language section contains %d questions; complete bank %s\n", language.name, questions, changed)
	}
	if !*write {
		fmt.Println("No files were changed. Run again with --write after reviewing the check.")
	}
}

func syncLanguage(repo string, files languageFiles, write bool) (string, int, error) {
	topicPath := filepath.Join(repo, files.topic)
	completePath := filepath.Join(repo, files.complete)
	topic, err := os.ReadFile(topicPath)
	if err != nil {
		return "", 0, err
	}
	section, err := sectionFromTopic(string(topic))
	if err != nil {
		return "", 0, err
	}
	questions := countQuestions(section)
	if questions < 5000 {
		return "", 0, fmt.Errorf("expected at least 5000 questions in section 4, got %d", questions)
	}

	complete, err := os.ReadFile(completePath)
	if err != nil {
		return "", 0, err
	}
	updated, err := replaceCompleteSection(string(complete), section)
	if err != nil {
		return "", 0, err
	}
	if updated == string(complete) {
		return "already synchronized", questions, nil
	}
	if !write {
		return "would be updated", questions, nil
	}
	if err := os.WriteFile(completePath, []byte(updated), 0o644); err != nil {
		return "", 0, err
	}
	return "updated", questions, nil
}

func sectionFromTopic(topic string) (string, error) {
	start := strings.Index(topic, "## 4.")
	if start < 0 {
		return "", fmt.Errorf("section 4 was not found")
	}
	section := questionID.ReplaceAllString(topic[start:], "")
	if !strings.Contains(section, "### 4.20 ") {
		return "", fmt.Errorf("section 4.20 was not found")
	}
	return strings.TrimRight(section, "\n") + "\n\n", nil
}

func replaceCompleteSection(complete, section string) (string, error) {
	start := strings.Index(complete, "## 4.")
	if start < 0 {
		return "", fmt.Errorf("section 4 was not found in the complete bank")
	}
	rest := complete[start:]
	endOffset := strings.Index(rest, "## 5. ")
	if endOffset < 0 {
		return "", fmt.Errorf("section 5 was not found in the complete bank")
	}
	return complete[:start] + section + rest[endOffset:], nil
}

func countQuestions(section string) int {
	count := 0
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "- ") {
			count++
		}
	}
	return count
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}
