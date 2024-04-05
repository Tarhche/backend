package forgetpassword

import (
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

func TestUseCase_SendResetToken(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}
	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"

	request := Request{
		Identity: "something@somewhere.loc",
	}

	t.Run("mails reset password token", func(t *testing.T) {
		userRepository := &MockUserRepository{}
		mailer := &MockMailer{}

		usecase := NewUseCase(userRepository, j, mailer, mailFrom)
		response, err := usecase.SendResetToken(request)

		if err != nil {
			t.Error("unexpected error")
		}

		if response == nil {
			t.Errorf("response not exists")
		}
	})

	t.Run("error on finding user", func(t *testing.T) {
		userRepository := &MockUserRepository{
			GetOneByIdentityErr: domain.ErrNotExists,
		}
		mailer := &MockMailer{}

		usecase := NewUseCase(userRepository, j, mailer, mailFrom)
		response, err := usecase.SendResetToken(request)

		if err != userRepository.GetOneByIdentityErr {
			t.Error("expected an error", userRepository.GetOneByIdentityErr)
		}

		if response != nil {
			t.Errorf("expected response to be nil but got %#v", response)
		}
	})

	t.Run("sending email fails", func(t *testing.T) {
		userRepository := &MockUserRepository{}
		mailer := &MockMailer{SendMailErr: errors.New("can't send mail")}

		usecase := NewUseCase(userRepository, j, mailer, mailFrom)
		response, err := usecase.SendResetToken(request)

		if err != mailer.SendMailErr {
			t.Error("expected an error", mailer.SendMailErr)
		}

		if response != nil {
			t.Errorf("expected response to be nil but got %#v", response)
		}
	})

}

type MockUserRepository struct {
	user.Repository

	GetOneByIdentityErr error
}

func (r *MockUserRepository) GetOneByIdentity(username string) (user.User, error) {
	if r.GetOneByIdentityErr != nil {
		return user.User{}, r.GetOneByIdentityErr
	}

	return user.User{
		UUID: "018ead22-d9d3-7e78-8b52-174c06ee1528",
	}, nil
}

type MockMailer struct {
	domain.Mailer
	SendMailErr error
}

func (r *MockMailer) SendMail(from string, to string, subject string, body []byte) error {
	return r.SendMailErr
}
