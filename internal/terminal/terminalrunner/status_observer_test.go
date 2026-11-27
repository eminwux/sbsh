// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package terminalrunner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eminwux/sbsh/pkg/api"
)

func newObservedExec(t *testing.T, fn func(api.TerminalDoc)) *Exec {
	t.Helper()
	spec := &api.TerminalSpec{ID: "term-obs", MetadataDir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var opts []ExecOption
	if fn != nil {
		opts = append(opts, WithStatusObserver(fn))
	}
	sr, ok := NewTerminalRunnerExecWithOptions(ctx, newDiscardLogger(), spec, opts...).(*Exec)
	if !ok {
		t.Fatal("NewTerminalRunnerExecWithOptions did not return *Exec")
	}
	t.Cleanup(sr.closeStatusObserver)
	return sr
}

// recorder collects observer calls and can hold the observer busy.
type recorder struct {
	mu      sync.Mutex
	docs    []api.TerminalDoc
	entered chan api.TerminalStatusMode
	release chan struct{}
}

func newRecorder(blocking bool) *recorder {
	r := &recorder{entered: make(chan api.TerminalStatusMode, 64)}
	if blocking {
		r.release = make(chan struct{})
	}
	return r
}

func (r *recorder) observe(doc api.TerminalDoc) {
	r.mu.Lock()
	r.docs = append(r.docs, doc)
	r.mu.Unlock()
	r.entered <- doc.Status.State
	if r.release != nil {
		<-r.release
	}
}

func (r *recorder) states() []api.TerminalStatusMode {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]api.TerminalStatusMode, 0, len(r.docs))
	for _, d := range r.docs {
		out = append(out, d.Status.State)
	}
	return out
}

func waitEntered(t *testing.T, r *recorder, want api.TerminalStatusMode) {
	t.Helper()
	select {
	case got := <-r.entered:
		if got != want {
			t.Fatalf("observer called with %v, want %v (all: %v)", got, want, r.states())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("observer not called with %v (all: %v)", want, r.states())
	}
}

func TestStatusObserver_NoOptionNoDispatcher(t *testing.T) {
	sr := newObservedExec(t, nil)
	if sr.statusCh != nil {
		t.Fatal("statusCh allocated without an observer")
	}
	sr.notifyStatus() // must not block or panic
	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatalf("updateTerminalState: %v", err)
	}
}

// TestStatusObserver_InOrderAndCoalescedWhileBusy: while the observer is
// busy with one document, later posts replace each other and only the
// newest is delivered next.
func TestStatusObserver_InOrderAndCoalescedWhileBusy(t *testing.T) {
	r := newRecorder(true)
	sr := newObservedExec(t, r.observe)

	if err := sr.updateTerminalState(api.Starting); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Starting) // observer now busy

	for _, s := range []api.TerminalStatusMode{api.SettingUp, api.OnInit, api.Ready} {
		if err := sr.updateTerminalState(s); err != nil {
			t.Fatal(err)
		}
	}
	r.release <- struct{}{} // finish Starting
	waitEntered(t, r, api.Ready)
	r.release <- struct{}{}

	got := r.states()
	if len(got) != 2 || got[0] != api.Starting || got[1] != api.Ready {
		t.Fatalf("observed %v, want [Starting Ready] (SettingUp/OnInit superseded)", got)
	}
}

// TestStatusObserver_UnblockedObserverSeesEachTransition steps the runner
// one transition at a time and asserts each is delivered, in order.
func TestStatusObserver_UnblockedObserverSeesEachTransition(t *testing.T) {
	r := newRecorder(false)
	sr := newObservedExec(t, r.observe)
	seq := []api.TerminalStatusMode{api.Initializing, api.Starting, api.SettingUp, api.OnInit, api.Ready}
	for _, s := range seq {
		if err := sr.updateTerminalState(s); err != nil {
			t.Fatal(err)
		}
		waitEntered(t, r, s)
	}
	if err := sr.markExited(); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Exited)
}

