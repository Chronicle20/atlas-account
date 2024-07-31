package account

import (
	"atlas-account/tenant"
	"gorm.io/gorm"
)

type EntityUpdateFunction func() ([]string, func(e *entity))

func create(db *gorm.DB) func(tenant tenant.Model, name string, password string, gender byte) (Model, error) {
	return func(tenant tenant.Model, name string, password string, gender byte) (Model, error) {
		a := &entity{
			TenantId: tenant.Id,
			Name:     name,
			Password: password,
			Gender:   gender,
		}

		err := db.Create(a).Error
		if err != nil {
			return Model{}, err
		}

		return modelFromEntity(*a)
	}
}

func update(db *gorm.DB) func(modifiers ...EntityUpdateFunction) IdOperator {
	return func(modifiers ...EntityUpdateFunction) IdOperator {
		return func(tenant tenant.Model, id uint32) error {
			e := &entity{}
			var columns []string
			for _, modifier := range modifiers {
				c, u := modifier()
				columns = append(columns, c...)
				u(e)
			}
			return db.Model(&entity{TenantId: tenant.Id, ID: id}).Select(columns).Updates(e).Error

		}
	}
}

func updatePic(pic string) EntityUpdateFunction {
	return func() ([]string, func(e *entity)) {
		var cs = []string{"pic"}

		uf := func(e *entity) {
			e.PIC = pic
		}
		return cs, uf
	}
}

func updatePin(pin string) EntityUpdateFunction {
	return func() ([]string, func(e *entity)) {
		var cs = []string{"pin"}

		uf := func(e *entity) {
			e.PIN = pin
		}
		return cs, uf
	}
}

func updateTos(tos bool) EntityUpdateFunction {
	return func() ([]string, func(e *entity)) {
		var cs = []string{"tos"}

		uf := func(e *entity) {
			e.TOS = tos
		}
		return cs, uf
	}
}

func updateGender(gender byte) EntityUpdateFunction {
	return func() ([]string, func(e *entity)) {
		var cs = []string{"gender"}

		uf := func(e *entity) {
			e.Gender = gender
		}
		return cs, uf
	}
}

func modelFromEntity(a entity) (Model, error) {
	r := Model{
		tenantId:  a.TenantId,
		id:        a.ID,
		name:      a.Name,
		password:  a.Password,
		pin:       a.PIN,
		pic:       a.PIC,
		gender:    a.Gender,
		banned:    false,
		tos:       a.TOS,
		updatedAt: a.UpdatedAt,
	}
	return r, nil
}
