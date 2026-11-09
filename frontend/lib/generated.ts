import { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
};

export type AnalysisFinding = {
  __typename?: 'AnalysisFinding';
  explanation: Scalars['String']['output'];
  leftId: Scalars['ID']['output'];
  leftVersion: Scalars['Int']['output'];
  rightId: Scalars['ID']['output'];
  rightVersion: Scalars['Int']['output'];
  verdict: Scalars['String']['output'];
};

export type AnalysisPair = {
  __typename?: 'AnalysisPair';
  checkedAt: Scalars['String']['output'];
  errorCode: Scalars['String']['output'];
  explanation: Scalars['String']['output'];
  generatorProfile: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  layaCheckedAt?: Maybe<Scalars['String']['output']>;
  layaErrorCode: Scalars['String']['output'];
  layaStatus: Scalars['String']['output'];
  left: AnalysisSource;
  position: Scalars['Int']['output'];
  preclassification?: Maybe<Scalars['String']['output']>;
  resultOrigin: Scalars['String']['output'];
  reusedFrom: Scalars['ID']['output'];
  right: AnalysisSource;
  selectionReasons: Array<Scalars['String']['output']>;
  similarityScore?: Maybe<Scalars['Float']['output']>;
  status: Scalars['String']['output'];
  verdict: Scalars['String']['output'];
};

