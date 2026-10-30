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
  errorCode: Scalars['String']['output'];
  explanation: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  left: AnalysisSource;
  position: Scalars['Int']['output'];
  right: AnalysisSource;
  status: Scalars['String']['output'];
  verdict: Scalars['String']['output'];
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
  createProject: CreateProjectPayload;
  createRequirement: RequirementPayload;
  deleteRequirement: DeleteRequirementPayload;
  indexRequirement: RequirementIndex;
  retryProjectAnalysis: ProjectAnalysis;
  startProjectAnalysis: ProjectAnalysis;
  updateRequirement: RequirementPayload;
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


export type MutationRetryProjectAnalysisArgs = {
  analysisId: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
};


export type MutationStartProjectAnalysisArgs = {
  projectId: Scalars['ID']['input'];
};


export type MutationUpdateRequirementArgs = {
  input: UpdateRequirementInput;
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
  completedAt: Scalars['String']['output'];
  createdAt: Scalars['String']['output'];
  embeddingProfile: Scalars['String']['output'];
  errorCode: Scalars['String']['output'];
  findings: Array<AnalysisFinding>;
  generatorProfile: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  missingRequirementIds: Array<Scalars['ID']['output']>;
  outcome: Scalars['String']['output'];
  pairs: Array<AnalysisPair>;
  processedPairs: Scalars['Int']['output'];
  projectId: Scalars['ID']['output'];
  reasonCode: Scalars['String']['output'];
  sources: Array<AnalysisSource>;
  stage: Scalars['String']['output'];
  stale: Scalars['Boolean']['output'];
  status: Scalars['String']['output'];
  totalPairs?: Maybe<Scalars['Int']['output']>;
};

export type ProjectInput = {
  description: Scalars['String']['input'];
  goal: Scalars['String']['input'];
  name: Scalars['String']['input'];
};

export type Query = {
  __typename?: 'Query';
  project?: Maybe<Project>;
  projectAnalysis?: Maybe<ProjectAnalysis>;
  projects: Array<Project>;
  requirementIndex: Array<RequirementIndex>;
  requirements: Array<Requirement>;
  searchRequirements: Array<RequirementSearchHit>;
};


export type QueryProjectArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProjectAnalysisArgs = {
  projectId: Scalars['ID']['input'];
};


export type QueryProjectsArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
};


export type QueryRequirementIndexArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
  projectId: Scalars['ID']['input'];
};