// TestStatusObserver_AttachDetachCalls asserts one call per attacher change.
func TestStatusObserver_AttachDetachCalls(t *testing.T) {
	r := newRecorder(false)
	sr := newObservedExec(t, r.observe)
	cid := api.ID("c1")
	sr.clients[cid] = &ioClient{id: &cid}
	if err := sr.updateTerminalAttachers(); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Initializing)
	delete(sr.clients, cid)
	if err := sr.updateTerminalAttachers(); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Initializing)

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.docs) != 2 || len(r.docs[0].Status.Attachers) != 1 || len(r.docs[1].Status.Attachers) != 0 {
		t.Fatalf("attacher calls = %d (%+v), want attach then detach", len(r.docs), r.docs)
	}
}

// TestStatusObserver_SleepingObserverDoesNotBlock asserts posts return at
// once while the observer sleeps, and that closing does not wait for it.
func TestStatusObserver_SleepingObserverDoesNotBlock(t *testing.T) {
	var calls sync.WaitGroup
	calls.Add(1)
	var once sync.Once
	sr := newObservedExec(t, func(api.TerminalDoc) {
		once.Do(calls.Done)
		time.Sleep(5 * time.Second)
	})
	if err := sr.updateTerminalState(api.Starting); err != nil {
		t.Fatal(err)
	}
	calls.Wait() // observer is asleep

	start := time.Now()
	for range 100 {
		if err := sr.updateTerminalState(api.Ready); err != nil {
			t.Fatal(err)
		}
	}
	if err := sr.markExited(); err != nil {
		t.Fatal(err)
	}
	sr.closeStatusObserver()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("posting with a sleeping observer took %v", d)
	}
}

// TestStatusObserver_CloseDeliversFinalThenStops asserts the document
// pending at close is still delivered and nothing after it.
func TestStatusObserver_CloseDeliversFinalThenStops(t *testing.T) {
	r := newRecorder(true)
	sr := newObservedExec(t, r.observe)
	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Ready)
	if err := sr.markExited(); err != nil {
		t.Fatal(err)
	}
	sr.closeStatusObserver()
	sr.notifyStatus() // after close: dropped
	sr.closeStatusObserver()

	r.release <- struct{}{}
	waitEntered(t, r, api.Exited)
	r.release <- struct{}{}
	select {
	case s := <-r.entered:
		t.Fatalf("observer called again after close with %v", s)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStatusObserver_PanicRecovered(t *testing.T) {
	r := newRecorder(false)
	first := true
	sr := newObservedExec(t, func(doc api.TerminalDoc) {
		if first {
			first = false
			panic("embedder bug")
		}
		r.observe(doc)
	})
	if err := sr.updateTerminalState(api.Starting); err != nil {
		t.Fatal(err)
	}
	// Wait for the panicking call to be consumed before posting again so
	// the second post is not coalesced with it.
	deadline := time.Now().Add(5 * time.Second)
	for len(sr.statusCh) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if err := sr.updateTerminalState(api.Ready); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, r, api.Ready)
}

// TestStatusObserver_DocumentIsACopy asserts the observer cannot mutate the
// runner's document through the delivered value.
func TestStatusObserver_DocumentIsACopy(t *testing.T) {
	got := make(chan api.TerminalDoc, 4)
	sr := newObservedExec(t, func(doc api.TerminalDoc) { got <- doc })
	code := 3
	sr.metadataMu.Lock()
	sr.metadata.Status.Attachers = []string{"c1"}
	sr.metadata.Status.ExitCode = &code
	sr.metadata.Metadata.Labels = map[string]string{"k": "v"}
	sr.metadataMu.Unlock()
	sr.notifyStatus()

	doc := <-got
	doc.Status.Attachers[0] = "mutated"
	*doc.Status.ExitCode = 99
	doc.Metadata.Labels["k"] = "mutated"

	md, _ := sr.Metadata()
	if md.Status.Attachers[0] != "c1" || *md.Status.ExitCode != 3 || md.Metadata.Labels["k"] != "v" {
		t.Fatalf("observer mutation leaked into runner document: %+v", md)
	}
}
