package demo

import (
	"context"
	"fmt"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/orm"
	"github.com/dal-go/dalgo/query"
)

type user struct {
	Email     orm.StringField
	FirstName orm.StringField
	LastName  orm.StringField
}

// User defines user collection
var User = user{
	FirstName: orm.NewStringField("fist_name"),
	LastName:  orm.NewStringField("last_name"),
	Email:     orm.NewStringField("email"),
}

func (v user) Collection() *dal.CollectionRef {
	return &dal.CollectionRef{
		Name: "users",
	}
}

type userData struct {
	Email string `json:"email"`
}

// SelectUserByEmail is a demo facade method
func SelectUserByEmail(ctx context.Context, db dal.ReadSession, email string, into interface{}) error {
	if db == nil {
		panic("db is a required parameter")
	}
	if into == nil {
		into = &userData{}
	}
	q := dal.Select{
		From:  User.Collection(),
		Where: User.Email.EqualToString(email),
		Into: func() interface{} {
			return into
		},
		Columns: query.Columns(User.FirstName.Name(), User.LastName.Name()),
		Limit:   1,
	}
	fmt.Print(q)
	reader, err := db.Select(ctx, q)
	if err != nil {
		return err
	}
	if reader == nil {
		panic("db.Select() returned no error and nil reader")
	}
	_, err = reader.Next()
	if err != nil {
		return err
	}
	return nil
}
