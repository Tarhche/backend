package resetpassword

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

func TestUseCase_ResetPassword(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	t.Run("malformed base64 token", func(t *testing.T) {
		repository := MockUserRepository{}
		h := &MockHasher{}

		usecase := NewUseCase(&repository, h, j)

		request := Request{
			Token:    "random.base64.token",
			Password: "test-password",
		}

		response, err := usecase.ResetPassword(request)
		if err == nil {
			t.Error("expected an error")
		}

		if response != nil {
			t.Errorf("unexpected response %#v", response)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		repository := MockUserRepository{}
		h := &MockHasher{}

		usecase := NewUseCase(&repository, h, j)

		testCases := make(map[string]string)
		testCases["expired token"], _ = resetPasswordToken(j, user.User{UUID: "test-uuid"}, time.Now().Add(-time.Second), auth.ResetPasswordToken)
		testCases["invalid audience"], _ = resetPasswordToken(j, user.User{UUID: "test-uuid"}, time.Now().Add(-time.Second), auth.AccessToken)
		testCases["expired token"], _ = resetPasswordToken(j, user.User{}, time.Now().Add(-time.Second), auth.ResetPasswordToken)

		for i := range testCases {
			request := Request{
				Token:    testCases[i],
				Password: "test-password",
			}

			response, err := usecase.ResetPassword(request)
			if err == nil {
				t.Error("expected an error")
			}

			if response != nil {
				t.Errorf("unexpected response %#v", response)
			}
		}
	})

	t.Run("error on fetching user", func(t *testing.T) {
		repository := MockUserRepository{
			GetOneErr: domain.ErrNotExists,
		}
		h := &MockHasher{}

		usecase := NewUseCase(&repository, h, j)

		token, _ := resetPasswordToken(j, user.User{UUID: "test-uuid"}, time.Now().Add(1*time.Minute), auth.ResetPasswordToken)
		request := Request{
			Token:    token,
			Password: "test-password",
		}

		response, err := usecase.ResetPassword(request)
		if err != domain.ErrNotExists {
			t.Errorf("expected error is %q but got %q", domain.ErrNotExists, err)
		}

		if repository.GetOneCount != 1 {
			t.Errorf("fetching user should happen only once, but happend %d times", repository.GetOneCount)
		}

		if response != nil {
			t.Errorf("unexpected response %#v", response)
		}
	})

	t.Run("error on persisting user's password", func(t *testing.T) {
		repository := MockUserRepository{
			SaveErr: domain.ErrNotExists,
		}
		h := &MockHasher{}

		usecase := NewUseCase(&repository, h, j)

		token, _ := resetPasswordToken(j, user.User{UUID: "test-uuid"}, time.Now().Add(1*time.Minute), auth.ResetPasswordToken)
		request := Request{
			Token:    token,
			Password: "test-password",
		}

		response, err := usecase.ResetPassword(request)
		if err != domain.ErrNotExists {
			t.Errorf("expected error is %q but got %q", domain.ErrNotExists, err)
		}

		if repository.GetOneCount != 1 {
			t.Errorf("fetching user should happen only once, but happend %d times", repository.GetOneCount)
		}

		if h.HashCount != 1 {
			t.Errorf("hashing password should happen only once, but happend %d times", h.HashCount)
		}

		if repository.SaveCount != 1 {
			t.Errorf("persisting user should happen only once, but happend %d times", repository.SaveCount)
		}

		if response != nil {
			t.Errorf("unexpected response %#v", response)
		}
	})

	t.Run("password successfully updates", func(t *testing.T) {
		repository := MockUserRepository{
			SaveErr: domain.ErrNotExists,
		}
		h := &MockHasher{}

		usecase := NewUseCase(&repository, h, j)

		token, _ := resetPasswordToken(j, user.User{UUID: "test-uuid"}, time.Now().Add(1*time.Minute), auth.ResetPasswordToken)
		request := Request{
			Token:    token,
			Password: "test-password",
		}

		response, err := usecase.ResetPassword(request)
		if err != domain.ErrNotExists {
			t.Errorf("expected error is %q but got %q", domain.ErrNotExists, err)
		}

		if repository.GetOneCount != 1 {
			t.Errorf("fetching user should happen only once, but happend %d times", repository.GetOneCount)
		}

		if h.HashCount != 1 {
			t.Errorf("hashing password should happen only once, but happend %d times", h.HashCount)
		}

		if repository.SaveCount != 1 {
			t.Errorf("persisting user should happen only once, but happend %d times", repository.SaveCount)
		}

		if response != nil {
			t.Errorf("unexpected response %#v", response)
		}
	})
}

type MockUserRepository struct {
	user.Repository

	GetOneCount uint
	GetOneErr   error

	SaveCount uint
	SaveErr   error
}

func (r *MockUserRepository) GetOne(UUID string) (user.User, error) {
	r.GetOneCount++

	if r.GetOneErr != nil {
		return user.User{}, r.GetOneErr
	}

	return user.User{
		UUID: UUID,
		PasswordHash: password.Hash{
			Value: []byte("test-password"),
			Salt:  []byte("test-salt"),
		},
	}, nil
}

func (r *MockUserRepository) Save(u *user.User) error {
	r.SaveCount++

	return r.SaveErr
}

type MockHasher struct {
	password.Hasher
	HashCount uint
}

func (m *MockHasher) Hash(value, salt []byte) []byte {
	m.HashCount++

	return []byte("random hash")
}

func resetPasswordToken(j *jwt.JWT, u user.User, expiresAt time.Time, audience string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now().Add(-time.Hour))
	b.SetExpirationTime(expiresAt)
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{audience})

	t, err := j.Generate(b.Build())
	if err != nil {
		return t, err
	}

	return base64.URLEncoding.EncodeToString([]byte(t)), nil
}
