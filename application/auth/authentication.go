package auth

import (
	"context"
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

	AccessTokenExpirationTime        = 15 * time.Minute
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

func ToContext(ctx context.Context, user *user.User) context.Context {
	return context.WithValue(ctx, AuthKey, user)
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

func (t *AuthTokenGenerator) GenerateAccessToken(userUUID string) (string, error) {
	roles, err := t.roleRepository.GetByUserUUID(userUUID)
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
	b.SetSubject(userUUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(AccessTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{AccessToken})
	b.SetRoles(roleNames)
	b.SetPermissions(permissionNames)

	return t.jwt.Generate(b.Build())
}

func (t *AuthTokenGenerator) GenerateRefreshToken(userUUID string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(userUUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(RefreshTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{RefreshToken})

	return t.jwt.Generate(b.Build())
}

func (t *AuthTokenGenerator) GenerateResetPasswordToken(userUUID string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(userUUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(ResetPasswordTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{ResetPasswordToken})

	return t.jwt.Generate(b.Build())
}

func (t *AuthTokenGenerator) GenerateRegistrationToken(identity string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(identity)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(RegistrationTokenExpirationTime))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{RegistrationToken})

	return t.jwt.Generate(b.Build())
}
