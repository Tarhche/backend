package login

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject.git/domain/user"
	"github.com/khanzadimahdi/testproject.git/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject.git/infrastructure/jwt"
)

func TestUseCase_GetArticle(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("returns jwt tokens", func(t *testing.T) {
		repository := MockUserRepository{}

		usecase := NewUseCase(&repository, j)

		request := Request{
			Username: "test-username",
			Password: "test-password",
		}

		response, err := usecase.Login(request)

		if repository.GetOneByUsernameCount != 1 {
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

		usecase := NewUseCase(&repository, j)

		request := Request{
			Username: "test-username",
			Password: "test-password",
		}

		response, err := usecase.Login(request)

		if repository.GetOneByUsernameCount != 1 {
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

	GetOneByUsernameCount uint
	GetOneErr             error
}

func (r *MockUserRepository) GetOneByUsername(username string) (user.User, error) {
	r.GetOneByUsernameCount++

	if r.GetOneErr != nil {
		return user.User{}, r.GetOneErr
	}

	return user.User{
		Username: username,
		Password: "test-password",
	}, nil
}