export type AnalysisPairConnection = {
  __typename?: 'AnalysisPairConnection';
  edges: Array<AnalysisPairEdge>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type AnalysisPairEdge = {
  __typename?: 'AnalysisPairEdge';
  cursor: Scalars['String']['output'];
  node: AnalysisPair;
};

export type AnalysisRunMetadata = {
  __typename?: 'AnalysisRunMetadata';
  embeddingProfile: Scalars['String']['output'];
  generatorProfile: Scalars['String']['output'];
  generatorProvenance: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  projectId: Scalars['ID']['output'];
  selectionProfile: Scalars['String']['output'];
};

export type AnalysisSource = {
  __typename?: 'AnalysisSource';
  id: Scalars['ID']['output'];
  text: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type CreateProjectPayload = {
  __typename?: 'CreateProjectPayload';
  fieldErrors: Array<FieldError>;
  project?: Maybe<Project>;
};

export type CreateRequirementInput = {
  projectId: Scalars['ID']['input'];
  text: Scalars['String']['input'];
};

export type DeleteRequirementPayload = {
  __typename?: 'DeleteRequirementPayload';
  deleted: Scalars['Boolean']['output'];
};

export type ErrorParam = {
  __typename?: 'ErrorParam';
  key: Scalars['String']['output'];
  value: Scalars['Int']['output'];
};

export type FieldError = {
  __typename?: 'FieldError';
  code: Scalars['String']['output'];
  messageKey: Scalars['String']['output'];
  params: Array<ErrorParam>;
  path: Array<Scalars['String']['output']>;
};

export type Mutation = {
  __typename?: 'Mutation';
  cancelProjectAnalysis: ProjectAnalysis;
  cancelRequirementSummary: RequirementSummary;
  createProject: CreateProjectPayload;
  createRequirement: RequirementPayload;
  deleteRequirement: DeleteRequirementPayload;
  indexRequirement: RequirementIndex;
  replaceProjectAnalysis: ProjectAnalysis;
  retryProjectAnalysis: ProjectAnalysis;
  retryRequirementSummary: RequirementSummary;
  startProjectAnalysis: ProjectAnalysis;
  updateRequirement: RequirementPayload;
};


export type MutationCancelProjectAnalysisArgs = {
  analysisId: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
};


export type MutationCancelRequirementSummaryArgs = {
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
};


export type MutationCreateProjectArgs = {
  input: ProjectInput;
};


export type MutationCreateRequirementArgs = {
  input: CreateRequirementInput;
};


export type MutationDeleteRequirementArgs = {
  id: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
  version: Scalars['Int']['input'];
};


export type MutationIndexRequirementArgs = {
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
};


export type MutationReplaceProjectAnalysisArgs = {
  analysisId: Scalars['ID']['input'];
  mode?: InputMaybe<Scalars['String']['input']>;
  projectId: Scalars['ID']['input'];
  strategy: Scalars['String']['input'];
  threshold?: InputMaybe<Scalars['Float']['input']>;
};


export type MutationRetryProjectAnalysisArgs = {
  analysisId: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
};


export type MutationRetryRequirementSummaryArgs = {
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
};


export type MutationStartProjectAnalysisArgs = {
  mode?: InputMaybe<Scalars['String']['input']>;
  projectId: Scalars['ID']['input'];
  threshold?: InputMaybe<Scalars['Float']['input']>;
};


export type MutationUpdateRequirementArgs = {
  input: UpdateRequirementInput;
};

export type PageInfo = {
  __typename?: 'PageInfo';
  endCursor?: Maybe<Scalars['String']['output']>;
  hasNextPage: Scalars['Boolean']['output'];
};

export type Project = {
  __typename?: 'Project';
  createdAt: Scalars['String']['output'];
  description: Scalars['String']['output'];
  goal: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['String']['output'];
};

export type ProjectAnalysis = {
  __typename?: 'ProjectAnalysis';
  activePairId: Scalars['ID']['output'];
  classifierProfile: Scalars['String']['output'];
  collisionPairs: Scalars['Int']['output'];
  compatiblePairs: Scalars['Int']['output'];
  completedAt: Scalars['String']['output'];
  completedPairs: Scalars['Int']['output'];
  createdAt: Scalars['String']['output'];
  duplicatePairs: Scalars['Int']['output'];
  embeddingProfile: Scalars['String']['output'];
  errorCode: Scalars['String']['output'];
  failedPairs: Scalars['Int']['output'];
  findings: Array<AnalysisFinding>;
  generatorProfile: Scalars['String']['output'];
  generatorProvenance: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  insufficientPairs: Scalars['Int']['output'];
  layaCompletedPairs: Scalars['Int']['output'];
  layaFailedPairs: Scalars['Int']['output'];
  layaPendingPairs: Scalars['Int']['output'];
  layaRunningPairs: Scalars['Int']['output'];
  layaSkippedPairs: Scalars['Int']['output'];
  missingRequirementIds: Array<Scalars['ID']['output']>;
  mode: Scalars['String']['output'];
  newPairs: Scalars['Int']['output'];
  notSelectedPairs?: Maybe<Scalars['Int']['output']>;
  outcome: Scalars['String']['output'];
  pairs: Array<AnalysisPair>;
  pendingPairs: Scalars['Int']['output'];
  possiblePairs?: Maybe<Scalars['Int']['output']>;
  processedPairs: Scalars['Int']['output'];
  projectId: Scalars['ID']['output'];
  reasonCode: Scalars['String']['output'];
  reusedPairs: Scalars['Int']['output'];
  revision: Scalars['String']['output'];
  runningPairs: Scalars['Int']['output'];
  selectionProfile: Scalars['String']['output'];
  similarityThreshold: Scalars['Float']['output'];
  sources: Array<AnalysisSource>;
  stage: Scalars['String']['output'];
  stale: Scalars['Boolean']['output'];
  status: Scalars['String']['output'];
  totalPairs?: Maybe<Scalars['Int']['output']>;
};

export type ProjectAnalysisSummary = {
  __typename?: 'ProjectAnalysisSummary';
  activePairId: Scalars['ID']['output'];
  classifierProfile: Scalars['String']['output'];
  collisionPairs: Scalars['Int']['output'];
  compatiblePairs: Scalars['Int']['output'];
  completedAt: Scalars['String']['output'];
  completedPairs: Scalars['Int']['output'];
  createdAt: Scalars['String']['output'];
  duplicatePairs: Scalars['Int']['output'];
  errorCode: Scalars['String']['output'];
  failedPairs: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  insufficientPairs: Scalars['Int']['output'];
  layaCompletedPairs: Scalars['Int']['output'];
  layaFailedPairs: Scalars['Int']['output'];
  layaPendingPairs: Scalars['Int']['output'];
  layaRunningPairs: Scalars['Int']['output'];
  layaSkippedPairs: Scalars['Int']['output'];
  missingRequirementIds: Array<Scalars['ID']['output']>;
  mode: Scalars['String']['output'];
  newPairs: Scalars['Int']['output'];
  notSelectedPairs?: Maybe<Scalars['Int']['output']>;
  outcome: Scalars['String']['output'];
  pendingPairs: Scalars['Int']['output'];
  possiblePairs?: Maybe<Scalars['Int']['output']>;
  processedPairs: Scalars['Int']['output'];
  projectId: Scalars['ID']['output'];
  reasonCode: Scalars['String']['output'];
  reusedPairs: Scalars['Int']['output'];
  revision: Scalars['String']['output'];
  runningPairs: Scalars['Int']['output'];
  similarityThreshold: Scalars['Float']['output'];
  stage: Scalars['String']['output'];
  stale: Scalars['Boolean']['output'];
  status: Scalars['String']['output'];
  totalPairs?: Maybe<Scalars['Int']['output']>;
};

export type ProjectConnection = {
  __typename?: 'ProjectConnection';
  edges: Array<ProjectEdge>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type ProjectEdge = {
  __typename?: 'ProjectEdge';
  cursor: Scalars['String']['output'];
  node: Project;
};

export type ProjectInput = {
  description: Scalars['String']['input'];
  goal: Scalars['String']['input'];
  name: Scalars['String']['input'];
};

export type Query = {
  __typename?: 'Query';
  analysisHistory: Array<ProjectAnalysis>;
  analysisPair?: Maybe<AnalysisPair>;
  analysisPairs: Array<AnalysisPair>;
  analysisPairsConnection: AnalysisPairConnection;
  analysisRunMetadata?: Maybe<AnalysisRunMetadata>;
  project?: Maybe<Project>;
  projectAnalysis?: Maybe<ProjectAnalysis>;
  projectAnalysisSummary?: Maybe<ProjectAnalysisSummary>;
  projects: Array<Project>;
  projectsConnection: ProjectConnection;
  requirementIndex: Array<RequirementIndex>;
  requirementIndexByIds: Array<RequirementIndex>;
  requirementIndexProgress: RequirementIndexProgress;
  requirementSummariesByIds: Array<RequirementSummary>;
  requirements: Array<Requirement>;
  requirementsConnection: RequirementConnection;
  searchRequirements: Array<RequirementSearchHit>;
};


export type QueryAnalysisHistoryArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryAnalysisPairArgs = {
  analysisId: Scalars['ID']['input'];
  pairId: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryAnalysisPairsArgs = {
  analysisId: Scalars['ID']['input'];
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
  status: Scalars['String']['input'];
  verdict: Scalars['String']['input'];
};


export type QueryAnalysisPairsConnectionArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  analysisId: Scalars['ID']['input'];
  first?: InputMaybe<Scalars['Int']['input']>;
  projectId: Scalars['ID']['input'];
  status?: InputMaybe<Scalars['String']['input']>;
  verdict?: InputMaybe<Scalars['String']['input']>;
};


export type QueryAnalysisRunMetadataArgs = {
  analysisId: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryProjectArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProjectAnalysisArgs = {
  analysisId?: InputMaybe<Scalars['ID']['input']>;
  projectId: Scalars['ID']['input'];
};


export type QueryProjectAnalysisSummaryArgs = {
  analysisId?: InputMaybe<Scalars['ID']['input']>;
  projectId: Scalars['ID']['input'];
};


export type QueryProjectsArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
};


export type QueryProjectsConnectionArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  first?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryRequirementIndexArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryRequirementIndexByIdsArgs = {
  projectId: Scalars['ID']['input'];
  requirementIds: Array<Scalars['ID']['input']>;
};


export type QueryRequirementIndexProgressArgs = {
  projectId: Scalars['ID']['input'];
};


export type QueryRequirementSummariesByIdsArgs = {
  projectId: Scalars['ID']['input'];
  requirementIds: Array<Scalars['ID']['input']>;
};


export type QueryRequirementsArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryRequirementsConnectionArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  first?: InputMaybe<Scalars['Int']['input']>;
  projectId: Scalars['ID']['input'];
};


export type QuerySearchRequirementsArgs = {
  limit: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
  text: Scalars['String']['input'];
};

export type Requirement = {
  __typename?: 'Requirement';
  createdAt: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  projectId: Scalars['ID']['output'];
  text: Scalars['String']['output'];
  updatedAt: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type RequirementConnection = {
  __typename?: 'RequirementConnection';
  edges: Array<RequirementEdge>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type RequirementEdge = {
  __typename?: 'RequirementEdge';
  cursor: Scalars['String']['output'];
  node: Requirement;
};

export type RequirementIndex = {
  __typename?: 'RequirementIndex';
  errorCode: Scalars['String']['output'];
  profile: Scalars['String']['output'];
  projectId: Scalars['ID']['output'];
  requirementId: Scalars['ID']['output'];
  status: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type RequirementIndexProgress = {
  __typename?: 'RequirementIndexProgress';
  configured: Scalars['Boolean']['output'];
  failed: Scalars['Int']['output'];
  profile: Scalars['String']['output'];
  projectId: Scalars['ID']['output'];
  queued: Scalars['Int']['output'];
  ready: Scalars['Int']['output'];
  running: Scalars['Int']['output'];
  total: Scalars['Int']['output'];
};

export type RequirementPayload = {
  __typename?: 'RequirementPayload';
  fieldErrors: Array<FieldError>;
  requirement?: Maybe<Requirement>;
};

export type RequirementSearchHit = {
  __typename?: 'RequirementSearchHit';
  end: Scalars['Int']['output'];
  projectId: Scalars['ID']['output'];
  requirementId: Scalars['ID']['output'];
  score: Scalars['Float']['output'];
  sourceText: Scalars['String']['output'];
  start: Scalars['Int']['output'];
  text: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type RequirementSummary = {
  __typename?: 'RequirementSummary';
  byteLength: Scalars['Int']['output'];
  createdAt: Scalars['String']['output'];
  errorCode: Scalars['String']['output'];
  layaEligible: Scalars['Boolean']['output'];
  profile: Scalars['String']['output'];
  projectId: Scalars['ID']['output'];
  provenance: Scalars['String']['output'];
  requirementId: Scalars['ID']['output'];
  status: Scalars['String']['output'];
  text: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type UpdateRequirementInput = {
  id: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
  text: Scalars['String']['input'];
  version: Scalars['Int']['input'];
};

export type CompactAnalysisSnapshotFragment = { __typename?: 'ProjectAnalysisSummary', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string };

export type AnalysisSnapshotFragment = { __typename?: 'ProjectAnalysis', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string };

export type ProjectAnalysisStatusQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type ProjectAnalysisStatusQuery = { __typename?: 'Query', projectAnalysis?: { __typename?: 'ProjectAnalysis', mode: string, similarityThreshold: number, selectionProfile: string, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, generatorProvenance: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, generatorProfile: string, embeddingProfile: string, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string, sources: Array<{ __typename?: 'AnalysisSource', id: string, version: number, text: string }>, findings: Array<{ __typename?: 'AnalysisFinding', leftId: string, leftVersion: number, rightId: string, rightVersion: number, verdict: string, explanation: string }>, pairs: Array<{ __typename?: 'AnalysisPair', selectionReasons: Array<string>, similarityScore?: number | null, resultOrigin: string, reusedFrom: string, checkedAt: string, generatorProfile: string, layaStatus: string, layaErrorCode: string, preclassification?: string | null, layaCheckedAt?: string | null, id: string, position: number, status: string, verdict: string, explanation: string, errorCode: string, left: { __typename?: 'AnalysisSource', id: string, version: number, text: string }, right: { __typename?: 'AnalysisSource', id: string, version: number, text: string } }> } | null };

export type StartProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  mode?: InputMaybe<Scalars['String']['input']>;
  threshold?: InputMaybe<Scalars['Float']['input']>;
}>;


export type StartProjectAnalysisMutation = { __typename?: 'Mutation', startProjectAnalysis: { __typename?: 'ProjectAnalysis', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string } };

export type ReplaceProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
  mode?: InputMaybe<Scalars['String']['input']>;
  threshold?: InputMaybe<Scalars['Float']['input']>;
  strategy: Scalars['String']['input'];
}>;


export type ReplaceProjectAnalysisMutation = { __typename?: 'Mutation', replaceProjectAnalysis: { __typename?: 'ProjectAnalysis', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string } };

export type RetryProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
}>;


export type RetryProjectAnalysisMutation = { __typename?: 'Mutation', retryProjectAnalysis: { __typename?: 'ProjectAnalysis', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string } };

export type AnalysisPairPageQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  status: Scalars['String']['input'];
  verdict: Scalars['String']['input'];
}>;


export type AnalysisPairPageQuery = { __typename?: 'Query', scope?: { __typename: 'ProjectAnalysisSummary', projectId: string } | null, analysisPairs: Array<{ __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, errorCode: string, selectionReasons: Array<string>, similarityScore?: number | null, resultOrigin: string, reusedFrom: string, checkedAt: string, generatorProfile: string, layaStatus: string, layaErrorCode: string, preclassification?: string | null, layaCheckedAt?: string | null, left: { __typename?: 'AnalysisSource', id: string, version: number }, right: { __typename?: 'AnalysisSource', id: string, version: number } }> };

export type AnalysisHistoryQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
}>;


export type AnalysisHistoryQuery = { __typename?: 'Query', analysisHistory: Array<{ __typename?: 'ProjectAnalysis', mode: string, similarityThreshold: number, selectionProfile: string, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, id: string, projectId: string, createdAt: string, status: string, generatorProfile: string, stale: boolean }> };

export type AnalysisPairConnectionQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
  first: Scalars['Int']['input'];
  after?: InputMaybe<Scalars['String']['input']>;
  status: Scalars['String']['input'];
  verdict: Scalars['String']['input'];
}>;


export type AnalysisPairConnectionQuery = { __typename?: 'Query', scope?: { __typename: 'ProjectAnalysisSummary', projectId: string } | null, analysisPairsConnection: { __typename?: 'AnalysisPairConnection', totalCount: number, edges: Array<{ __typename?: 'AnalysisPairEdge', cursor: string, node: { __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, errorCode: string, selectionReasons: Array<string>, similarityScore?: number | null, resultOrigin: string, reusedFrom: string, checkedAt: string, generatorProfile: string, layaStatus: string, layaErrorCode: string, preclassification?: string | null, layaCheckedAt?: string | null, left: { __typename?: 'AnalysisSource', id: string, version: number }, right: { __typename?: 'AnalysisSource', id: string, version: number } } }>, pageInfo: { __typename?: 'PageInfo', endCursor?: string | null, hasNextPage: boolean } } };

export type ProjectAnalysisSummaryQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type ProjectAnalysisSummaryQuery = { __typename?: 'Query', projectAnalysis?: { __typename?: 'ProjectAnalysisSummary', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string } | null };

export type CancelProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
}>;


