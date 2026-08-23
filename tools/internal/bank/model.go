package bank

import "fmt"

type Language string

const (
	RU Language = "RU"
	EN Language = "EN"
)

func (l Language) QuestionsDir() string {
	return fmt.Sprintf("%s/Questions_By_Topic_%s", l, l)
}

func (l Language) AnswersDir() string {
	return fmt.Sprintf("%s/Questions_with_AI_Answers_By_Topic_%s", l, l)
}

func (l Language) IDPrefix() string { return string(l) }

type Question struct {
	ID      string
	Text    string
	Section string
	File    string
	Line    int
	HasID   bool
}

type Answer struct {
	ID         string
	DeclaredID string
	Text       string
	Section    string
	File       string
	Line       int
	HasID      bool
}

type Change struct {
	File string
	Line int
	Kind string
	Text string
}

type StagedFile struct {
	Original string
	Temp     string
}

type Index struct {
	Language      Language
	Questions     []*Question
	ByID          map[string]*Question
	BySectionText map[string][]*Question
	ByText        map[string][]*Question
	ByLocation    map[string]*Question
	NextIDNumber  int
	QuestionFiles []string
}

type Stats struct {
	Language           Language
	Questions          int
	QuestionsWithID    int
	Answers            int
	AnswersWithID      int
	AnsweredQuestions  int
	Unanswered         []*Question
	UnmatchedAnswers   []Answer
	DuplicateAnswerID  []string
	MismatchedAnswerID []string
	AnswerRootMissing  bool
	ReviewedSections   []string
	ReviewedQuestions  int
	ReviewedAnswered   int
	ReviewedUnanswered []*Question
}
