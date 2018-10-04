// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import context "context"
import mock "github.com/stretchr/testify/mock"
import rs "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/rs"

// RulesController is an autogenerated mock type for the RulesController type
type RulesController struct {
	mock.Mock
}

// Reconcile provides a mock function with given fields: _a0, _a1
func (_m *RulesController) Reconcile(_a0 context.Context, _a1 *rs.Rules) error {
	ret := _m.Called(_a0, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *rs.Rules) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}