export type CancelProjectAnalysisMutation = { __typename?: 'Mutation', cancelProjectAnalysis: { __typename?: 'ProjectAnalysis', revision: string, mode: string, similarityThreshold: number, possiblePairs?: number | null, notSelectedPairs?: number | null, duplicatePairs: number, reusedPairs: number, newPairs: number, completedPairs: number, pendingPairs: number, runningPairs: number, failedPairs: number, collisionPairs: number, compatiblePairs: number, insufficientPairs: number, layaPendingPairs: number, layaRunningPairs: number, layaCompletedPairs: number, layaFailedPairs: number, layaSkippedPairs: number, classifierProfile: string, id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string } };

export type AnalysisPairDetailQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
  pairId: Scalars['ID']['input'];
}>;


export type AnalysisPairDetailQuery = { __typename?: 'Query', analysisPair?: { __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, explanation: string, errorCode: string, selectionReasons: Array<string>, similarityScore?: number | null, resultOrigin: string, reusedFrom: string, checkedAt: string, generatorProfile: string, layaStatus: string, layaErrorCode: string, preclassification?: string | null, layaCheckedAt?: string | null, left: { __typename?: 'AnalysisSource', id: string, version: number, text: string }, right: { __typename?: 'AnalysisSource', id: string, version: number, text: string } } | null };

