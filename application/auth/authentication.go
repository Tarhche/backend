package auth

import (
	"context"
	"reflect"
	"time"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	AccessToken        = "permission"
	RefreshToken       = "refresh"
	ResetPasswordToken = "reset-password"
	RegistrationToken  = "registration"

	AccessTokenExpirationTime        = 3 * time.Minute
	RefreshTokenExpirationTime       = 2 * 24 * time.Hour
	ResetPasswordTokenExpirationTime = 15 * time.Minute
	RegistrationTokenExpirationTime  = 24 * time.Hour
)

type authKey struct{}

// AuthKey is the request context key under which URL params are stored.
var AuthKey = authKey{}

func FromContext(ctx context.Context) *user.User {
	u, _ := ctx.Value(AuthKey).(*user.User)

	return u
}

func UUIDFromContext(ctx context.Context) string {
	u := FromContext(ctx)
	if u == nil || reflect.ValueOf(u).IsNil() || len(u.UUID) == 0 {
		return ""
	}

	return u.UUID
}

func ToContext(ctx context.Context, user *user.User) context.Context {
	return context.WithValue(ctx, AuthKey, user)
}

type impersonatorKey struct{}

// ImpersonatorKey is the request context key under which whoever is behind a
// shadow session is stored.
var ImpersonatorKey = impersonatorKey{}

// ImpersonatorFromContext is who obtained this session to be seen as the user
// the request is from, and is empty for an ordinary session.
func ImpersonatorFromContext(ctx context.Context) string {
	impersonatorUUID, _ := ctx.Value(ImpersonatorKey).(string)

	return impersonatorUUID
}

// IdentityToContext carries everything a token established: who the request
// acts as, and who, if anybody, is behind it.
func IdentityToContext(ctx context.Context, identity Identity) context.Context {
	ctx = ToContext(ctx, &identity.User)

	if len(identity.ImpersonatorUUID) > 0 {
		ctx = context.WithValue(ctx, ImpersonatorKey, identity.ImpersonatorUUID)
	}

	return ctx
}

// TokenOption says something more about the session a token stands for.
type TokenOption func(*tokenOptions)

type tokenOptions struct {
	impersonatorUUID string
}

// OnBehalfOf marks a token as one somebody else obtained to be seen as its
// subject. The token is the subject's in every other way -- their permissions,
// their language, their articles -- and this is the only thing that says who is
// behind it.
func OnBehalfOf(impersonatorUUID string) TokenOption {
	return func(o *tokenOptions) {
		o.impersonatorUUID = impersonatorUUID
	}
}

func newTokenOptions(options []TokenOption) tokenOptions {
	var o tokenOptions
	for _, option := range options {
		option(&o)
	}

	return o
}

type AuthTokenGenerator struct {
	jwt            *jwt.JWT
	roleRepository role.Repository
}

func NewTokenGenerator(jwt *jwt.JWT, roleRepository role.Repository) *AuthTokenGenerator {
	return &AuthTokenGenerator{
		jwt:            jwt,
		roleRepository: roleRepository,
	}
}

func (t *AuthTokenGenerator) GenerateAccessToken(ctx context.Context, u *user.User, options ...TokenOption) (string, error) {
	roles, err := t.roleRepository.GetByUserUUID(ctx, u.UUID)
	if err != nil {
		return "", err
	}

	var permissionsCount int
	for i := range roles {
		permissionsCount += len(roles[i].Permissions)
	}

	uniqueRoleNames := make(map[string]struct{}, len(roles))
	uniquePermissionNames := make(map[string]struct{}, permissionsCount)
	for i := range roles {
		uniqueRoleNames[roles[i].Name] = struct{}{}
		for _, permission := range roles[i].Permissions {
			uniquePermissionNames[permission] = struct{}{}
		}
	}

	roleNames := make([]string, 0, len(uniqueRoleNames))
	for name := range uniqueRoleNames {
		roleNames = append(roleNames, name)
	}

	permissionNames := make([]string, 0, len(uniquePermissionNames))
	for name := range uniquePermissionNames {
		permissionNames = append(permissionNames, name)
	}

	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(AccessTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{AccessToken})
	b.SetRoles(roleNames)
	b.SetPermissions(permissionNames)
	b.SetLanguage(u.LanguageCode)

	if o := newTokenOptions(options); len(o.impersonatorUUID) > 0 {
		b.SetImpersonator(o.impersonatorUUID)
	}

	return t.jwt.Generate(ctx, b.Build())
}

func (t *AuthTokenGenerator) GenerateRefreshToken(ctx context.Context, userUUID string, options ...TokenOption) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(userUUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(RefreshTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{RefreshToken})

	// a shadow session outlives its access token, so the refresh token has to
	// carry who is behind it too -- otherwise the next refresh would hand the
	// impersonated user their own session.
	if o := newTokenOptions(options); len(o.impersonatorUUID) > 0 {
		b.SetImpersonator(o.impersonatorUUID)
	}

	return t.jwt.Generate(ctx, b.Build())
}

func (t *AuthTokenGenerator) GenerateResetPasswordToken(ctx context.Context, userUUID string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(userUUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(ResetPasswordTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{ResetPasswordToken})

	return t.jwt.Generate(ctx, b.Build())
}

func (t *AuthTokenGenerator) GenerateRegistrationToken(ctx context.Context, identity string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(identity)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(RegistrationTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{RegistrationToken})

	return t.jwt.Generate(ctx, b.Build())
}
