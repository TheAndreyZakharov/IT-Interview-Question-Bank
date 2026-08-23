package bank

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func transformQuestionFile(file string, index *Index, stage bool) ([]Change, StagedFile, error) {
	input, err := os.Open(file)
	if err != nil {
		return nil, StagedFile{}, fmt.Errorf("open %s: %w", file, err)
	}
	defer input.Close()

	var output *os.File
	var staged StagedFile
	if stage {
		output, staged, err = createStageFile(file)
		if err != nil {
			return nil, StagedFile{}, err
		}
	}
	if output != nil {
		defer func() {
			_ = output.Close()
		}()
	}

	reader := newLineReader(input)
	var changes []Change
	for {
		line, readErr := reader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if output != nil {
				_ = output.Close()
				_ = os.Remove(staged.Temp)
			}
			return nil, StagedFile{}, fmt.Errorf("read %s: %w", file, readErr)
		}
		newText := line.Text
		if question, ok := index.ByLocation[locationKey(file, line.Number)]; ok {
			_, _, prefix, _, _ := parseSourceQuestion(line.Text, index.Language)
			newText = prefix + "- " + question.Text + " [id: " + question.ID + "]"
			if newText != line.Text {
				changes = append(changes, Change{File: file, Line: line.Number, Kind: "question-id", Text: question.ID})
			}
		}
		if output != nil {
			if err := writeRawLine(output, newText, line.Ending); err != nil {
				_ = output.Close()
				_ = os.Remove(staged.Temp)
				return nil, StagedFile{}, fmt.Errorf("write staged %s: %w", file, err)
			}
		}
	}
	if output != nil {
		if err := output.Close(); err != nil {
			_ = os.Remove(staged.Temp)
			return nil, StagedFile{}, fmt.Errorf("close staged %s: %w", file, err)
		}
		if len(changes) == 0 {
			_ = os.Remove(staged.Temp)
			staged.Temp = ""
		}
	}
	return changes, staged, nil
}

func createStageFile(original string) (*os.File, StagedFile, error) {
	dir := filepath.Dir(original)
	temp, err := os.CreateTemp(dir, ".bank-normalize-*.tmp")
	if err != nil {
		return nil, StagedFile{}, fmt.Errorf("create temporary file for %s: %w", original, err)
	}
	return temp, StagedFile{Original: original, Temp: temp.Name()}, nil
}

func cleanupStaged(staged []StagedFile) {
	for _, file := range staged {
		if file.Temp != "" {
			_ = os.Remove(file.Temp)
		}
	}
}

func commitStaged(staged []StagedFile) error {
	for _, file := range staged {
		if err := os.Rename(file.Temp, file.Original); err != nil {
			return fmt.Errorf("replace %s: %w", file.Original, err)
		}
	}
	return nil
}

func isBlank(line string) bool { return strings.TrimSpace(stripBlockquotePrefix(line)) == "" }

func isAnswerMarker(line string) bool {
	return answerMarkRE.MatchString(strings.TrimSpace(stripBlockquotePrefix(line)))
}

