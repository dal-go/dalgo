package dal

//
//import (
//	"context"
//	"testing"
//)
//
//func TestWithHooks2(t *testing.T) {
//	type args struct {
//		options []func(bag *Hooks)
//	}
//	tests := []struct {
//		name        string
//		args        args
//		assertHooks func(tt *testing.T, hooks *Hooks)
//	}{
//		{
//			name: "nil",
//			args: args{
//				options: nil,
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeGet != nil {
//					tt.Error("beforeGet is not nil")
//				}
//			},
//		},
//		{
//			name: "beforeGet",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeGet(func(ctx context.Context, record Record) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeGet == nil {
//					tt.Error("beforeGet is nil")
//				}
//			},
//		},
//		{
//			name: "afterGet",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterGet(func(ctx context.Context, record Record, err error) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterGet == nil {
//					tt.Error("afterGet is nil")
//				}
//			},
//		},
//		{
//			name: "beforeGetMulti",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeGetMulti(func(ctx context.Context, records []Record) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeGetMulti == nil {
//					tt.Error("beforeGetMulti is nil")
//				}
//			},
//		},
//		{
//			name: "afterGetMulti",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterGetMulti(func(ctx context.Context, records []Record, err error) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterGetMulti == nil {
//					tt.Error("afterGetMulti is nil")
//				}
//			},
//		},
//		{
//			name: "beforeInsert",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeInsert(func(ctx context.Context, record Record) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeInsert == nil {
//					tt.Error("beforeInsert is nil")
//				}
//			},
//		},
//		{
//			name: "afterInsert",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterInsert(func(ctx context.Context, record Record, err error) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterInsert == nil {
//					tt.Error("afterInsert is nil")
//				}
//			},
//		},
//		{
//			name: "beforeSet",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeSet(func(ctx context.Context, record Record) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeSet == nil {
//					tt.Error("beforeSet is nil")
//				}
//			},
//		},
//		{
//			name: "afterSet",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterSet(func(ctx context.Context, record Record, err error) error { return nil }),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterSet == nil {
//					tt.Error("afterSet is nil")
//				}
//			},
//		},
//		{
//			name: "beforeUpdate",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeUpdate(func(ctx context.Context, key *Key, updates []update, preconditions ...Precondition) error {
//						return nil
//					}),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeUpdate == nil {
//					tt.Error("beforeUpdate is nil")
//				}
//			},
//		},
//		{
//			name: "afterUpdate",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterUpdate(func(ctx context.Context, key *Key, err error) error {
//						return nil
//					}),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterUpdate == nil {
//					tt.Error("afterUpdate is nil")
//				}
//			},
//		},
//		{
//			name: "beforeDelete",
//			args: args{
//				options: []func(bag *Hooks){
//					BeforeDelete(func(ctx context.Context, key *Key) error {
//						return nil
//					}),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.beforeDelete == nil {
//					tt.Error("beforeDelete is nil")
//				}
//			},
//		},
//		{
//			name: "afterDelete",
//			args: args{
//				options: []func(bag *Hooks){
//					AfterDelete(func(ctx context.Context, key *Key, err error) error {
//						return nil
//					}),
//				},
//			},
//			assertHooks: func(tt *testing.T, hooks *Hooks) {
//				if hooks.afterDelete == nil {
//					tt.Error("afterDelete is nil")
//				}
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			hooks := WithHooks(tt.args.options...)
//			tt.assertHooks(t, hooks)
//		})
//	}
//}
