package account

import (
	"github.com/google/uuid"
	"time"
)

type Model struct {
	tenantId  uuid.UUID
	id        uint32
	name      string
	password  string
	pin       string
	pic       string
	state     State
	gender    byte
	banned    bool
	tos       bool
	updatedAt time.Time
}

func (a Model) Id() uint32 {
	return a.id
}

func (a Model) Name() string {
	return a.name
}

func (a Model) Password() string {
	return a.password
}

func (a Model) Banned() bool {
	return a.banned
}

func (a Model) State() State {
	return a.state
}

func (a Model) TOS() bool {
	return a.tos
}

func (a Model) UpdatedAt() time.Time {
	return a.updatedAt
}

func (a Model) TenantId() uuid.UUID {
	return a.tenantId
}
