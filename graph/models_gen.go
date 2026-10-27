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
type ProjectArgs struct {
	ID graphql.ID
}
type ProjectsArgs struct {
	Limit  int32
	Offset int32
}
type CreateProjectArgs struct {
	Input ProjectInput
}