export type AnalysisRunMetadataQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
}>;


export type AnalysisRunMetadataQuery = { __typename?: 'Query', projectAnalysis?: { __typename?: 'AnalysisRunMetadata', id: string, projectId: string, generatorProfile: string, generatorProvenance: string, embeddingProfile: string, selectionProfile: string } | null };

export type RequirementIndexByIdsQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  requirementIds: Array<Scalars['ID']['input']> | Scalars['ID']['input'];
}>;


export type RequirementIndexByIdsQuery = { __typename?: 'Query', requirementIndexByIds: Array<{ __typename?: 'RequirementIndex', projectId: string, requirementId: string, version: number, status: string, profile: string, errorCode: string }> };

export type RequirementIndexProgressQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
}>;


export type RequirementIndexProgressQuery = { __typename?: 'Query', requirementIndexProgress: { __typename?: 'RequirementIndexProgress', projectId: string, total: number, queued: number, running: number, ready: number, failed: number, profile: string, configured: boolean } };

export type ProjectListQueryVariables = Exact<{
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
}>;


export type ProjectListQuery = { __typename?: 'Query', projects: Array<{ __typename?: 'Project', id: string, name: string, goal: string, description: string, createdAt: string, updatedAt: string }> };

export type ProjectDetailQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type ProjectDetailQuery = { __typename?: 'Query', project?: { __typename?: 'Project', id: string, name: string, goal: string, description: string, createdAt: string, updatedAt: string } | null };

