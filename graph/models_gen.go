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
type PageInfo struct {
	EndCursor   *string
	HasNextPage bool
}
type ProjectEdge struct {
	Cursor string
	Node   *Project
}
type RequirementEdge struct {
	Cursor string
	Node   *Requirement
}
type AnalysisPairEdge struct {
	Cursor string
	Node   *AnalysisPair
}
type ProjectConnection struct {
	Edges      []*ProjectEdge
	PageInfo   *PageInfo
	TotalCount int32
}
type RequirementConnection struct {
	Edges      []*RequirementEdge
	PageInfo   *PageInfo
	TotalCount int32
}
type AnalysisPairConnection struct {
	Edges      []*AnalysisPairEdge
	PageInfo   *PageInfo
	TotalCount int32
}
type RequirementIndexProgressArgs struct {
	ProjectId graphql.ID
}
type ProjectAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId *graphql.ID
}
type ProjectAnalysisSummaryArgs struct {
	ProjectId  graphql.ID
	AnalysisId *graphql.ID
}
type AnalysisRunMetadataArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type AnalysisPairArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
	PairId     graphql.ID
}
type AnalysisPairsArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
	Limit      int32
	Offset     int32
	Status     string
	Verdict    string
}
type AnalysisPairsConnectionArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
	First      *int32
	After      *string
	Status     *string
	Verdict    *string
}
type AnalysisHistoryArgs struct {
	ProjectId graphql.ID
	Limit     int32
	Offset    int32
}
type RequirementIndexArgs struct {
	ProjectId graphql.ID
	Limit     int32
	Offset    int32
}
type RequirementIndexByIdsArgs struct {
	ProjectId      graphql.ID
	RequirementIds []graphql.ID
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
type RequirementsConnectionArgs struct {
	ProjectId graphql.ID
	First     *int32
	After     *string
}
type ProjectArgs struct {
	ID graphql.ID
}
type ProjectsArgs struct {
	Limit  int32
	Offset int32
}
type ProjectsConnectionArgs struct {
	First *int32
	After *string
}
type ReplaceProjectAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
	Mode       *string
	Threshold  *float64
	Strategy   string
}
type CancelProjectAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type RetryProjectAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type StartProjectAnalysisArgs struct {
	ProjectId graphql.ID
	Mode      *string
	Threshold *float64
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
	ProjectId     graphql.ID
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
	SelectionReasons []string
	SimilarityScore  *float64
	ResultOrigin     string
	ReusedFrom       graphql.ID
	CheckedAt        string
	GeneratorProfile string
	ID               graphql.ID
	Position         int32
	Status           string
	Left             *AnalysisSource
	Right            *AnalysisSource
	Verdict          string
	Explanation      string
	ErrorCode        string
}
type ProjectAnalysis struct {
	Revision              string
	Mode                  string
	SimilarityThreshold   float64
	SelectionProfile      string
	PossiblePairs         *int32
	NotSelectedPairs      *int32
	DuplicatePairs        int32
	ReusedPairs           int32
	NewPairs              int32
	CompletedPairs        int32
	PendingPairs          int32
	RunningPairs          int32
	FailedPairs           int32
	CollisionPairs        int32
	CompatiblePairs       int32
	InsufficientPairs     int32
	GeneratorProvenance   string
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
type RequirementIndexProgress struct {
	ProjectId  graphql.ID
	Total      int32
	Queued     int32
	Running    int32
	Ready      int32
	Failed     int32
	Profile    string
	Configured bool
}
type AnalysisRunMetadata struct {
	ID                  graphql.ID
	ProjectId           graphql.ID
	GeneratorProfile    string
	GeneratorProvenance string
	EmbeddingProfile    string
	SelectionProfile    string
}
type ProjectAnalysisSummary struct {
	Revision              string
	Mode                  string
	SimilarityThreshold   float64
	PossiblePairs         *int32
	NotSelectedPairs      *int32
	DuplicatePairs        int32
	ReusedPairs           int32
	NewPairs              int32
	CompletedPairs        int32
	PendingPairs          int32
	RunningPairs          int32
	FailedPairs           int32
	CollisionPairs        int32
	CompatiblePairs       int32
	InsufficientPairs     int32
	Stage                 string
	TotalPairs            *int32
	ProcessedPairs        int32
	ActivePairId          graphql.ID
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
}
