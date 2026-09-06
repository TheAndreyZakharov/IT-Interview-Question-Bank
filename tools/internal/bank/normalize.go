package bank

import (
	"fmt"
	"os"
	"path/filepath"
)

type NormalizationReport struct {
	Language          Language
	Questions         int
	ReindexedIDs      int
	NewQuestionIDs    int
	Answers           int
	NewAnswerIDs      int
	OutlineIDsUpdated int
	RemovedQuoteLines int
	RepairedHeadings  int
	ChangedFiles      int
	Changes           []Change
}

// Reindex assigns IDs in the current source-question order and propagates the
// new IDs to every answer heading. It is intentionally separate from
// Normalize because changing stable IDs is a deliberate migration.
func Reindex(repo string, language Language, write bool) (NormalizationReport, error) {
	index, _, err := BuildReindexIndex(repo, language)
	if err != nil {
		return NormalizationReport{}, err
	}

	for number, question := range index.Questions {
		question.ID = formatID(language, number+1)
	}
	index.ByID = make(map[string]*Question, len(index.Questions))
	for _, question := range index.Questions {
		index.ByID[question.ID] = question
	}

	report := NormalizationReport{
		Language:     language,
		Questions:    len(index.Questions),
		ReindexedIDs: len(index.Questions),
	}

	staged, questionChanges, err := writeQuestionFiles(index, write)
	if err != nil {
		cleanupStaged(staged)
		return NormalizationReport{}, err
	}
	report.Changes = append(report.Changes, questionChanges...)

	answerRoot := filepath.Join(repo, language.AnswersDir())
	if _, statErr := os.Stat(answerRoot); statErr != nil {
		if os.IsNotExist(statErr) {
			report.ChangedFiles = countChangedFiles(report.Changes)
			if write {
				if err := commitStaged(staged); err != nil {
					cleanupStaged(staged)
					return NormalizationReport{}, err
				}
			} else {
				cleanupStaged(staged)
			}
			return report, nil
		}
		cleanupStaged(staged)
		return NormalizationReport{}, fmt.Errorf("stat %s: %w", answerRoot, statErr)
	}

	answerFiles, err := markdownFiles(answerRoot)
	if err != nil {
		cleanupStaged(staged)
		return NormalizationReport{}, fmt.Errorf("find answer files in %s: %w", answerRoot, err)
	}
	seenAnswerIDs := make(map[string]Answer)
	for _, file := range answerFiles {
		answerChanges, stagedFile, answers, quoteLinesRemoved, processErr := processAnswerFile(file, language, index, write)
		if processErr != nil {
			cleanupStaged(staged)
			return NormalizationReport{}, processErr
		}
		if stagedFile.Temp != "" {
			staged = append(staged, stagedFile)
		}
		for _, answer := range answers {
			if previous, exists := seenAnswerIDs[answer.ID]; exists {
				cleanupStaged(staged)
				return NormalizationReport{}, fmt.Errorf("duplicate answer for %s at %s:%d and %s:%d", answer.ID, previous.File, previous.Line, answer.File, answer.Line)
			}
			seenAnswerIDs[answer.ID] = answer
		}
		report.OutlineIDsUpdated += countChanges(answerChanges, "outline-id")
		report.Answers += len(answers)
		report.RemovedQuoteLines += quoteLinesRemoved
		report.RepairedHeadings += countChanges(answerChanges, "answer-id") + countChanges(answerChanges, "answer-heading") + countChanges(answerChanges, "answer-id-mismatch")
		report.Changes = append(report.Changes, answerChanges...)
	}

	report.ChangedFiles = countChangedFiles(report.Changes)
	if write {
		if err := commitStaged(staged); err != nil {
			cleanupStaged(staged)
			return NormalizationReport{}, err
		}
	} else {
		cleanupStaged(staged)
	}
	return report, nil
}

func Normalize(repo string, language Language, write bool) (NormalizationReport, error) {
	index, questionPlan, err := BuildIndex(repo, language)
	if err != nil {
		return NormalizationReport{}, err
	}
	report := NormalizationReport{
		Language:       language,
		Questions:      len(index.Questions),
		NewQuestionIDs: countChanges(questionPlan, "question-id"),
		Changes:        append([]Change(nil), questionPlan...),
	}

	staged, _, err := writeQuestionFiles(index, write)
	if err != nil {
		cleanupStaged(staged)
		return NormalizationReport{}, err
	}
	answerRoot := filepath.Join(repo, language.AnswersDir())
	if _, statErr := os.Stat(answerRoot); statErr != nil {
		if os.IsNotExist(statErr) {
			seenFiles := make(map[string]struct{})
			for _, change := range report.Changes {
				seenFiles[change.File] = struct{}{}
			}
			report.ChangedFiles = len(seenFiles)
			if write {
				if err := commitStaged(staged); err != nil {
					cleanupStaged(staged)
					return NormalizationReport{}, err
				}
			} else {
				cleanupStaged(staged)
			}
			return report, nil
		}
		cleanupStaged(staged)
		return NormalizationReport{}, fmt.Errorf("stat %s: %w", answerRoot, statErr)
	}
	answerFiles, err := markdownFiles(answerRoot)
	if err != nil {
		cleanupStaged(staged)
		return NormalizationReport{}, fmt.Errorf("find answer files in %s: %w", answerRoot, err)
	}
	seenAnswerIDs := make(map[string]Answer)
	for _, file := range answerFiles {
		answerChanges, stagedFile, answers, quoteLinesRemoved, processErr := processAnswerFile(file, language, index, write)
		if processErr != nil {
			cleanupStaged(staged)
			return NormalizationReport{}, processErr
		}
		if stagedFile.Temp != "" {
			staged = append(staged, stagedFile)
		}
		for _, answer := range answers {
			if previous, exists := seenAnswerIDs[answer.ID]; exists {
				cleanupStaged(staged)
				return NormalizationReport{}, fmt.Errorf("duplicate answer for %s at %s:%d and %s:%d", answer.ID, previous.File, previous.Line, answer.File, answer.Line)
			}
			seenAnswerIDs[answer.ID] = answer
		}
		report.OutlineIDsUpdated += countChanges(answerChanges, "outline-id")
		report.Answers += len(answers)
		report.NewAnswerIDs += countAnswerChanges(answerChanges)
		report.RemovedQuoteLines += quoteLinesRemoved
		report.RepairedHeadings += countChanges(answerChanges, "answer-id") + countChanges(answerChanges, "answer-heading") + countChanges(answerChanges, "answer-id-mismatch")
		report.Changes = append(report.Changes, answerChanges...)
	}

	report.ChangedFiles = countChangedFiles(report.Changes)
	if write {
		if err := commitStaged(staged); err != nil {
			cleanupStaged(staged)
			return NormalizationReport{}, err
		}
	} else {
		cleanupStaged(staged)
	}
	return report, nil
}

func countChangedFiles(changes []Change) int {
	seenFiles := make(map[string]struct{})
	for _, change := range changes {
		seenFiles[change.File] = struct{}{}
	}
	return len(seenFiles)
}

func countChanges(changes []Change, kind string) int {
	count := 0
	for _, change := range changes {
		if change.Kind == kind {
			count++
		}
	}
	return count
}

func countAnswerChanges(changes []Change) int {
	count := 0
	for _, change := range changes {
		if change.Kind == "answer-id" {
			count++
		}
	}
	return count
}