func isFenceDelimiter(line string) bool {
	trimmed := strings.TrimSpace(stripBlockquotePrefix(line))
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

type pendingAnswerQuestion struct {
	line       rawLine
	text       string
	declaredID string
	hasID      bool
	section    string
	buffer     []rawLine
}

func processAnswerFile(file string, language Language, index *Index, stage bool) ([]Change, StagedFile, []Answer, int, error) {
	input, err := os.Open(file)
	if err != nil {
		return nil, StagedFile{}, nil, 0, fmt.Errorf("open %s: %w", file, err)
	}
	defer input.Close()

	var output *os.File
	var staged StagedFile
	if stage {
		output, staged, err = createStageFile(file)
		if err != nil {
			return nil, StagedFile{}, nil, 0, err
		}
	}
	if output != nil {
		defer func() { _ = output.Close() }()
	}
	success := false
	defer func() {
		if !success {
			if output != nil {
				_ = output.Close()
			}
			if staged.Temp != "" {
				_ = os.Remove(staged.Temp)
			}
		}
	}()

	reader := newLineReader(input)
	section := answerFileSection(file)
	inFence := false
	var pending *pendingAnswerQuestion
	var changes []Change
	var answers []Answer
	quoteLinesRemoved := 0
	var seenAnswerIDs = make(map[string]Answer)

	flushPending := func() error {
		if pending == nil {
			return nil
		}
		if err := writeAnswerLine(output, pending.line, pending.line.Text, &quoteLinesRemoved); err != nil {
			return fmt.Errorf("write pending question at %s:%d: %w", file, pending.line.Number, err)
		}
		for _, buffered := range pending.buffer {
			if err := writeAnswerLine(output, buffered, buffered.Text, &quoteLinesRemoved); err != nil {
				return fmt.Errorf("write buffered line at %s:%d: %w", file, buffered.Number, err)
			}
		}
		pending = nil
		return nil
	}

	for {
		line, readErr := reader.Next()
		if readErr == io.EOF {
			if err := flushPending(); err != nil {
				return nil, StagedFile{}, nil, 0, fmt.Errorf("finish %s: %w", file, err)
			}
			break
		}
		if readErr != nil {
			return nil, StagedFile{}, nil, 0, fmt.Errorf("read %s: %w", file, readErr)
		}

		if pending != nil {
			if isBlank(line.Text) {
				pending.buffer = append(pending.buffer, line)
				continue
			}
			if isAnswerMarker(line.Text) {
				question, err := resolveAnswerQuestion(index, pending.section, pending.text)
				if err != nil {
					return nil, StagedFile{}, nil, 0, fmt.Errorf("%s:%d: %w", file, pending.line.Number, err)
				}
				if previous, exists := seenAnswerIDs[question.ID]; exists {
					return nil, StagedFile{}, nil, 0, fmt.Errorf("duplicate answer for %s at %s:%d and %s:%d", question.ID, previous.File, previous.Line, file, pending.line.Number)
				}
				answer := Answer{ID: question.ID, DeclaredID: pending.declaredID, Text: question.Text, Section: pending.section, File: file, Line: pending.line.Number, HasID: pending.hasID}
				seenAnswerIDs[question.ID] = answer
				answers = append(answers, answer)
				newQuestionLine := "- **" + question.Text + "** [id: " + question.ID + "]"
				if newQuestionLine != stripBlockquotePrefix(pending.line.Text) {
					kind := "answer-id"
					if pending.hasID {
						kind = "answer-heading"
						if pending.declaredID != question.ID {
							kind = "answer-id-mismatch"
						}
					}
					changes = append(changes, Change{File: file, Line: pending.line.Number, Kind: kind, Text: question.ID})
				}
				if err := writeAnswerLine(output, pending.line, newQuestionLine, &quoteLinesRemoved); err != nil {
					return nil, StagedFile{}, nil, 0, err
				}
				for _, buffered := range pending.buffer {
					if err := writeAnswerLine(output, buffered, buffered.Text, &quoteLinesRemoved); err != nil {
						return nil, StagedFile{}, nil, 0, err
					}
				}
				if err := writeAnswerLine(output, line, stripBlockquotePrefix(line.Text), &quoteLinesRemoved); err != nil {
					return nil, StagedFile{}, nil, 0, err
				}
				pending = nil
				continue
			}
			if err := flushPending(); err != nil {
				return nil, StagedFile{}, nil, 0, fmt.Errorf("continue %s: %w", file, err)
			}
		}

		if !strings.HasPrefix(strings.TrimLeft(line.Text, " \t"), ">") {
			if !inFence {
				if code := headingCode(line.Text); code != "" {
					section = code
				}
			}
		}
		text, id, hasID, ok, parseErr := parseAnswerQuestion(line.Text, language)
		if parseErr != nil {
			return nil, StagedFile{}, nil, 0, fmt.Errorf("%s:%d: %w", file, line.Number, parseErr)
		}
		if ok {
			pending = &pendingAnswerQuestion{line: line, text: text, declaredID: id, hasID: hasID, section: section}
			continue
		}

		if isFenceDelimiter(line.Text) {
			if err := writeAnswerLine(output, line, stripBlockquotePrefix(line.Text), &quoteLinesRemoved); err != nil {
				return nil, StagedFile{}, nil, 0, fmt.Errorf("write fence at %s:%d: %w", file, line.Number, err)
			}
			inFence = !inFence
			continue
		}
		if inFence {
			if err := writeAnswerLine(output, line, stripBlockquotePrefix(line.Text), &quoteLinesRemoved); err != nil {
				return nil, StagedFile{}, nil, 0, fmt.Errorf("write code line at %s:%d: %w", file, line.Number, err)
			}
			continue
		}

		if err := writeAnswerLine(output, line, stripBlockquotePrefix(line.Text), &quoteLinesRemoved); err != nil {
			return nil, StagedFile{}, nil, 0, fmt.Errorf("write line at %s:%d: %w", file, line.Number, err)
		}
	}

	if quoteLinesRemoved > 0 {
		changes = append(changes, Change{File: file, Kind: "blockquote-prefix", Text: strconv.Itoa(quoteLinesRemoved)})
	}
	if output != nil {
		if err := output.Close(); err != nil {
			_ = os.Remove(staged.Temp)
			return nil, StagedFile{}, nil, 0, fmt.Errorf("close staged %s: %w", file, err)
		}
		if len(changes) == 0 {
			_ = os.Remove(staged.Temp)
			staged.Temp = ""
		}
	}
	success = true
	return changes, staged, answers, quoteLinesRemoved, nil
}

func writeAnswerLine(output *os.File, line rawLine, text string, quoteLinesRemoved *int) error {
	normalized := stripBlockquotePrefix(text)
	if normalized != line.Text && quoteLinesRemoved != nil {
		*quoteLinesRemoved = *quoteLinesRemoved + 1
	}
	if output == nil {
		return nil
	}
	return writeRawLine(output, normalized, line.Ending)
}

func resolveAnswerQuestion(index *Index, section, text string) (*Question, error) {
	hits := index.BySectionText[sectionKey(section, text)]
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return nil, fmt.Errorf("ambiguous answer question %q in section %s", text, section)
	}

	keyText := canonicalText(text)
	globalHits := index.ByText[keyText]
	if len(globalHits) == 1 {
		return globalHits[0], nil
	}
	if len(globalHits) > 1 {
		return nil, fmt.Errorf("ambiguous answer question %q: exact text occurs %d times in the source bank", text, len(globalHits))
	}

	var candidates []*Question
	maxDistance := 2
	if len([]rune(keyText)) > 45 {
		maxDistance = 3
	}
	for _, question := range index.Questions {
		if question.Section != section {
			continue
		}
		if editDistance(keyText, canonicalText(question.Text)) <= maxDistance {
			candidates = append(candidates, question)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return nil, fmt.Errorf("ambiguous near-match for answer question %q in section %s", text, section)
	}
	return nil, fmt.Errorf("answer question not found in source questions: %q (section %s)", text, section)
}
