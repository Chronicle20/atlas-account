package account

import (
	"atlas-account/database"
	"atlas-account/kafka/producer"
	"atlas-account/tenant"
	"context"
	"errors"
	"go.opentelemetry.io/otel"
	"time"

	"github.com/Chronicle20/atlas-model/model"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type IdOperator func(tenant.Model, uint32) error

func byIdProvider(db *gorm.DB) func(tenant tenant.Model, id uint32) model.Provider[Model] {
	return func(tenant tenant.Model, id uint32) model.Provider[Model] {
		mp := database.ModelProvider[Model, entity](db)(entityById(tenant, id), modelFromEntity)
		return model.Map(mp, decorateState(tenant))
	}
}

func byNameProvider(db *gorm.DB) func(tenant tenant.Model, name string) model.Provider[[]Model] {
	return func(tenant tenant.Model, name string) model.Provider[[]Model] {
		mp := database.ModelSliceProvider[Model, entity](db)(entitiesByName(tenant, name), modelFromEntity)
		return model.SliceMap(mp, decorateState(tenant))
	}
}

func allByTenantProvider(db *gorm.DB) func(tenant tenant.Model) model.Provider[[]Model] {
	return func(tenant tenant.Model) model.Provider[[]Model] {
		mp := database.ModelSliceProvider[Model, entity](db)(allInTenant(tenant), modelFromEntity)
		return model.SliceMap(mp, decorateState(tenant))
	}
}

func allProvider(db *gorm.DB) model.Provider[[]Model] {
	tenants := Get().Tenants()
	mp := database.ModelSliceProvider[Model, entity](db)(allEntities, modelFromEntity)
	return model.SliceMap(mp, func(m Model) (Model, error) {
		if t, ok := tenants[m.TenantId()]; ok {
			return decorateState(t)(m)
		}
		return m, nil
	})
}

func decorateState(tenant tenant.Model) func(m Model) (Model, error) {
	return func(m Model) (Model, error) {
		st := Get().MaximalState(AccountKey{Tenant: tenant, AccountId: m.Id()})
		m.state = st
		return m, nil
	}
}

func GetById(l logrus.FieldLogger, db *gorm.DB, tenant tenant.Model) func(id uint32) (Model, error) {
	return func(id uint32) (Model, error) {
		m, err := byIdProvider(db)(tenant, id)()
		if err != nil {
			l.WithError(err).Errorf("Unable to retrieve account by id [%d].", id)
			return Model{}, err
		}
		return m, nil
	}
}

func GetByName(l logrus.FieldLogger, db *gorm.DB, tenant tenant.Model) func(name string) (Model, error) {
	return func(name string) (Model, error) {
		m, err := model.First[Model](byNameProvider(db)(tenant, name))
		if err != nil {
			l.WithError(err).Errorf("Unable to locate account with name [%s].", name)
			return Model{}, err
		}
		return m, nil
	}
}

func GetAll(l logrus.FieldLogger, db *gorm.DB, tenant tenant.Model) ([]Model, error) {
	return allByTenantProvider(db)(tenant)()
}

func GetInTransition(timeout time.Duration) ([]AccountKey, error) {
	return model.FixedProvider(Get().GetExpiredInTransition(timeout))()
}

func GetOrCreate(l logrus.FieldLogger, db *gorm.DB, ctx context.Context, tenant tenant.Model) func(name string, password string, automaticRegister bool) (Model, error) {
	return func(name string, password string, automaticRegister bool) (Model, error) {
		m, err := model.First[Model](byNameProvider(db)(tenant, name))
		if err == nil {
			return m, nil
		}

		if !automaticRegister {
			l.Errorf("Unable to locate account by name [%s], and automatic account creation is not enabled.", name)
			return Model{}, errors.New("account not found")
		}

		return Create(l, db, ctx, tenant)(name, password)
	}
}

func Create(l logrus.FieldLogger, db *gorm.DB, ctx context.Context, tenant tenant.Model) func(name string, password string) (Model, error) {
	return func(name string, password string) (Model, error) {
		l.Debugf("Attempting to create account [%s] with password [%s].", name, password)
		hashPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			l.WithError(err).Errorf("Error generating hash when creating account [%s].", name)
			return Model{}, err
		}

		gender := byte(0)
		if tenant.Region == "GMS" && tenant.MajorVersion > 83 {
			gender = byte(10)
		}
		l.Debugf("Defaulting gender to [%d]. 0 = Male, 1 = Female, 10 = UI Choose. This is determined by Region and Version capabilities.", gender)

		m, err := create(db)(tenant, name, string(hashPass), gender)
		if err != nil {
			l.WithError(err).Errorf("Unable to create account [%s].", name)
			return Model{}, err
		}
		l.Debugf("Created account [%d] for [%s].", m.Id(), m.Name())
		_ = producer.ProviderImpl(l)(ctx)(EnvEventTopicAccountStatus)(createdEventProvider()(tenant, m.Id(), name))
		return m, nil
	}
}

