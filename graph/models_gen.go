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
	ArchivedAt *string
	ID         graphql.ID
	ProjectId  graphql.ID
	Text       string
	Version    int32
	CreatedAt  string
	UpdatedAt  string
}
type CreateRequirementInput struct {
	ProjectId graphql.ID
	Text      string
}
type UpdateRequirementInput struct {
	ID           graphql.ID
	ProjectId    graphql.ID
	Version      int32
	Text         string
	ChangeReason *string
}
type RequirementTextRevision struct {
	Version      int32
	Text         string
	PreviousText *string
	ChangeReason *string
	Author       *string
	CreatedAt    string
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
type ClassificationAlternative struct {
	SemanticType       string
	RepresentationType string
	Score              *float64
}
type RequirementClassification struct {
	ProjectId          graphql.ID
	Digest             string
	Profile            string
	QuestionVersion    string
	ErrorCode          string
	Reason             string
	RequirementId      graphql.ID
	TextVersion        int32
	Revision           int32
	Status             string
	SemanticType       string
	RepresentationType string
	Source             string
	RulesVersion       string
	Model              string
	CreatedAt          string
	Alternatives       []*ClassificationAlternative
}
type ConfirmClassificationInput struct {
	ProjectId          graphql.ID
	RequirementId      graphql.ID
	TextVersion        int32
	Revision           int32
	SemanticType       string
	RepresentationType string
}
type ClassificationListItem struct {
	ProjectId          graphql.ID
	RequirementId      graphql.ID
	Text               string
	TextVersion        int32
	Revision           int32
	Status             string
	SemanticType       string
	RepresentationType string
	ErrorCode          string
}
type ClassificationProgress struct {
	ProjectId    graphql.ID
	Configured   bool
	Profile      string
	Total        int32
	Unclassified int32
	Queued       int32
	Classifying  int32
	Suggested    int32
	NeedsReview  int32
	Confirmed    int32
	Failed       int32
	Stale        int32
}
type ClassificationGroup struct {
	ProjectId          graphql.ID
	SemanticType       string
	RepresentationType string
	Count              int32
}
type ClassificationQueuePayload struct {
	Queued      int32
	FieldErrors []*FieldError
}
type ClassificationPayload struct {
	Classification *RequirementClassification
	FieldErrors    []*FieldError
}
type GlossaryTermsPage struct {
	Items    []*GlossaryTerm
	PageInfo *PageInfo
}
type GlossaryMatchesPage struct {
	Items    []*GlossaryCandidate
	PageInfo *PageInfo
}
type GlossaryMentionPage struct {
	Items    []*Requirement
	PageInfo *PageInfo
}
type ExplainClassificationInput struct {
	ProjectId          graphql.ID
	RequirementId      graphql.ID
	TextVersion        int32
	Revision           int32
	SuggestionRevision int32
	SemanticType       string
	RepresentationType string
	Locale             string
}
type ClassificationExplanationCriterion struct {
	Name     string
	Status   string
	Evidence string
	Note     string
}
type ClassificationExplanationAssessment struct {
	Verdict  string
	Reason   string
	Criteria []*ClassificationExplanationCriterion
}
type ClassificationExplanationResult struct {
	Meaning     *ClassificationExplanationAssessment
	Form        *ClassificationExplanationAssessment
	Questions   []string
	Wording     string
	Limitations string
}
type ClassificationExplanationPayload struct {
	Status      string
	Result      *ClassificationExplanationResult
	FieldErrors []*FieldError
}
type ExternalProposalSource struct {
	RequirementId graphql.ID
	Version       int32
	Status        string
}
type ExternalProposalSourceInput struct {
	RequirementId graphql.ID
	Version       int32
}
type ExternalProposalGlossarySource struct {
	TermId  graphql.ID
	Version int32
	Status  string
}
type ExternalProposalGlossarySourceInput struct {
	TermId  graphql.ID
	Version int32
}
type ExternalProposalGlossaryInput struct {
	Sources []ExternalProposalGlossarySourceInput
}
type ExternalProposal struct {
	GlossarySources []*ExternalProposalGlossarySource
	ID              graphql.ID
	Analysis        string
	Limitations     string
	Client          string
	Model           string
	Sharing         string
	Status          string
	CreatedAt       string
	Sources         []*ExternalProposalSource
}
type SaveExternalProposalInput struct {
	Glossary    *ExternalProposalGlossaryInput
	ProjectId   graphql.ID
	RequestId   string
	Analysis    string
	Limitations string
	Client      string
	Model       string
	Sharing     string
	Sources     []ExternalProposalSourceInput
}
type ExternalProposalPayload struct {
	Proposal    *ExternalProposal
	FieldErrors []*FieldError
}
type ExternalProposalPage struct {
	Items    []*ExternalProposal
	PageInfo *PageInfo
}
type BuildProcessGraphInput struct {
	ProjectId graphql.ID
	TermId    *graphql.ID
	Keyword   *string
}
type ProcessGraphBatchInput struct {
	ProjectId graphql.ID
	RunId     graphql.ID
	Revision  int32
}
type ProcessGraphNode struct {
	ID    graphql.ID
	Kind  string
	Label string
}
type ProcessGraphEvidence struct {
	RequirementId graphql.ID
	Version       int32
	Start         int32
	End           int32
	Quote         string
}
type ProcessGraphEdge struct {
	From     graphql.ID
	To       graphql.ID
	Kind     string
	Label    string
	Evidence []*ProcessGraphEvidence
}
type ProcessGraphDraft struct {
	Nodes       []*ProcessGraphNode
	Edges       []*ProcessGraphEdge
	SourceCount int32
	Partial     bool
}
type ProcessGraphPayload struct {
	Status      string
	Draft       *ProcessGraphDraft
	FieldErrors []*FieldError
}
type ProcessGraphRun struct {
	ID        graphql.ID
	Topic     string
	Revision  int32
	Completed int32
	Total     int32
	Failed    int32
}
type ProcessGraphRunPayload struct {
	Status      string
	Run         *ProcessGraphRun
	Draft       *ProcessGraphDraft
	FieldErrors []*FieldError
}
type PairGlossaryHint struct {
	TermId     *graphql.ID
	Version    *int32
	Spelling   string
	Name       *string
	Definition *string
	Context    *string
	Ambiguous  bool
}
type SimilarRequirementPair struct {
	GlossaryContext []*PairGlossaryHint
	Left            *AnalysisSource
	Right           *AnalysisSource
	SimilarityScore float64
}
type SimilarRequirementPairPage struct {
	Items               []*SimilarRequirementPair
	PageInfo            *PageInfo
	Status              string
	EmbeddingProfile    string
	MissingRequirements int32
}
type LatestProcessGraphRunArgs struct {
	ProjectId graphql.ID
}
type SimilarRequirementPairsArgs struct {
	ProjectId graphql.ID
	Threshold float64
	First     *int32
	After     *string
}
type ExternalProposalsArgs struct {
	ProjectId graphql.ID
	First     *int32
	After     *string
}
type ClassificationExplanationSuggestionRevisionArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
}
type RequirementRevisionHistoryArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
	First         int32
	BeforeVersion *int32
}
type ClassificationProgressArgs struct {
	ProjectId graphql.ID
}
type ClassificationListArgs struct {
	ProjectId          graphql.ID
	SemanticType       *string
	RepresentationType *string
	Status             *string
	Limit              int32
	Offset             int32
}
type ClassificationListByIdsArgs struct {
	ProjectId      graphql.ID
	RequirementIds []graphql.ID
}
type ClassificationGroupsArgs struct {
	ProjectId graphql.ID
}
type GlossaryTermsByIdsArgs struct {
	ProjectId graphql.ID
	Ids       []graphql.ID
}
type GlossaryMentionsArgs struct {
	ProjectId graphql.ID
	TermId    graphql.ID
	First     *int32
	After     *string
}
type GlossaryTermsConnectionArgs struct {
	ProjectId graphql.ID
	Search    string
	First     *int32
	After     *string
}
type GlossaryCandidatesConnectionArgs struct {
	ProjectId graphql.ID
	First     *int32
	After     *string
}
type RequirementClassificationArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
}
type RequirementClassificationHistoryArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
	Limit         int32
	Offset        int32
}
type ClassifiedRequirementsArgs struct {
	ProjectId          graphql.ID
	SemanticType       *string
	RepresentationType *string
	Limit              int32
	Offset             int32
}
type GlossaryCandidatesArgs struct {
	ProjectId graphql.ID
	Limit     int32
	Offset    int32
}
type GlossaryTermsArgs struct {
	ProjectId graphql.ID
	Search    string
	Limit     int32
	Offset    int32
}
type RequirementGlossaryArgs struct {
	ProjectId     graphql.ID
	AnalysisId    *graphql.ID
	RequirementId graphql.ID
	Version       int32
}
type FailedRequirementSummaryCountArgs struct {
	ProjectId graphql.ID
}
type RequirementSummariesByIdsArgs struct {
	ProjectId      graphql.ID
	RequirementIds []graphql.ID
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
	Archived  *bool
	First     *int32
	After     *string
	Match     *string
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
type BuildProcessGraphArgs struct {
	Input BuildProcessGraphInput
}
type StartProcessGraphArgs struct {
	Input BuildProcessGraphInput
}
type StartBackgroundProcessGraphArgs struct {
	Input BuildProcessGraphInput
}
type ProcessGraphBatchArgs struct {
	Input ProcessGraphBatchInput
}
type SaveExternalProposalArgs struct {
	Input SaveExternalProposalInput
}
type ExplainRequirementClassificationArgs struct {
	Input ExplainClassificationInput
}
type ScheduleAnalysisDeletionArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type DeleteMarkedAnalysisArgs struct {
	ProjectId  graphql.ID
	AnalysisId graphql.ID
}
type BackfillClassificationsArgs struct {
	ProjectId graphql.ID
}
type RetryFailedClassificationsArgs struct {
	ProjectId graphql.ID
}
type RetryRequirementClassificationArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
	TextVersion   int32
	Reanalyze     bool
}
type SuggestGlossaryDefinitionArgs struct {
	Input SuggestGlossaryDefinitionInput
}
type ConfirmRequirementClassificationArgs struct {
	Input ConfirmClassificationInput
}
type SaveGlossaryTermArgs struct {
	Input GlossaryInput
}
type DeleteGlossaryTermArgs struct {
	ProjectId graphql.ID
	ID        graphql.ID
	Version   int32
}
type RetryFailedRequirementSummariesArgs struct {
	ProjectId graphql.ID
}
type RetryRequirementSummaryArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
}
type CancelRequirementSummaryArgs struct {
	ProjectId     graphql.ID
	RequirementId graphql.ID
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
type ArchiveRequirementArgs struct {
	ID        graphql.ID
	ProjectId graphql.ID
	Version   int32
}
type RestoreRequirementArgs struct {
	ID         graphql.ID
	ProjectId  graphql.ID
	Version    int32
	ArchivedAt string
}
type DeleteRequirementArgs struct {
	ID         graphql.ID
	ProjectId  graphql.ID
	Version    int32
	ArchivedAt *string
	Confirmed  *bool
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
	GlossaryContext    []*PairGlossaryHint
	GlossaryStale      bool
	ExplanationMatches []*GlossaryMatch
	DetailsVersion     string
	IsExactDuplicate   bool
	LayaErrorCode      string
	LayaStatus         string
	Preclassification  *string
	LayaCheckedAt      *string
	SelectionReasons   []string
	SimilarityScore    *float64
	ResultOrigin       string
	ReusedFrom         graphql.ID
	CheckedAt          string
	GeneratorProfile   string
	ID                 graphql.ID
	Position           int32
	Status             string
	Left               *AnalysisSource
	Right              *AnalysisSource
	Verdict            string
	Explanation        string
	ErrorCode          string
}
type ProjectAnalysis struct {
	GlossaryRevision      string
	DeletionScheduledFor  *string
	ClassifierProfile     string
	LayaPendingPairs      int32
	LayaRunningPairs      int32
	LayaCompletedPairs    int32
	LayaFailedPairs       int32
	LayaSkippedPairs      int32
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
	GlossaryRevision      string
	DeletionScheduledFor  *string
	ClassifierProfile     string
	LayaPendingPairs      int32
	LayaRunningPairs      int32
	LayaCompletedPairs    int32
	LayaFailedPairs       int32
	LayaSkippedPairs      int32
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
type RequirementSummary struct {
	GlossaryMatches []*GlossaryMatch
	ByteLength      int32
	LayaEligible    bool
	RequirementId   graphql.ID
	ProjectId       graphql.ID
	Version         int32
	Status          string
	Text            string
	Profile         string
	Provenance      string
	CreatedAt       string
	ErrorCode       string
}
type GlossaryVariant struct {
	Text string
	Kind string
}
type GlossaryVariantInput struct {
	Text string
	Kind string
}
type SuggestGlossaryDefinitionInput struct {
	ProjectId     graphql.ID
	SourceId      graphql.ID
	SourceVersion int32
	Start         int32
	End           int32
	Name          string
}
type GlossaryDefinitionSuggestion struct {
	Definition  *string
	Status      string
	FieldErrors []*FieldError
}
type GlossaryInput struct {
	ProjectId     graphql.ID
	ID            *graphql.ID
	Version       *int32
	Name          string
	Definition    string
	Context       string
	Variants      []GlossaryVariantInput
	SourceId      *graphql.ID
	SourceVersion *int32
}
type GlossaryTerm struct {
	ID            graphql.ID
	Name          string
	Definition    string
	Context       string
	Variants      []*GlossaryVariant
	Version       int32
	SourceId      graphql.ID
	SourceVersion int32
	CreatedAt     string
	UpdatedAt     string
}
type GlossaryPayload struct {
	Term        *GlossaryTerm
	FieldErrors []*FieldError
}
type DeleteGlossaryPayload struct {
	Deleted bool
}
type GlossaryMatch struct {
	Start int32
	End   int32
	Text  string
	Terms []*GlossaryTerm
}
type GlossaryCandidatePage struct {
	Items   []*GlossaryCandidate
	HasMore bool
}
type GlossaryCandidate struct {
	SourceId        graphql.ID
	SourceVersion   int32
	Name            string
	Abbreviation    string
	Before          string
	After           string
	BeforeTruncated bool
	AfterTruncated  bool
	Start           int32
	End             int32
}
type RequirementGlossary struct {
	Version    int32
	Matches    []*GlossaryMatch
	Candidates []*GlossaryCandidate
}
