package database

import (
	"gorm.io/gorm"
)

type UserQuery struct {
	Query[UserInfo]
	id    string
	scope string
}

func NewUserQuery(db *gorm.DB) *UserQuery {
	return &UserQuery{
		Query: Query[UserInfo]{
			db:     db,
			limit:  100,
			offset: 0,
			order:  "login DESC",
		},
	}
}

func (q *UserQuery) Order(s string) *UserQuery {
	q.order = s
	return q
}

func (q *UserQuery) Limit(n int) *UserQuery {
	q.limit = n
	return q
}

func (q *UserQuery) Offset(n int) *UserQuery {

	q.offset = n
	return q
}

func (q *UserQuery) ID(id string) *UserQuery {
	q.id = id
	return q
}

func (q *UserQuery) Scope(scope string) *UserQuery {
	q.scope = scope
	return q
}

func (q *UserQuery) where() *gorm.DB {
	tx := q.db

	if q.id != "" {
		tx = tx.Where("id = ?", q.id)
	}

	if q.scope != "" {
		tx = tx.Where("scope = ?", q.scope)
	}

	return tx
}

func (q *UserQuery) Get() []*UserInfo {
	return q.get(q.where().Model(&UserInfo{}))
}

func (q *UserQuery) One() *UserInfo {
	return q.one(q.where().Model(&UserInfo{}))
}

func (q *UserQuery) Count() int64 {
	return q.count(q.where().Model(&UserInfo{}))
}

func (q *UserQuery) Update(updates map[string]any) error {
	return q.updateOrError(q.where().Model(&UserInfo{}), updates)
}
