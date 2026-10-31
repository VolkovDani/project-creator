// Code generated from schema.graphqls; DO NOT EDIT.
package graph

import graphql "github.com/graph-gophers/graphql-go"

type Project struct {
	ID          graphql.ID
	Name        string
	Goal        string
	Description string
	CreatedAt   string
	UpdatedAt   string
}
type ProjectInput struct {
	Name        string
	Goal        string
	Description string
}
type FieldError struct {
	Path       []string
	Code       string
	MessageKey string
	Params     []*ErrorParam
}
type ErrorParam struct {
	Key   string
	Value int32
}
type CreateProjectPayload struct {
	Project     *Project
	FieldErrors []*FieldError
}
type Requirement struct {
	ID        graphql.ID
	ProjectId graphql.ID
	Text      string
	Version   int32
	CreatedAt string
	UpdatedAt string
}
type CreateRequirementInput struct {
	ProjectId graphql.ID
	Text      string
}
type UpdateRequirementInput struct {
	ID        graphql.ID
	ProjectId graphql.ID
	Version   int32
	Text      string
}
type RequirementPayload struct {
	Requirement *Requirement
	FieldErrors []*FieldError
}
type DeleteRequirementPayload struct {
	Deleted bool
}
type ProjectAnalysisArgs struct {
	ProjectId graphql.ID
}
type RequirementIndexArgs struct {
	ProjectId graphql.ID
	Limit     int32
	Offset    int32
}
type SearchRequirementsArgs struct {
	ProjectId graphql.ID
	Text      string
	Limit     int32
}
type RequirementsArgs struct {
	ProjectId graphql.ID
	Limit     int32
	Offset    int32
}
type ProjectArgs struct {
	ID graphql.ID
}
type ProjectsArgs struct {
	Limit  int32
	Offset int32
}
type RetryProjectAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type StartProjectAnalysisArgs struct {
	ProjectId graphql.ID
}
type IndexRequirementArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
}
type CreateRequirementArgs struct {
	Input CreateRequirementInput
}
type UpdateRequirementArgs struct {
	Input UpdateRequirementInput
}
type DeleteRequirementArgs struct {
	ID        graphql.ID
	ProjectId graphql.ID
	Version   int32
}
type CreateProjectArgs struct {
	Input ProjectInput
}
type RequirementIndex struct {
	RequirementId graphql.ID
	Version       int32
	Status        string
	Profile       string
	ErrorCode     string
}
type RequirementSearchHit struct {
	RequirementId graphql.ID
	ProjectId     graphql.ID
	Text          string
	SourceText    string
	Version       int32
	Start         int32
	End           int32
	Score         float64
}
type AnalysisSource struct {
	ID      graphql.ID
	Version int32
	Text    string
}
type AnalysisFinding struct {
	LeftId       graphql.ID
	RightId      graphql.ID
	LeftVersion  int32
	RightVersion int32
	Verdict      string
	Explanation  string
}
type AnalysisPair struct {
	ID          graphql.ID
	Position    int32
	Status      string
	Left        *AnalysisSource
	Right       *AnalysisSource
	Verdict     string
	Explanation string
	ErrorCode   string
}
type ProjectAnalysis struct {
	Stage                 string
	TotalPairs            *int32
	ProcessedPairs        int32
	ActivePairId          graphql.ID
	Pairs                 []*AnalysisPair
	ReasonCode            string
	MissingRequirementIds []graphql.ID
	ID                    graphql.ID
	ProjectId             graphql.ID
	Status                string
	Outcome               string
	ErrorCode             string
	CreatedAt             string
	CompletedAt           string
	Stale                 bool
	GeneratorProfile      string
	EmbeddingProfile      string
	Sources               []*AnalysisSource
	Findings              []*AnalysisFinding
}
