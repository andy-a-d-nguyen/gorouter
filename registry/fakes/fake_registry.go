// Code generated by counterfeiter. DO NOT EDIT.
package fakes

import (
	"sync"

	"code.cloudfoundry.org/gorouter/registry"
	"code.cloudfoundry.org/gorouter/route"
)

type FakeRegistry struct {
	LookupStub        func(route.Uri) *route.EndpointPool
	lookupMutex       sync.RWMutex
	lookupArgsForCall []struct {
		arg1 route.Uri
	}
	lookupReturns struct {
		result1 *route.EndpointPool
	}
	lookupReturnsOnCall map[int]struct {
		result1 *route.EndpointPool
	}
	LookupWithAppInstanceStub        func(route.Uri, string, string) *route.EndpointPool
	lookupWithAppInstanceMutex       sync.RWMutex
	lookupWithAppInstanceArgsForCall []struct {
		arg1 route.Uri
		arg2 string
		arg3 string
	}
	lookupWithAppInstanceReturns struct {
		result1 *route.EndpointPool
	}
	lookupWithAppInstanceReturnsOnCall map[int]struct {
		result1 *route.EndpointPool
	}
	LookupWithProcessInstanceStub        func(route.Uri, string, string) *route.EndpointPool
	lookupWithProcessInstanceMutex       sync.RWMutex
	lookupWithProcessInstanceArgsForCall []struct {
		arg1 route.Uri
		arg2 string
		arg3 string
	}
	lookupWithProcessInstanceReturns struct {
		result1 *route.EndpointPool
	}
	lookupWithProcessInstanceReturnsOnCall map[int]struct {
		result1 *route.EndpointPool
	}
	RegisterStub        func(route.Uri, *route.Endpoint)
	registerMutex       sync.RWMutex
	registerArgsForCall []struct {
		arg1 route.Uri
		arg2 *route.Endpoint
	}
	UnregisterStub        func(route.Uri, *route.Endpoint)
	unregisterMutex       sync.RWMutex
	unregisterArgsForCall []struct {
		arg1 route.Uri
		arg2 *route.Endpoint
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeRegistry) Lookup(arg1 route.Uri) *route.EndpointPool {
	fake.lookupMutex.Lock()
	ret, specificReturn := fake.lookupReturnsOnCall[len(fake.lookupArgsForCall)]
	fake.lookupArgsForCall = append(fake.lookupArgsForCall, struct {
		arg1 route.Uri
	}{arg1})
	stub := fake.LookupStub
	fakeReturns := fake.lookupReturns
	fake.recordInvocation("Lookup", []interface{}{arg1})
	fake.lookupMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *FakeRegistry) LookupCallCount() int {
	fake.lookupMutex.RLock()
	defer fake.lookupMutex.RUnlock()
	return len(fake.lookupArgsForCall)
}

func (fake *FakeRegistry) LookupCalls(stub func(route.Uri) *route.EndpointPool) {
	fake.lookupMutex.Lock()
	defer fake.lookupMutex.Unlock()
	fake.LookupStub = stub
}

func (fake *FakeRegistry) LookupArgsForCall(i int) route.Uri {
	fake.lookupMutex.RLock()
	defer fake.lookupMutex.RUnlock()
	argsForCall := fake.lookupArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeRegistry) LookupReturns(result1 *route.EndpointPool) {
	fake.lookupMutex.Lock()
	defer fake.lookupMutex.Unlock()
	fake.LookupStub = nil
	fake.lookupReturns = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) LookupReturnsOnCall(i int, result1 *route.EndpointPool) {
	fake.lookupMutex.Lock()
	defer fake.lookupMutex.Unlock()
	fake.LookupStub = nil
	if fake.lookupReturnsOnCall == nil {
		fake.lookupReturnsOnCall = make(map[int]struct {
			result1 *route.EndpointPool
		})
	}
	fake.lookupReturnsOnCall[i] = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) LookupWithAppInstance(arg1 route.Uri, arg2 string, arg3 string) *route.EndpointPool {
	fake.lookupWithAppInstanceMutex.Lock()
	ret, specificReturn := fake.lookupWithAppInstanceReturnsOnCall[len(fake.lookupWithAppInstanceArgsForCall)]
	fake.lookupWithAppInstanceArgsForCall = append(fake.lookupWithAppInstanceArgsForCall, struct {
		arg1 route.Uri
		arg2 string
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.LookupWithAppInstanceStub
	fakeReturns := fake.lookupWithAppInstanceReturns
	fake.recordInvocation("LookupWithAppInstance", []interface{}{arg1, arg2, arg3})
	fake.lookupWithAppInstanceMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *FakeRegistry) LookupWithAppInstanceCallCount() int {
	fake.lookupWithAppInstanceMutex.RLock()
	defer fake.lookupWithAppInstanceMutex.RUnlock()
	return len(fake.lookupWithAppInstanceArgsForCall)
}

func (fake *FakeRegistry) LookupWithAppInstanceCalls(stub func(route.Uri, string, string) *route.EndpointPool) {
	fake.lookupWithAppInstanceMutex.Lock()
	defer fake.lookupWithAppInstanceMutex.Unlock()
	fake.LookupWithAppInstanceStub = stub
}

func (fake *FakeRegistry) LookupWithAppInstanceArgsForCall(i int) (route.Uri, string, string) {
	fake.lookupWithAppInstanceMutex.RLock()
	defer fake.lookupWithAppInstanceMutex.RUnlock()
	argsForCall := fake.lookupWithAppInstanceArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *FakeRegistry) LookupWithAppInstanceReturns(result1 *route.EndpointPool) {
	fake.lookupWithAppInstanceMutex.Lock()
	defer fake.lookupWithAppInstanceMutex.Unlock()
	fake.LookupWithAppInstanceStub = nil
	fake.lookupWithAppInstanceReturns = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) LookupWithAppInstanceReturnsOnCall(i int, result1 *route.EndpointPool) {
	fake.lookupWithAppInstanceMutex.Lock()
	defer fake.lookupWithAppInstanceMutex.Unlock()
	fake.LookupWithAppInstanceStub = nil
	if fake.lookupWithAppInstanceReturnsOnCall == nil {
		fake.lookupWithAppInstanceReturnsOnCall = make(map[int]struct {
			result1 *route.EndpointPool
		})
	}
	fake.lookupWithAppInstanceReturnsOnCall[i] = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) LookupWithProcessInstance(arg1 route.Uri, arg2 string, arg3 string) *route.EndpointPool {
	fake.lookupWithProcessInstanceMutex.Lock()
	ret, specificReturn := fake.lookupWithProcessInstanceReturnsOnCall[len(fake.lookupWithProcessInstanceArgsForCall)]
	fake.lookupWithProcessInstanceArgsForCall = append(fake.lookupWithProcessInstanceArgsForCall, struct {
		arg1 route.Uri
		arg2 string
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.LookupWithProcessInstanceStub
	fakeReturns := fake.lookupWithProcessInstanceReturns
	fake.recordInvocation("LookupWithProcessInstance", []interface{}{arg1, arg2, arg3})
	fake.lookupWithProcessInstanceMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *FakeRegistry) LookupWithProcessInstanceCallCount() int {
	fake.lookupWithProcessInstanceMutex.RLock()
	defer fake.lookupWithProcessInstanceMutex.RUnlock()
	return len(fake.lookupWithProcessInstanceArgsForCall)
}

func (fake *FakeRegistry) LookupWithProcessInstanceCalls(stub func(route.Uri, string, string) *route.EndpointPool) {
	fake.lookupWithProcessInstanceMutex.Lock()
	defer fake.lookupWithProcessInstanceMutex.Unlock()
	fake.LookupWithProcessInstanceStub = stub
}

func (fake *FakeRegistry) LookupWithProcessInstanceArgsForCall(i int) (route.Uri, string, string) {
	fake.lookupWithProcessInstanceMutex.RLock()
	defer fake.lookupWithProcessInstanceMutex.RUnlock()
	argsForCall := fake.lookupWithProcessInstanceArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *FakeRegistry) LookupWithProcessInstanceReturns(result1 *route.EndpointPool) {
	fake.lookupWithProcessInstanceMutex.Lock()
	defer fake.lookupWithProcessInstanceMutex.Unlock()
	fake.LookupWithProcessInstanceStub = nil
	fake.lookupWithProcessInstanceReturns = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) LookupWithProcessInstanceReturnsOnCall(i int, result1 *route.EndpointPool) {
	fake.lookupWithProcessInstanceMutex.Lock()
	defer fake.lookupWithProcessInstanceMutex.Unlock()
	fake.LookupWithProcessInstanceStub = nil
	if fake.lookupWithProcessInstanceReturnsOnCall == nil {
		fake.lookupWithProcessInstanceReturnsOnCall = make(map[int]struct {
			result1 *route.EndpointPool
		})
	}
	fake.lookupWithProcessInstanceReturnsOnCall[i] = struct {
		result1 *route.EndpointPool
	}{result1}
}

func (fake *FakeRegistry) Register(arg1 route.Uri, arg2 *route.Endpoint) {
	fake.registerMutex.Lock()
	fake.registerArgsForCall = append(fake.registerArgsForCall, struct {
		arg1 route.Uri
		arg2 *route.Endpoint
	}{arg1, arg2})
	stub := fake.RegisterStub
	fake.recordInvocation("Register", []interface{}{arg1, arg2})
	fake.registerMutex.Unlock()
	if stub != nil {
		fake.RegisterStub(arg1, arg2)
	}
}

func (fake *FakeRegistry) RegisterCallCount() int {
	fake.registerMutex.RLock()
	defer fake.registerMutex.RUnlock()
	return len(fake.registerArgsForCall)
}

func (fake *FakeRegistry) RegisterCalls(stub func(route.Uri, *route.Endpoint)) {
	fake.registerMutex.Lock()
	defer fake.registerMutex.Unlock()
	fake.RegisterStub = stub
}

func (fake *FakeRegistry) RegisterArgsForCall(i int) (route.Uri, *route.Endpoint) {
	fake.registerMutex.RLock()
	defer fake.registerMutex.RUnlock()
	argsForCall := fake.registerArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *FakeRegistry) Unregister(arg1 route.Uri, arg2 *route.Endpoint) {
	fake.unregisterMutex.Lock()
	fake.unregisterArgsForCall = append(fake.unregisterArgsForCall, struct {
		arg1 route.Uri
		arg2 *route.Endpoint
	}{arg1, arg2})
	stub := fake.UnregisterStub
	fake.recordInvocation("Unregister", []interface{}{arg1, arg2})
	fake.unregisterMutex.Unlock()
	if stub != nil {
		fake.UnregisterStub(arg1, arg2)
	}
}

func (fake *FakeRegistry) UnregisterCallCount() int {
	fake.unregisterMutex.RLock()
	defer fake.unregisterMutex.RUnlock()
	return len(fake.unregisterArgsForCall)
}

func (fake *FakeRegistry) UnregisterCalls(stub func(route.Uri, *route.Endpoint)) {
	fake.unregisterMutex.Lock()
	defer fake.unregisterMutex.Unlock()
	fake.UnregisterStub = stub
}

func (fake *FakeRegistry) UnregisterArgsForCall(i int) (route.Uri, *route.Endpoint) {
	fake.unregisterMutex.RLock()
	defer fake.unregisterMutex.RUnlock()
	argsForCall := fake.unregisterArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *FakeRegistry) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.lookupMutex.RLock()
	defer fake.lookupMutex.RUnlock()
	fake.lookupWithAppInstanceMutex.RLock()
	defer fake.lookupWithAppInstanceMutex.RUnlock()
	fake.lookupWithProcessInstanceMutex.RLock()
	defer fake.lookupWithProcessInstanceMutex.RUnlock()
	fake.registerMutex.RLock()
	defer fake.registerMutex.RUnlock()
	fake.unregisterMutex.RLock()
	defer fake.unregisterMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeRegistry) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ registry.Registry = new(FakeRegistry)
