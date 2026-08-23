package bank

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func CollectStats(repo string, language Language) (Stats, error) {
	index, questionChanges, err := BuildIndex(repo, language)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{
		Language:        language,
		Questions:       len(index.Questions),
		QuestionsWithID: len(index.Questions) - countChanges(questionChanges, "question-id"),
	}
	answered := make(map[string]struct{})
	answerRoot := filepath.Join(repo, language.AnswersDir())
	if _, statErr := os.Stat(answerRoot); statErr != nil {
		if os.IsNotExist(statErr) {
			stats.AnswerRootMissing = true
			stats.Unanswered = append(stats.Unanswered, index.Questions...)
			return stats, nil
		}
		return Stats{}, fmt.Errorf("stat %s: %w", answerRoot, statErr)
	}
	answerFiles, err := markdownFiles(answerRoot)
	if err != nil {
		return Stats{}, err
	}
	seenAnswerIDs := make(map[string]Answer)
	reviewedSections := make(map[string]struct{})
	for _, file := range answerFiles {
		if section := answerFileSection(file); section != "" {
			reviewedSections[section] = struct{}{}
		}
		_, _, answers, _, processErr := processAnswerFile(file, language, index, false)
		if processErr != nil {
			stats.UnmatchedAnswers = append(stats.UnmatchedAnswers, Answer{File: file, Text: processErr.Error()})
			continue
		}
		for _, answer := range answers {
			stats.Answers++
			if answer.HasID {
				stats.AnswersWithID++
				if answer.DeclaredID != answer.ID {
					stats.MismatchedAnswerID = append(stats.MismatchedAnswerID,
						fmt.Sprintf("declares %s but question ID is %s (%s:%d)", answer.DeclaredID, answer.ID, answer.File, answer.Line))
				}
			}
			if previous, exists := seenAnswerIDs[answer.ID]; exists {
				stats.DuplicateAnswerID = append(stats.DuplicateAnswerID, fmt.Sprintf("%s (%s:%d, %s:%d)", answer.ID, previous.File, previous.Line, answer.File, answer.Line))
				continue
			}
			seenAnswerIDs[answer.ID] = answer
			answered[answer.ID] = struct{}{}
		}
	}
	for _, question := range index.Questions {
		if _, exists := answered[question.ID]; exists {
			stats.AnsweredQuestions++
		} else {
			stats.Unanswered = append(stats.Unanswered, question)
		}
		if _, reviewed := reviewedSections[question.Section]; reviewed {
			stats.ReviewedQuestions++
			if _, exists := answered[question.ID]; exists {
				stats.ReviewedAnswered++
			} else {
				stats.ReviewedUnanswered = append(stats.ReviewedUnanswered, question)
			}
		}
	}
	for section := range reviewedSections {
		stats.ReviewedSections = append(stats.ReviewedSections, section)
	}
	sort.SliceStable(stats.ReviewedSections, func(i, j int) bool {
		return sectionLess(stats.ReviewedSections[i], stats.ReviewedSections[j])
	})
	return stats, nil
}

func sectionNumber(section string) []int {
	parts := strings.Split(section, ".")
	numbers := make([]int, len(parts))
	for i, part := range parts {
		numbers[i], _ = strconv.Atoi(part)
	}
	return numbers
}

func sectionLess(left, right string) bool {
	leftNumbers := sectionNumber(left)
	rightNumbers := sectionNumber(right)
	length := len(leftNumbers)
	if len(rightNumbers) < length {
		length = len(rightNumbers)
	}
	for i := 0; i < length; i++ {
		if leftNumbers[i] != rightNumbers[i] {
			return leftNumbers[i] < rightNumbers[i]
		}
	}
	if len(leftNumbers) != len(rightNumbers) {
		return len(leftNumbers) < len(rightNumbers)
	}
	return left < right
}
