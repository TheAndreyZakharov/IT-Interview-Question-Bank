package bank

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func BuildIndex(repo string, language Language) (*Index, []Change, error) {
	return buildIndex(repo, language, false)
}

// BuildReindexIndex reads source questions for a full renumbering migration.
// Existing IDs are deliberately not treated as unique here: newly appended
// questions can temporarily carry copied IDs until Reindex assigns the final
// sequence. All normal validation paths still use BuildIndex.
func BuildReindexIndex(repo string, language Language) (*Index, []Change, error) {
	return buildIndex(repo, language, true)
}

func buildIndex(repo string, language Language, allowDuplicateIDs bool) (*Index, []Change, error) {
	root := filepath.Join(repo, language.QuestionsDir())
	files, err := naturalTopicFiles(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s question directory: %w", language, err)
	}
	index := &Index{
		Language:      language,
		ByID:          make(map[string]*Question),
		BySectionText: make(map[string][]*Question),
		ByText:        make(map[string][]*Question),
		ByLocation:    make(map[string]*Question),
		QuestionFiles: files,
	}
	var changes []Change
	maxNumber := 0
	seenIDs := make(map[string]*Question)

	for _, file := range files {
		questions, err := scanQuestionFile(file, language)
		if err != nil {
			return nil, nil, err
		}
		for _, question := range questions {
			if question.ID != "" {
				prefix := language.IDPrefix() + "-"
				if !strings.HasPrefix(question.ID, prefix) {
					return nil, nil, fmt.Errorf("%s:%d: invalid ID %q: expected prefix %s", file, question.Line, question.ID, prefix)
				}
				number, conversionErr := strconv.Atoi(strings.TrimPrefix(question.ID, prefix))
				if conversionErr != nil || number < 1 {
					return nil, nil, fmt.Errorf("%s:%d: invalid ID %q: expected %s followed by a positive number", file, question.Line, question.ID, prefix)
				}
				if number > maxNumber {
					maxNumber = number
				}
				if previous, exists := seenIDs[question.ID]; exists && !allowDuplicateIDs {
					return nil, nil, fmt.Errorf("duplicate ID %s at %s:%d and %s:%d", question.ID, previous.File, previous.Line, question.File, question.Line)
				}
				seenIDs[question.ID] = question
			}
			index.Questions = append(index.Questions, question)
		}
	}

	for _, question := range index.Questions {
		if question.ID == "" {
			maxNumber++
			question.ID = formatID(language, maxNumber)
			changes = append(changes, Change{File: question.File, Line: question.Line, Kind: "question-id", Text: question.ID})
		}
		index.ByID[question.ID] = question
		key := sectionKey(question.Section, question.Text)
		index.BySectionText[key] = append(index.BySectionText[key], question)
		textKey := canonicalText(question.Text)
		index.ByText[textKey] = append(index.ByText[textKey], question)
		index.ByLocation[locationKey(question.File, question.Line)] = question
	}
	index.NextIDNumber = maxNumber + 1
	return index, changes, nil
}

func scanQuestionFile(file string, language Language) ([]*Question, error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	defer handle.Close()

	reader := newLineReader(handle)
	section := ""
	var questions []*Question
	for {
		line, readErr := reader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", file, readErr)
		}
		if code := headingCode(line.Text); code != "" {
			section = code
		}
		text, id, _, ok, parseErr := parseSourceQuestion(line.Text, language)
		if parseErr != nil {
			return nil, fmt.Errorf("%s:%d: %w", file, line.Number, parseErr)
		}
		if !ok {
			continue
		}
		questions = append(questions, &Question{ID: id, Text: text, Section: section, File: file, Line: line.Number, HasID: id != ""})
	}
	return questions, nil
}

func writeQuestionFiles(index *Index, stage bool) ([]StagedFile, []Change, error) {
	var staged []StagedFile
	var changes []Change
	for _, file := range index.QuestionFiles {
		fileChanges, stagedFile, err := transformQuestionFile(file, index, stage)
		if err != nil {
			cleanupStaged(staged)
			return nil, nil, err
		}
		changes = append(changes, fileChanges...)
		if stagedFile.Temp != "" {
			staged = append(staged, stagedFile)
		}
	}
	return staged, changes, nil
}