export type QueryRequirementsArgs = {
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
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

export type RequirementIndex = {
  __typename?: 'RequirementIndex';
  errorCode: Scalars['String']['output'];
  profile: Scalars['String']['output'];
  requirementId: Scalars['ID']['output'];
  status: Scalars['String']['output'];
  version: Scalars['Int']['output'];
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

export type UpdateRequirementInput = {
  id: Scalars['ID']['input'];
  projectId: Scalars['ID']['input'];
  text: Scalars['String']['input'];
  version: Scalars['Int']['input'];
};

export type ProjectAnalysisStatusQueryVariables = Exact<{
  projectId: Scalars['ID']['input'];
}>;


export type ProjectAnalysisStatusQuery = { __typename?: 'Query', projectAnalysis?: { __typename?: 'ProjectAnalysis', id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, generatorProfile: string, embeddingProfile: string, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string, sources: Array<{ __typename?: 'AnalysisSource', id: string, version: number, text: string }>, findings: Array<{ __typename?: 'AnalysisFinding', leftId: string, rightId: string, leftVersion: number, rightVersion: number, verdict: string, explanation: string }>, pairs: Array<{ __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, explanation: string, errorCode: string, left: { __typename?: 'AnalysisSource', id: string, version: number, text: string }, right: { __typename?: 'AnalysisSource', id: string, version: number, text: string } }> } | null };

export type StartProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
}>;


export type StartProjectAnalysisMutation = { __typename?: 'Mutation', startProjectAnalysis: { __typename?: 'ProjectAnalysis', id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, generatorProfile: string, embeddingProfile: string, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string, sources: Array<{ __typename?: 'AnalysisSource', id: string, version: number, text: string }>, findings: Array<{ __typename?: 'AnalysisFinding', leftId: string, rightId: string, leftVersion: number, rightVersion: number, verdict: string, explanation: string }>, pairs: Array<{ __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, explanation: string, errorCode: string, left: { __typename?: 'AnalysisSource', id: string, version: number, text: string }, right: { __typename?: 'AnalysisSource', id: string, version: number, text: string } }> } };

export type RetryProjectAnalysisMutationVariables = Exact<{
  projectId: Scalars['ID']['input'];
  analysisId: Scalars['ID']['input'];
}>;


export type RetryProjectAnalysisMutation = { __typename?: 'Mutation', retryProjectAnalysis: { __typename?: 'ProjectAnalysis', id: string, projectId: string, status: string, outcome: string, errorCode: string, reasonCode: string, missingRequirementIds: Array<string>, createdAt: string, completedAt: string, stale: boolean, generatorProfile: string, embeddingProfile: string, stage: string, totalPairs?: number | null, processedPairs: number, activePairId: string, sources: Array<{ __typename?: 'AnalysisSource', id: string, version: number, text: string }>, findings: Array<{ __typename?: 'AnalysisFinding', leftId: string, rightId: string, leftVersion: number, rightVersion: number, verdict: string, explanation: string }>, pairs: Array<{ __typename?: 'AnalysisPair', id: string, position: number, status: string, verdict: string, explanation: string, errorCode: string, left: { __typename?: 'AnalysisSource', id: string, version: number, text: string }, right: { __typename?: 'AnalysisSource', id: string, version: number, text: string } }> } };

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


export const ProjectAnalysisStatusDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectAnalysisStatus"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"embeddingProfile"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}},{"kind":"Field","name":{"kind":"Name","value":"sources"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"findings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leftId"}},{"kind":"Field","name":{"kind":"Name","value":"rightId"}},{"kind":"Field","name":{"kind":"Name","value":"leftVersion"}},{"kind":"Field","name":{"kind":"Name","value":"rightVersion"}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pairs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]}}]} as unknown as DocumentNode<ProjectAnalysisStatusQuery, ProjectAnalysisStatusQueryVariables>;
export const StartProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"embeddingProfile"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}},{"kind":"Field","name":{"kind":"Name","value":"sources"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"findings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leftId"}},{"kind":"Field","name":{"kind":"Name","value":"rightId"}},{"kind":"Field","name":{"kind":"Name","value":"leftVersion"}},{"kind":"Field","name":{"kind":"Name","value":"rightVersion"}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pairs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]}}]} as unknown as DocumentNode<StartProjectAnalysisMutation, StartProjectAnalysisMutationVariables>;
export const RetryProjectAnalysisDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RetryProjectAnalysis"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"retryProjectAnalysis"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"analysisId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"analysisId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"outcome"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}},{"kind":"Field","name":{"kind":"Name","value":"missingRequirementIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stale"}},{"kind":"Field","name":{"kind":"Name","value":"generatorProfile"}},{"kind":"Field","name":{"kind":"Name","value":"embeddingProfile"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"totalPairs"}},{"kind":"Field","name":{"kind":"Name","value":"processedPairs"}},{"kind":"Field","name":{"kind":"Name","value":"activePairId"}},{"kind":"Field","name":{"kind":"Name","value":"sources"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"findings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leftId"}},{"kind":"Field","name":{"kind":"Name","value":"rightId"}},{"kind":"Field","name":{"kind":"Name","value":"leftVersion"}},{"kind":"Field","name":{"kind":"Name","value":"rightVersion"}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pairs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"left"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"right"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"text"}}]}},{"kind":"Field","name":{"kind":"Name","value":"verdict"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]}}]} as unknown as DocumentNode<RetryProjectAnalysisMutation, RetryProjectAnalysisMutationVariables>;
export const ProjectListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectList"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projects"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<ProjectListQuery, ProjectListQueryVariables>;
export const ProjectDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProjectDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"project"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<ProjectDetailQuery, ProjectDetailQueryVariables>;
export const CreateProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ProjectInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"project"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"goal"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<CreateProjectMutation, CreateProjectMutationVariables>;
export const RequirementsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Requirements"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirements"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<RequirementsQuery, RequirementsQueryVariables>;
export const AddRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRequirementInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirement"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AddRequirementMutation, AddRequirementMutationVariables>;
export const EditRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRequirementInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirement"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fieldErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"messageKey"}},{"kind":"Field","name":{"kind":"Name","value":"params"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]}}]}}]} as unknown as DocumentNode<EditRequirementMutation, EditRequirementMutationVariables>;
export const RemoveRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"version"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"version"},"value":{"kind":"Variable","name":{"kind":"Name","value":"version"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleted"}}]}}]}}]} as unknown as DocumentNode<RemoveRequirementMutation, RemoveRequirementMutationVariables>;
export const RequirementIndexListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RequirementIndexList"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementIndex"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]} as unknown as DocumentNode<RequirementIndexListQuery, RequirementIndexListQueryVariables>;
export const SearchRequirementsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SearchRequirements"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"text"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"searchRequirements"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"text"},"value":{"kind":"Variable","name":{"kind":"Name","value":"text"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}},{"kind":"Field","name":{"kind":"Name","value":"text"}},{"kind":"Field","name":{"kind":"Name","value":"sourceText"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"start"}},{"kind":"Field","name":{"kind":"Name","value":"end"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}}]}}]} as unknown as DocumentNode<SearchRequirementsQuery, SearchRequirementsQueryVariables>;
export const IndexRequirementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"IndexRequirement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"indexRequirement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"projectId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"projectId"}}},{"kind":"Argument","name":{"kind":"Name","value":"requirementId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"requirementId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requirementId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profile"}},{"kind":"Field","name":{"kind":"Name","value":"errorCode"}}]}}]}}]} as unknown as DocumentNode<IndexRequirementMutation, IndexRequirementMutationVariables>;