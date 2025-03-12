// Code generated by counterfeiter. DO NOT EDIT.
package fakes

import (
	"sync"

	"code.cloudfoundry.org/gorouter/metrics"
)

type FakeMonitorReporter struct {
	CaptureFoundFileDescriptorsStub        func(int)
	captureFoundFileDescriptorsMutex       sync.RWMutex
	captureFoundFileDescriptorsArgsForCall []struct {
		arg1 int
	}
	CaptureNATSBufferedMessagesStub        func(int)
	captureNATSBufferedMessagesMutex       sync.RWMutex
	captureNATSBufferedMessagesArgsForCall []struct {
		arg1 int
	}
	CaptureNATSDroppedMessagesStub        func(int)
	captureNATSDroppedMessagesMutex       sync.RWMutex
	captureNATSDroppedMessagesArgsForCall []struct {
		arg1 int
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeMonitorReporter) CaptureFoundFileDescriptors(arg1 int) {
	fake.captureFoundFileDescriptorsMutex.Lock()
	fake.captureFoundFileDescriptorsArgsForCall = append(fake.captureFoundFileDescriptorsArgsForCall, struct {
		arg1 int
	}{arg1})
	stub := fake.CaptureFoundFileDescriptorsStub
	fake.recordInvocation("CaptureFoundFileDescriptors", []interface{}{arg1})
	fake.captureFoundFileDescriptorsMutex.Unlock()
	if stub != nil {
		fake.CaptureFoundFileDescriptorsStub(arg1)
	}
}

func (fake *FakeMonitorReporter) CaptureFoundFileDescriptorsCallCount() int {
	fake.captureFoundFileDescriptorsMutex.RLock()
	defer fake.captureFoundFileDescriptorsMutex.RUnlock()
	return len(fake.captureFoundFileDescriptorsArgsForCall)
}

func (fake *FakeMonitorReporter) CaptureFoundFileDescriptorsCalls(stub func(int)) {
	fake.captureFoundFileDescriptorsMutex.Lock()
	defer fake.captureFoundFileDescriptorsMutex.Unlock()
	fake.CaptureFoundFileDescriptorsStub = stub
}

func (fake *FakeMonitorReporter) CaptureFoundFileDescriptorsArgsForCall(i int) int {
	fake.captureFoundFileDescriptorsMutex.RLock()
	defer fake.captureFoundFileDescriptorsMutex.RUnlock()
	argsForCall := fake.captureFoundFileDescriptorsArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeMonitorReporter) CaptureNATSBufferedMessages(arg1 int) {
	fake.captureNATSBufferedMessagesMutex.Lock()
	fake.captureNATSBufferedMessagesArgsForCall = append(fake.captureNATSBufferedMessagesArgsForCall, struct {
		arg1 int
	}{arg1})
	stub := fake.CaptureNATSBufferedMessagesStub
	fake.recordInvocation("CaptureNATSBufferedMessages", []interface{}{arg1})
	fake.captureNATSBufferedMessagesMutex.Unlock()
	if stub != nil {
		fake.CaptureNATSBufferedMessagesStub(arg1)
	}
}

func (fake *FakeMonitorReporter) CaptureNATSBufferedMessagesCallCount() int {
	fake.captureNATSBufferedMessagesMutex.RLock()
	defer fake.captureNATSBufferedMessagesMutex.RUnlock()
	return len(fake.captureNATSBufferedMessagesArgsForCall)
}

func (fake *FakeMonitorReporter) CaptureNATSBufferedMessagesCalls(stub func(int)) {
	fake.captureNATSBufferedMessagesMutex.Lock()
	defer fake.captureNATSBufferedMessagesMutex.Unlock()
	fake.CaptureNATSBufferedMessagesStub = stub
}

func (fake *FakeMonitorReporter) CaptureNATSBufferedMessagesArgsForCall(i int) int {
	fake.captureNATSBufferedMessagesMutex.RLock()
	defer fake.captureNATSBufferedMessagesMutex.RUnlock()
	argsForCall := fake.captureNATSBufferedMessagesArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeMonitorReporter) CaptureNATSDroppedMessages(arg1 int) {
	fake.captureNATSDroppedMessagesMutex.Lock()
	fake.captureNATSDroppedMessagesArgsForCall = append(fake.captureNATSDroppedMessagesArgsForCall, struct {
		arg1 int
	}{arg1})
	stub := fake.CaptureNATSDroppedMessagesStub
	fake.recordInvocation("CaptureNATSDroppedMessages", []interface{}{arg1})
	fake.captureNATSDroppedMessagesMutex.Unlock()
	if stub != nil {
		fake.CaptureNATSDroppedMessagesStub(arg1)
	}
}

func (fake *FakeMonitorReporter) CaptureNATSDroppedMessagesCallCount() int {
	fake.captureNATSDroppedMessagesMutex.RLock()
	defer fake.captureNATSDroppedMessagesMutex.RUnlock()
	return len(fake.captureNATSDroppedMessagesArgsForCall)
}

func (fake *FakeMonitorReporter) CaptureNATSDroppedMessagesCalls(stub func(int)) {
	fake.captureNATSDroppedMessagesMutex.Lock()
	defer fake.captureNATSDroppedMessagesMutex.Unlock()
	fake.CaptureNATSDroppedMessagesStub = stub
}

func (fake *FakeMonitorReporter) CaptureNATSDroppedMessagesArgsForCall(i int) int {
	fake.captureNATSDroppedMessagesMutex.RLock()
	defer fake.captureNATSDroppedMessagesMutex.RUnlock()
	argsForCall := fake.captureNATSDroppedMessagesArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeMonitorReporter) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.captureFoundFileDescriptorsMutex.RLock()
	defer fake.captureFoundFileDescriptorsMutex.RUnlock()
	fake.captureNATSBufferedMessagesMutex.RLock()
	defer fake.captureNATSBufferedMessagesMutex.RUnlock()
	fake.captureNATSDroppedMessagesMutex.RLock()
	defer fake.captureNATSDroppedMessagesMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeMonitorReporter) recordInvocation(key string, args []interface{}) {
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

var _ metrics.MonitorReporter = new(FakeMonitorReporter)
