package session

import (
	"atlas-account/account"
	"atlas-account/configuration"
	"atlas-account/tenant"

	"github.com/google/uuid"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	SystemError       = "SYSTEM_ERROR"
	NotRegistered     = "NOT_REGISTERED"
	DeletedOrBlocked  = "DELETED_OR_BLOCKED"
	AlreadyLoggedIn   = "ALREADY_LOGGED_IN"
	IncorrectPassword = "INCORRECT_PASSWORD"
	TooManyAttempts   = "TOO_MANY_ATTEMPTS"
	LicenseAgreement  = "LICENSE_AGREEMENT"
)

func AttemptLogin(l logrus.FieldLogger, db *gorm.DB, span opentracing.Span, tenant tenant.Model) func(sessionId uuid.UUID, name string, password string) Model {
	return func(sessionId uuid.UUID, name string, password string) Model {
		l.Debugf("Attemting login for [%s].", name)
		if checkLoginAttempts(sessionId) > 4 {
			return ErrorModel(TooManyAttempts)
		}

		c, err := configuration.Get()
		if err != nil {
			l.WithError(err).Errorf("Error reading needed configuration.")
			return ErrorModel(SystemError)
		}

		a, err := account.GetOrCreate(l, db, span, tenant)(name, password, c.AutomaticRegister)
		if err != nil && !c.AutomaticRegister {
			return ErrorModel(NotRegistered)
		}
		if err != nil {
			return ErrorModel(SystemError)
		}

		if a.Banned() {
			return ErrorModel(DeletedOrBlocked)
		}

		// TODO implement ip, mac, and temporary banning practices

		if a.State() != account.StateNotLoggedIn {
			return ErrorModel(AlreadyLoggedIn)
		} else if a.Password()[0] == uint8('$') && a.Password()[1] == uint8('2') && bcrypt.CompareHashAndPassword([]byte(a.Password()), []byte(password)) == nil {
			// TODO implement tos tracking
		} else {
			return ErrorModel(IncorrectPassword)
		}

		account.Login(l, db, span, tenant)(sessionId, a.Id(), account.ServiceLogin)

		l.Debugf("Login successful for [%s].", name)

		if !a.TOS() && tenant.Region != "JMS" {
			return ErrorModel(LicenseAgreement)
		}
		return OkModel()
	}
}

func ProgressState(l logrus.FieldLogger, db *gorm.DB, span opentracing.Span, tenant tenant.Model) func(sessionId uuid.UUID, issuer string, accountId uint32, state account.State) Model {
	return func(sessionId uuid.UUID, issuer string, accountId uint32, state account.State) Model {
		a, err := account.GetById(l, db, tenant)(accountId)
		if err != nil {
			l.WithError(err).Errorf("Unable to locate account a session is being created for.")
			return ErrorModel(NotRegistered)
		}

		l.Debugf("Received request to progress state for account [%d] to state [%d] from state [%d].", accountId, state, a.State())
		if a.State() == account.StateNotLoggedIn {
			return ErrorModel(SystemError)
		}
		if state == account.StateNotLoggedIn {
			account.Logout(l, db, span, tenant)(sessionId, accountId, issuer)
			return OkModel()
		}
		if state == account.StateLoggedIn {
			account.Login(l, db, span, tenant)(sessionId, accountId, issuer)
			return OkModel()
		}
		if state == account.StateTransition {
			err = account.Get().Transition(account.AccountKey{TenantId: tenant.Id, AccountId: accountId}, account.ServiceKey{SessionId: sessionId, Service: account.Service(issuer)})
			if err == nil {
				l.Debugf("State transition triggered a transition.")
			}
			return OkModel()
		}
		return ErrorModel(SystemError)
	}
}

func checkLoginAttempts(sessionId uuid.UUID) byte {
	return 0
}
