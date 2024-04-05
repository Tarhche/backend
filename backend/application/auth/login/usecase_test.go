package login

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

func TestUseCase_Login(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())
	h := &MockHasher{}

	t.Run("returns jwt tokens", func(t *testing.T) {
		repository := MockUserRepository{}

		usecase := NewUseCase(&repository, j, h)

		request := Request{
			Identity: "test-username",
			Password: "test-password",
		}

		response, err := usecase.Login(request)

		if repository.GetOneByIdentityCount != 1 {
			t.Error("unexpected number of calls")
		}

		if response == nil {
			t.Fatal("unexpected response")
		}

		if err != nil {
			t.Error("unexpected an error")
		}

		accessTokenClaims, err := j.Verify(response.AccessToken)
		if err != nil {
			t.Errorf("unexpected an error %s", err)
		}

		audience, err := accessTokenClaims.GetAudience()
		t.Log(audience)
		if err != nil {
			t.Error("unexpected an error")
		}
		if audience[0] != "access" {
			t.Error("invalid audience")
		}

		refreshTokenClaims, err := j.Verify(response.RefreshToken)
		if err != nil {
			t.Error("unexpected an error")
		}

		audience, err = refreshTokenClaims.GetAudience()
		if err != nil {
			t.Error("unexpected an error")
		}
		if audience[0] != "refresh" {
			t.Error("invalid audience")
		}
	})

	t.Run("returns an error", func(t *testing.T) {
		repository := MockUserRepository{
			GetOneErr: errors.New("user with given username found"),
		}

		usecase := NewUseCase(&repository, j, h)

		request := Request{
			Identity: "test-username",
			Password: "test-password",
		}

		response, err := usecase.Login(request)

		if repository.GetOneByIdentityCount != 1 {
			t.Error("unexpected number of calls")
		}

		if response != nil {
			t.Error("unexpected response")
		}

		if err == nil {
			t.Error("expects an error")
		}
	})
}

type MockUserRepository struct {
	user.Repository

	GetOneByIdentityCount uint
	GetOneErr             error
}

func (r *MockUserRepository) GetOneByIdentity(username string) (user.User, error) {
	r.GetOneByIdentityCount++

	if r.GetOneErr != nil {
		return user.User{}, r.GetOneErr
	}

	return user.User{
		Username: username,
		PasswordHash: password.Hash{
			Value: []byte("test-password"),
			Salt:  []byte("test-salt"),
		},
	}, nil
}

type MockHasher struct {
}

func (m *MockHasher) Hash(value, salt []byte) []byte {
	return []byte("random hash")
}

func (m *MockHasher) Equal(value, hash, salt []byte) bool {
	return true
}
