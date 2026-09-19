package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// ErrNoVerifiedEmail means the provider vouched for somebody this estate has no
// way to name. An address a provider has not verified is an address somebody
// typed into it, and enrolling on one would be enrolling as whoever owns it
// here.
var ErrNoVerifiedEmail = errors.New("the provider gave no verified email address")

// usernameAttempts is how many times a taken username is tried with a number on
// the end before the address itself is given up on.
const usernameAttempts = 100

// Identities is who somebody else's account stands for here.
//
// Signing in and signing up are the same door: a person arriving with a Google
// account this estate has never seen cannot tell you whether they meant to
// register, and does not care. What they get is the account they already have,
// or one made for them on the spot.
//
// Three things can be true of an arrival, and they are tried in this order:
// they have signed in this way before, and are that user; they have not, but an
// account already answers to the address the provider verified, and it is
// theirs by another door; or nobody is them, and they are enrolled.
type Identities struct {
	userRepository   user.Repository
	roleRepository   role.Repository
	configRepository config.Repository
	languageResolver resolver.Resolver
}

func NewIdentities(
	userRepository user.Repository,
	roleRepository role.Repository,
	configRepository config.Repository,
	languageResolver resolver.Resolver,
) *Identities {
	return &Identities{
		userRepository:   userRepository,
		roleRepository:   roleRepository,
		configRepository: configRepository,
		languageResolver: languageResolver,
	}
}

func (i *Identities) Resolve(ctx context.Context, identity oauth.Identity) (user.User, error) {
	u, err := i.userRepository.GetOneByProviderIdentity(ctx, identity.Provider, identity.ID)
	if err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrNotExists) {
		return user.User{}, err
	}

	// an address the provider has not verified proves nothing, so it is not
	// allowed to find an account here, let alone open one
	if !identity.Verified || len(identity.Email) == 0 {
		return user.User{}, ErrNoVerifiedEmail
	}

	existing, err := i.userRepository.GetOneByIdentity(ctx, identity.Email)
	if err == nil {
		return i.link(ctx, existing, identity)
	} else if !errors.Is(err, domain.ErrNotExists) {
		return user.User{}, err
	}

	return i.enroll(ctx, identity)
}

// link remembers that this account is also reached that way, so the next
// arrival is recognised by the provider's own id rather than by an address the
// person may since have changed.
func (i *Identities) link(ctx context.Context, u user.User, identity oauth.Identity) (user.User, error) {
	if u.HasIdentity(identity.Provider, identity.ID) {
		return u, nil
	}

	u.Identities = append(u.Identities, user.Identity{
		Provider: identity.Provider,
		ID:       identity.ID,
	})

	if _, err := i.userRepository.Save(ctx, &u); err != nil {
		return user.User{}, err
	}

	return u, nil
}

// enroll opens an account for somebody who has never been here. They have no
// password: the provider is how they get in, and it is how they will get in
// next time.
func (i *Identities) enroll(ctx context.Context, identity oauth.Identity) (user.User, error) {
	username, err := i.availableUsername(ctx, identity)
	if err != nil {
		return user.User{}, err
	}

	languageCode, err := i.languageResolver.DefaultCode(ctx)
	if err != nil {
		return user.User{}, err
	}

	name := identity.Name
	if len(name) == 0 {
		name = username
	}

	u := user.User{
		Name:         name,
		Email:        identity.Email,
		Username:     username,
		Avatar:       identity.Avatar,
		LanguageCode: languageCode,
		Identities: []user.Identity{{
			Provider: identity.Provider,
			ID:       identity.ID,
		}},
	}

	userUUID, err := i.userRepository.Save(ctx, &u)
	if err != nil {
		return user.User{}, err
	}

	u.UUID = userUUID

	if err := i.assignDefaultRoles(ctx, userUUID); err != nil {
		return user.User{}, err
	}

	return u, nil
}

// availableUsername is a username nobody here has yet, built from the address
// the person signed in with and numbered when that is already somebody's.
func (i *Identities) availableUsername(ctx context.Context, identity oauth.Identity) (string, error) {
	candidate := usernameFromEmail(identity.Email)

	for attempt := range usernameAttempts {
		username := candidate
		if attempt > 0 {
			username += strconv.Itoa(attempt + 1)
		}

		taken, err := i.usernameIsTaken(ctx, username)
		if err != nil {
			return "", err
		}

		if !taken {
			return username, nil
		}
	}

	return "", ErrNoVerifiedEmail
}

func (i *Identities) usernameIsTaken(ctx context.Context, username string) (bool, error) {
	_, err := i.userRepository.GetOneByIdentity(ctx, username)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return true, err
	}

	return true, nil
}

// usernameFromEmail keeps of an address only what a username may hold:
// lowercase letters, digits, and the punctuation between them.
func usernameFromEmail(email string) string {
	local, _, _ := strings.Cut(email, "@")

	var builder strings.Builder
	for _, r := range strings.ToLower(local) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			builder.WriteRune(r)
		}
	}

	username := strings.Trim(builder.String(), "._-")

	// an address of nothing but punctuation, or written in another script,
	// leaves nothing to call somebody: the number on the end of the next
	// attempt is what will name them.
	if len(username) == 0 {
		return "user"
	}

	return username
}

// assignDefaultRoles gives a new account whatever the estate hands everybody,
// which is the same welcome a registration through the form gets.
func (i *Identities) assignDefaultRoles(ctx context.Context, userUUID string) error {
	c, err := i.configRepository.GetLatestRevision(ctx)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	roles, err := i.roleRepository.GetByUUIDs(ctx, c.UserDefaultRoleUUIDs)
	if err != nil {
		return err
	}

	for i2 := range roles {
		roles[i2].UserUUIDs = append(roles[i2].UserUUIDs, userUUID)

		if _, err := i.roleRepository.Save(ctx, &roles[i2]); err != nil {
			return err
		}
	}

	return nil
}
