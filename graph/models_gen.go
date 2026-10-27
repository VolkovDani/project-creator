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