export type CreateProjectMutationVariables = Exact<{
  input: ProjectInput;
}>;


export type CreateProjectMutation = { __typename?: 'Mutation', createProject: { __typename?: 'CreateProjectPayload', project?: { __typename?: 'Project', id: string, name: string, goal: string, description: string, createdAt: string, updatedAt: string } | null, fieldErrors: Array<{ __typename?: 'FieldError', path: Array<string>, code: string, messageKey: string, params: Array<{ __typename?: 'ErrorParam', key: string, value: number }> }> } };

export type RequirementsQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
}>;


export type RequirementsQuery = { __typename?: 'Query', requirements: Array<{ __typename?: 'Requirement', id: string, projectId: string, text: string, version: number, createdAt: string, updatedAt: string }> };

export type AddRequirementMutationVariables = Exact<{
  input: CreateRequirementInput;
}>;


export type AddRequirementMutation = { __typename?: 'Mutation', createRequirement: { __typename?: 'RequirementPayload', requirement?: { __typename?: 'Requirement', id: string, text: string, version: number } | null, fieldErrors: Array<{ __typename?: 'FieldError', path: Array<string>, code: string, messageKey: string, params: Array<{ __typename?: 'ErrorParam', key: string, value: number }> }> } };

export type EditRequirementMutationVariables = Exact<{
  input: UpdateRequirementInput;
}>;


export type EditRequirementMutation = { __typename?: 'Mutation', updateRequirement: { __typename?: 'RequirementPayload', requirement?: { __typename?: 'Requirement', id: string, text: string, version: number } | null, fieldErrors: Array<{ __typename?: 'FieldError', path: Array<string>, code: string, messageKey: string, params: Array<{ __typename?: 'ErrorParam', key: string, value: number }> }> } };

export type RemoveRequirementMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
  version: Scalars['Int']['input'];
}>;


export type RemoveRequirementMutation = { __typename?: 'Mutation', deleteRequirement: { __typename?: 'DeleteRequirementPayload', deleted: boolean } };

export type ProjectConnectionQueryVariables = Exact<{
  first: Scalars['Int']['input'];
  after?: InputMaybe<Scalars['String']['input']>;
}>;


