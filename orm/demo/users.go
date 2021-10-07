package demo

import (
	"context"
	"fmt"
	"github.com/strongo/dalgo"
	"github.com/strongo/dalgo/orm"
)

type user struct {
	Email     orm.StringField
	FirstName orm.StringField
	LastName  orm.StringField
}

func (v user) Collection() dalgo.CollectionRef {
	return dalgo.CollectionRef{
		Name: "users",
	}
}

var User = user{
	FirstName: orm.NewStringField("fist_name"),
	LastName:  orm.NewStringField("last_name"),
}

func SelectUserByEmail(ctx context.Context, db dalgo.Database, email string) {
	q := dalgo.Select{
		From:  User.Collection(),
		Where: User.Email.EqualToString(email),
	}
	fmt.Print(q)
	_, err := db.Select(ctx, q)
	if err != nil {
		fmt.Print(err)
		return
	}
}
