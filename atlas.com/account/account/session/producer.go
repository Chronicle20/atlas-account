package session

import (
	"atlas-account/tenant"
	"github.com/Chronicle20/atlas-kafka/producer"
	"github.com/Chronicle20/atlas-model/model"
	"github.com/segmentio/kafka-go"
)

func logoutCommandProvider(tenant tenant.Model, accountId uint32) model.Provider[[]kafka.Message] {
	key := producer.CreateKey(int(accountId))
	value := &logoutCommand{
		Tenant:    tenant,
		AccountId: accountId,
	}
	return producer.SingleMessageProvider(key, value)
}

func loggedInEventProvider() func(tenant tenant.Model, accountId uint32, name string) model.Provider[[]kafka.Message] {
	return accountStatusEventProvider(EventAccountStatusLoggedIn)
}

func loggedOutEventProvider() func(tenant tenant.Model, accountId uint32, name string) model.Provider[[]kafka.Message] {
	return accountStatusEventProvider(EventAccountStatusLoggedOut)
}

func accountStatusEventProvider(status string) func(tenant tenant.Model, accountId uint32, name string) model.Provider[[]kafka.Message] {
	return func(tenant tenant.Model, accountId uint32, name string) model.Provider[[]kafka.Message] {
		key := producer.CreateKey(int(accountId))
		value := &statusEvent{
			Tenant:    tenant,
			AccountId: accountId,
			Name:      name,
			Status:    status,
		}
		return producer.SingleMessageProvider(key, value)
	}
}