export type ProjectConnectionQuery = { __typename?: 'Query', projectsConnection: { __typename?: 'ProjectConnection', totalCount: number, edges: Array<{ __typename?: 'ProjectEdge', cursor: string, node: { __typename?: 'Project', id: string, name: string, goal: string, description: string, createdAt: string, updatedAt: string } }>, pageInfo: { __typename?: 'PageInfo', endCursor?: string | null, hasNextPage: boolean } } };

export type RequirementConnectionQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  first: Scalars['Int']['input'];
  after?: InputMaybe<Scalars['String']['input']>;
}>;


export type RequirementConnectionQuery = { __typename?: 'Query', requirementsConnection: { __typename?: 'RequirementConnection', totalCount: number, edges: Array<{ __typename?: 'RequirementEdge', cursor: string, node: { __typename?: 'Requirement', id: string, projectId: string, text: string, version: number, createdAt: string, updatedAt: string } }>, pageInfo: { __typename?: 'PageInfo', endCursor?: string | null, hasNextPage: boolean } } };

export type RequirementIndexListQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
}>;


export type RequirementIndexListQuery = { __typename?: 'Query', requirementIndex: Array<{ __typename?: 'RequirementIndex', requirementId: string, version: number, status: string, profile: string, errorCode: string }> };

export type SearchRequirementsQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  text: Scalars['String']['input'];
  limit: Scalars['Int']['input'];
}>;


export type SearchRequirementsQuery = { __typename?: 'Query', searchRequirements: Array<{ __typename?: 'RequirementSearchHit', requirementId: string, projectId: string, text: string, sourceText: string, version: number, start: number, end: number, score: number }> };

export type IndexRequirementMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
}>;


export type IndexRequirementMutation = { __typename?: 'Mutation', indexRequirement: { __typename?: 'RequirementIndex', requirementId: string, version: number, status: string, profile: string, errorCode: string } };

export type RequirementSummariesByIdsQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
  requirementIds: Array<Scalars['ID']['input']> | Scalars['ID']['input'];
}>;


export type RequirementSummariesByIdsQuery = { __typename?: 'Query', requirementSummariesByIds: Array<{ __typename?: 'RequirementSummary', requirementId: string, projectId: string, version: number, status: string, text: string, profile: string, provenance: string, createdAt: string, errorCode: string, byteLength: number, layaEligible: boolean }> };

export type RetryRequirementSummaryMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
}>;


export type RetryRequirementSummaryMutation = { __typename?: 'Mutation', retryRequirementSummary: { __typename?: 'RequirementSummary', requirementId: string, projectId: string, version: number, status: string, text: string, profile: string, provenance: string, createdAt: string, errorCode: string, byteLength: number, layaEligible: boolean } };

export type CancelRequirementSummaryMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  requirementId: Scalars['ID']['input'];
}>;


export type CancelRequirementSummaryMutation = { __typename?: 'Mutation', cancelRequirementSummary: { __typename?: 'RequirementSummary', requirementId: string, projectId: string, version: number, status: string, text: string, profile: string, provenance: string, createdAt: string, errorCode: string, byteLength: number, layaEligible: boolean } };

export const CompactAnalysisSnapshotFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CompactAnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysisSummary"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<CompactAnalysisSnapshotFragment, unknown>;
export const AnalysisSnapshotFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysis"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<AnalysisSnapshotFragment, unknown>;
export const ProjectAnalysisStatusDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectAnalysisStatus"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"selectionProfile"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProvenance"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"embeddingProfile"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}},{"kind":"Field","name":{"kind":"Name","value":"sources"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"findings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leftId"}},{"kind":"Field","name":{"kind":"Name","value":"leftVersion"}},{"kind":"Field","name":{"kind":"Name","value":"rightId"}},{"kind":"Field","name":{"kind":"Name","value":"rightVersion"}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pairs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"selectionReasons"}},{"kind":"Field","name":{"kind":"Name","value":"similarityScore"}},{"kind":"Field","name":{"kind":"Name","value":"resultOrigin"}},{"kind":"Field","name":{"kind":"Name","value":"reusedFrom"}},{"kind":"Field","name":{"kind":"Name","value":"checkedAt"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"layaStatus"}},{"kind":"Field","name":{"kind":"Name","value":"layaErrorCode"}},{"kind":"Field","name":{"kind":"Name","value":"preclassification"}},{"kind":"Field","name":{"kind":"Name","value":"layaCheckedAt"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]}}]} as unknown as DocumentNode<ProjectAnalysisStatusQuery, ProjectAnalysisStatusQueryVariables>;
export const StartProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"mode"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"threshold"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Float"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"mode"},"value":{"kind":"Variable","name":{"kind":"Name","value":"mode"}}},{"kind":"Argument","name":{"kind":"Name","value":"threshold"},"value":{"kind":"Variable","name":{"kind":"Name","value":"threshold"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AnalysisSnapshot"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysis"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<StartProjectAnalysisMutation, StartProjectAnalysisMutationVariables>;
export const ReplaceProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReplaceProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"mode"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"threshold"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Float"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"strategy"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"replaceProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}},{"kind":"Argument","name":{"kind":"Name","value":"mode"},"value":{"kind":"Variable","name":{"kind":"Name","value":"mode"}}},{"kind":"Argument","name":{"kind":"Name","value":"threshold"},"value":{"kind":"Variable","name":{"kind":"Name","value":"threshold"}}},{"kind":"Argument","name":{"kind":"Name","value":"strategy"},"value":{"kind":"Variable","name":{"kind":"Name","value":"strategy"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AnalysisSnapshot"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysis"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<ReplaceProjectAnalysisMutation, ReplaceProjectAnalysisMutationVariables>;
export const RetryProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RetryProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"retryProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AnalysisSnapshot"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysis"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<RetryProjectAnalysisMutation, RetryProjectAnalysisMutationVariables>;
export const AnalysisPairPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalysisPairPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"status"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"verdict"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"scope"},"name":{"kind":"Name","value":"projectAnalysisSummary"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"__typename"}}]}},{"kind":"Field","name":{"kind":"Name","value":"analysisPairs"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}},{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"Variable","name":{"kind":"Name","value":"status"}}},{"kind":"Argument","name":{"kind":"Name","value":"verdict"},"value":{"kind":"Variable","name":{"kind":"Name","value":"verdict"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"selectionReasons"}},{"kind":"Field","name":{"kind":"Name","value":"similarityScore"}},{"kind":"Field","name":{"kind":"Name","value":"resultOrigin"}},{"kind":"Field","name":{"kind":"Name","value":"reusedFrom"}},{"kind":"Field","name":{"kind":"Name","value":"checkedAt"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"layaStatus"}},{"kind":"Field","name":{"kind":"Name","value":"layaErrorCode"}},{"kind":"Field","name":{"kind":"Name","value":"preclassification"}},{"kind":"Field","name":{"kind":"Name","value":"layaCheckedAt"}}]}}]}}]} as unknown as DocumentNode<AnalysisPairPageQuery, AnalysisPairPageQueryVariables>;
export const AnalysisHistoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalysisHistory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"analysisHistory"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"selectionProfile"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}}]}}]}}]} as unknown as DocumentNode<AnalysisHistoryQuery, AnalysisHistoryQueryVariables>;
export const AnalysisPairConnectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalysisPairConnection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"status"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"verdict"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"scope"},"name":{"kind":"Name","value":"projectAnalysisSummary"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"__typename"}}]}},{"kind":"Field","name":{"kind":"Name","value":"analysisPairsConnection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}},{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"Variable","name":{"kind":"Name","value":"status"}}},{"kind":"Argument","name":{"kind":"Name","value":"verdict"},"value":{"kind":"Variable","name":{"kind":"Name","value":"verdict"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"selectionReasons"}},{"kind":"Field","name":{"kind":"Name","value":"similarityScore"}},{"kind":"Field","name":{"kind":"Name","value":"resultOrigin"}},{"kind":"Field","name":{"kind":"Name","value":"reusedFrom"}},{"kind":"Field","name":{"kind":"Name","value":"checkedAt"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"layaStatus"}},{"kind":"Field","name":{"kind":"Name","value":"layaErrorCode"}},{"kind":"Field","name":{"kind":"Name","value":"preclassification"}},{"kind":"Field","name":{"kind":"Name","value":"layaCheckedAt"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<AnalysisPairConnectionQuery, AnalysisPairConnectionQueryVariables>;
export const ProjectAnalysisSummaryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectAnalysisSummary"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"projectAnalysis"},"name":{"kind":"Name","value":"projectAnalysisSummary"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CompactAnalysisSnapshot"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CompactAnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysisSummary"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<ProjectAnalysisSummaryQuery, ProjectAnalysisSummaryQueryVariables>;
export const CancelProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AnalysisSnapshot"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AnalysisSnapshot"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectAnalysis"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revision"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"similarityThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"possiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"notSelectedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"duplicatePairs"}},{"kind":"Field","name":{"kind":"Name","value":"reusedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"newPairs"}},{"kind":"Field","name":{"kind":"Name","value":"completedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"pendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"runningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"failedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"collisionPairs"}},{"kind":"Field","name":{"kind":"Name","value":"compatiblePairs"}},{"kind":"Field","name":{"kind":"Name","value":"insufficientPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaPendingPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaRunningPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaCompletedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaFailedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"layaSkippedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"classifierProfile"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}}]}}]} as unknown as DocumentNode<CancelProjectAnalysisMutation, CancelProjectAnalysisMutationVariables>;
export const AnalysisPairDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalysisPairDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"pairId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"analysisPair"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}},{"kind":"Argument","name":{"kind":"Name","value":"pairId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"pairId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"selectionReasons"}},{"kind":"Field","name":{"kind":"Name","value":"similarityScore"}},{"kind":"Field","name":{"kind":"Name","value":"resultOrigin"}},{"kind":"Field","name":{"kind":"Name","value":"reusedFrom"}},{"kind":"Field","name":{"kind":"Name","value":"checkedAt"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"layaStatus"}},{"kind":"Field","name":{"kind":"Name","value":"layaErrorCode"}},{"kind":"Field","name":{"kind":"Name","value":"preclassification"}},{"kind":"Field","name":{"kind":"Name","value":"layaCheckedAt"}}]}}]}}]} as unknown as DocumentNode<AnalysisPairDetailQuery, AnalysisPairDetailQueryVariables>;
export const AnalysisRunMetadataDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalysisRunMetadata"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"projectAnalysis"},"name":{"kind":"Name","value":"analysisRunMetadata"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProvenance"}},{"kind":"Field","name":{"kind":"Name","value":"embeddingProfile"}},{"kind":"Field","name":{"kind":"Name","value":"selectionProfile"}}]}}]}}]} as unknown as DocumentNode<AnalysisRunMetadataQuery, AnalysisRunMetadataQueryVariables>;
export const RequirementIndexByIdsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementIndexByIds"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementIds"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementIndexByIds"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementIds"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementIds"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]} as unknown as DocumentNode<RequirementIndexByIdsQuery, RequirementIndexByIdsQueryVariables>;
export const RequirementIndexProgressDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementIndexProgress"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementIndexProgress"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"queued"}},{"kind":"Field","name":{"kind":"Name","value":"running"}},{"kind":"Field","name":{"kind":"Name","value":"ready"}},{"kind":"Field","name":{"kind":"Name","value":"failed"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"configured"}}]}}]}}]} as unknown as DocumentNode<RequirementIndexProgressQuery, RequirementIndexProgressQueryVariables>;
export const ProjectListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectList"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projects"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<ProjectListQuery, ProjectListQueryVariables>;
export const ProjectDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"project"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<ProjectDetailQuery, ProjectDetailQueryVariables>;
export const CreateProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"project"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<CreateProjectMutation, CreateProjectMutationVariables>;
export const RequirementsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Requirements"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirements"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<RequirementsQuery, RequirementsQueryVariables>;
export const AddRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRequirementInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirement"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AddRequirementMutation, AddRequirementMutationVariables>;
export const EditRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRequirementInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirement"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<EditRequirementMutation, EditRequirementMutationVariables>;
export const RemoveRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"version"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"version"},"value":{"kind":"Variable","name":{"kind":"Name","value":"version"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleted"}}]}}]}}]} as unknown as DocumentNode<RemoveRequirementMutation, RemoveRequirementMutationVariables>;
export const ProjectConnectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectConnection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectsConnection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<ProjectConnectionQuery, ProjectConnectionQueryVariables>;
export const RequirementConnectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementConnection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementsConnection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<RequirementConnectionQuery, RequirementConnectionQueryVariables>;
export const RequirementIndexListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementIndexList"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementIndex"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]} as unknown as DocumentNode<RequirementIndexListQuery, RequirementIndexListQueryVariables>;
export const SearchRequirementsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SearchRequirements"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"text"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"searchRequirements"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"text"},"value":{"kind":"Variable","name":{"kind":"Name","value":"text"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"sourceText"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"start"}},{"kind":"Field","name":{"kind":"Name","value":"end"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}}]}}]} as unknown as DocumentNode<SearchRequirementsQuery, SearchRequirementsQueryVariables>;
export const IndexRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"IndexRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"indexRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]} as unknown as DocumentNode<IndexRequirementMutation, IndexRequirementMutationVariables>;
export const RequirementSummariesByIdsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementSummariesByIds"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementIds"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementSummariesByIds"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementIds"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementIds"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"provenance"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"byteLength"}},{"kind":"Field","name":{"kind":"Name","value":"layaEligible"}}]}}]}}]} as unknown as DocumentNode<RequirementSummariesByIdsQuery, RequirementSummariesByIdsQueryVariables>;
export const RetryRequirementSummaryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RetryRequirementSummary"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"retryRequirementSummary"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"provenance"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"byteLength"}},{"kind":"Field","name":{"kind":"Name","value":"layaEligible"}}]}}]}}]} as unknown as DocumentNode<RetryRequirementSummaryMutation, RetryRequirementSummaryMutationVariables>;
export const CancelRequirementSummaryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelRequirementSummary"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelRequirementSummary"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"provenance"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"byteLength"}},{"kind":"Field","name":{"kind":"Name","value":"layaEligible"}}]}}]}}]} as unknown as DocumentNode<CancelRequirementSummaryMutation, CancelRequirementSummaryMutationVariables>;