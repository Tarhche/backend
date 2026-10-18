// Package workloadtest is what the workload's dashboard use cases are tested
// with: the words they answer in, whose things they are asked about, and the
// ways the workload answers other than with what was asked for.
//
// Every one of those use cases passes a request on to the workload and makes
// the same few things of what comes back, so the answers are written down once
// here and each use case is held to all of them.
package workloadtest

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

const (
	// OwnerUUID is the person the my routes ask for, and who owns what the
	// tests are about.
	OwnerUUID = "owner-uuid"

	// IngressDomain is where the workload serves ports, as production has it.
	IngressDomain = "workload.example.com"
)

// Translator says codes in English, as a request in English is answered.
func Translator() translatorContract.Translator {
	return translator.New(translation.Translations, translation.EN)
}

// Validator holds a request to its rules and says what is wrong in English.
func Validator() domain.Validator {
	return validator.New(Translator())
}

// Owner is who OwnerUUID is.
func Owner() user.User {
	return user.User{UUID: OwnerUUID, Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}
}

// Owners puts a name to OwnerUUID, and to nobody else.
func Owners() *presenter.Directory {
	users := &usersMock.MockUsersRepository{}
	users.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{Owner()}, nil).Maybe()

	return presenter.NewDirectory(users)
}

// Scope is whose things a request is asked about.
type Scope struct {
	Name      string
	OwnerUUID string
}

// Scopes are the two ways a route asks: for anybody's, from the workload's
// routes, and for one person's own, from the my routes. Either way the use case
// hands the workload what it was given, and the workload does the narrowing.
var Scopes = []Scope{
	{Name: "anybody's, from the workload's routes", OwnerUUID: ""},
	{Name: "one's own, from the my routes", OwnerUUID: OwnerUUID},
}

// ErrUnreachable is a workload that could not be asked at all.
var ErrUnreachable = errors.New("the workload is unreachable")

// Answer is one way the workload answers other than with what was asked
// for, and what a use case is expected to make of it: a refusal, said in
// English by field, or a failure.
type Answer struct {
	Name string

	// Err is what the workload answers with.
	Err error

	// Refused is what the request is refused with; Failure is the error the
	// use case fails with instead. One of the two is set.
	Refused domain.ValidationErrors
	Failure error
}

// Answers are every way a use case that is refused by the workload is
// answered.
func Answers() []Answer {
	return []Answer{
		{
			Name:    "what the control plane refuses is said in the reader's language",
			Err:     &client.ValidationError{ValidationErrors: domain.ValidationErrors{"resources.memory": "quota_exceeded"}},
			Refused: domain.ValidationErrors{"resources.memory": "this would take you past your quota"},
		},
		{
			Name:    "a reason there are no words for is kept as the control plane gave it",
			Err:     &client.ValidationError{ValidationErrors: domain.ValidationErrors{"name": "is taken"}},
			Refused: domain.ValidationErrors{"name": "is taken"},
		},
		{
			Name:    "what a node refuses is said under what it is about",
			Err:     &noderequest.Error{Code: noderequest.CodeNotRunning, Message: "the vm is stopped"},
			Refused: domain.ValidationErrors{"vm": "the VM is not running"},
		},
		{
			Name:    "what dockerd refuses keeps its own words, which say what to fix",
			Err:     &noderequest.Error{Code: noderequest.CodeInvalid, Message: "No such image: nope:latest"},
			Refused: domain.ValidationErrors{"docker": "Docker refused the request: No such image: nope:latest"},
		},
		Failures()[0],
		Failures()[1],
		Failures()[2],
	}
}

// Failures are the answers that are not refusals: something that is not
// there, something that took too long, and a workload that could not be
// asked. A read of a record is only ever answered one of these ways besides
// with the record.
func Failures() []Answer {
	return []Answer{
		{
			Name:    "something that is not there, or not the caller's, is not found",
			Err:     &noderequest.Error{Code: noderequest.CodeNotFound},
			Failure: domain.ErrNotExists,
		},
		{
			Name:    "something that took too long is a timeout, which may yet be done",
			Err:     &noderequest.Error{Code: noderequest.CodeTimeout},
			Failure: context.DeadlineExceeded,
		},
		{
			Name:    "a workload that could not be asked is a failure, not a refusal",
			Err:     ErrUnreachable,
			Failure: ErrUnreachable,
		},
	}
}
