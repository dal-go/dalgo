package demo

import (
	"context"
	"fmt"
	"github.com/strongo/dalgo/dal"
	"github.com/strongo/dalgo/orm"
)

type user struct {
	Email     orm.StringField
	FirstName orm.StringField
	LastName  orm.StringField
}

func (v user) Collection() dal.CollectionRef {
	return dal.CollectionRef{
		Name: "users",
	}
}

var User = user{
	FirstName: orm.NewStringField("fist_name"),
	LastName:  orm.NewStringField("last_name"),
}

func SelectUserByEmail(ctx context.Context, db dal.Database, email string) {
	q := dal.Select{
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