func Update(l logrus.FieldLogger, db *gorm.DB, tenant tenant.Model) func(accountId uint32, input Model) (Model, error) {
	return func(accountId uint32, input Model) (Model, error) {
		a, err := GetById(l, db, tenant)(accountId)
		if err != nil {
			l.WithError(err).Errorf("Unable to locate account being updated.")
			return Model{}, err
		}

		var modifiers = make([]EntityUpdateFunction, 0)

		if a.pin != input.pin && input.pin != "" {
			l.Debugf("Updating PIN [%s] of account [%d].", input.pin, accountId)
			modifiers = append(modifiers, updatePin(input.pin))
		}
		if a.pic != input.pic && input.pic != "" {
			l.Debugf("Updating PIC [%s] of account [%d].", input.pic, accountId)
			modifiers = append(modifiers, updatePic(input.pic))
		}
		if a.tos != input.tos && input.tos != false {
			l.Debugf("Updating TOS [%t] of account [%d].", input.tos, accountId)
			modifiers = append(modifiers, updateTos(input.tos))
		}
		if a.gender != input.gender {
			l.Debugf("Updating Gender [%d] of account [%d].", input.gender, accountId)
			modifiers = append(modifiers, updateGender(input.gender))
		}

		if len(modifiers) == 0 {
			return a, nil
		}

		err = update(db)(modifiers...)(tenant, accountId)
		if err != nil {
			l.WithError(err).Errorf("Unable to update account.")
			return Model{}, err
		}

		return GetById(l, db, tenant)(accountId)
	}
}

func Login(l logrus.FieldLogger, db *gorm.DB, ctx context.Context, tenant tenant.Model) func(sessionId uuid.UUID, accountId uint32, issuer string) error {
	return func(sessionId uuid.UUID, accountId uint32, issuer string) error {
		a, err := GetById(l, db, tenant)(accountId)
		if err != nil {
			return err
		}

		ak := AccountKey{Tenant: tenant, AccountId: accountId}
		sk := ServiceKey{SessionId: sessionId, Service: Service(issuer)}
		err = Get().Login(ak, sk)
		if err == nil {
			l.Debugf("State transition triggered a login.")
			err = producer.ProviderImpl(l)(ctx)(EnvEventTopicAccountStatus)(loggedInEventProvider()(tenant, a.Id(), a.Name()))
			return err
		}
		return err
	}
}

func Logout(l logrus.FieldLogger, db *gorm.DB, ctx context.Context, tenant tenant.Model) func(sessionId uuid.UUID, accountId uint32, issuer string) error {
	return func(sessionId uuid.UUID, accountId uint32, issuer string) error {
		a, err := GetById(l, db, tenant)(accountId)
		if err != nil {
			return err
		}

		ok := Get().Logout(AccountKey{Tenant: tenant, AccountId: accountId}, ServiceKey{SessionId: sessionId, Service: Service(issuer)})
		if ok {
			l.Debugf("State transition triggered a logout.")
			err = producer.ProviderImpl(l)(ctx)(EnvEventTopicAccountStatus)(loggedOutEventProvider()(tenant, a.Id(), a.Name()))
			return err
		}
		return errors.New("error while logging out")
	}
}

func Teardown(l logrus.FieldLogger, db *gorm.DB) func() {
	return func() {
		sctx, span := otel.GetTracerProvider().Tracer("atlas-account").Start(context.Background(), "teardown")
		defer span.End()

		tenants := Get().Tenants()

		logoutForModel := func(m Model) error {
			l.Debugf("Logging out [%d] [%s] on service shutdown.", m.Id(), m.Name())
			return producer.ProviderImpl(l)(sctx)(EnvEventTopicAccountStatus)(loggedOutEventProvider()(tenants[m.TenantId()], m.Id(), m.Name()))
		}

		lmp := model.FilteredProvider(allProvider(db), LoggedIn)
		err := model.ForEachSlice(lmp, logoutForModel, model.ParallelExecute())
		if err != nil {
			l.WithError(err).Errorf("Error destroying all monsters on teardown.")
		}
	}
}

func LoggedIn(m Model) bool {
	return m.state != StateNotLoggedIn
}
