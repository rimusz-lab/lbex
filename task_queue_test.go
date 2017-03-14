/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var sr uint32

type mockEnqueueObj struct {
	k string
	v string
}

func mockSynFn(interface{}) error {
	// sr will be plus one times after enqueue
	atomic.AddUint32(&sr, 1)
	return nil
}

func mockKeyFn(interface{}) (interface{}, error) {
	return mockEnqueueObj{
		k: "static_key",
		v: "static_value",
	}, nil
}

func mockErrorKeyFn(interface{}) (interface{}, error) {
	return nil, fmt.Errorf("failed to get key")
}

func TestShutdown(t *testing.T) {
	q := newTaskQueue(mockSynFn)
	stopCh := make(chan struct{})
	// run queue
	go q.run(10*time.Second, stopCh)
	q.shutdown()
	s := q.isShuttingDown()
	if !s {
		t.Errorf("queue should be shutdown")
	}
}

func TestEnqueueSuccess(t *testing.T) {
	// initialize result
	atomic.StoreUint32(&sr, 0)
	q := newCustomTaskQueue(mockSynFn, mockKeyFn)
	stopCh := make(chan struct{})
	// run queue
	go q.run(10*time.Second, stopCh)
	// mock object whichi will be enqueue
	mo := mockEnqueueObj{
		k: "testKey",
		v: "testValue",
	}
	q.enqueue(mo)
	// wait for 'mockSynFn'
	time.Sleep(time.Millisecond * 10)
	if atomic.LoadUint32(&sr) != 1 {
		t.Errorf("sr should be 1, but is %d", sr)
	}

	// shutdown queue before exit
	q.shutdown()
}

func TestEnqueueFailed(t *testing.T) {
	// initialize result
	atomic.StoreUint32(&sr, 0)
	q := newCustomTaskQueue(mockSynFn, mockKeyFn)
	stopCh := make(chan struct{})
	// run queue
	go q.run(10*time.Second, stopCh)
	// mock object whichi will be enqueue
	mo := mockEnqueueObj{
		k: "testKey",
		v: "testValue",
	}

	// shutdown queue before enqueue
	q.shutdown()
	// wait for shutdown
	time.Sleep(time.Millisecond * 10)
	q.enqueue(mo)
	// wait for 'mockSynFn'
	time.Sleep(time.Millisecond * 10)
	// queue is shutdown, so mockSynFn should not be executed, so the result should be 0
	if atomic.LoadUint32(&sr) != 0 {
		t.Errorf("queue has been shutdown, so sr should be 0, but is %d", sr)
	}
}

func TestEnqueueKeyError(t *testing.T) {
	// initialize result
	atomic.StoreUint32(&sr, 0)
	q := newCustomTaskQueue(mockSynFn, mockErrorKeyFn)
	stopCh := make(chan struct{})
	// run queue
	go q.run(10*time.Second, stopCh)
	// mock object whichi will be enqueue
	mo := mockEnqueueObj{
		k: "testKey",
		v: "testValue",
	}

	q.enqueue(mo)
	// wait for 'mockSynFn'
	time.Sleep(time.Millisecond * 10)
	// key error, so the result should be 0
	if atomic.LoadUint32(&sr) != 0 {
		t.Errorf("error occurs while get key, so sr should be 0, but is %d", sr)
	}
	// shutdown queue before exit
	q.shutdown()
}