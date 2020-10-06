package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mailru/easyjson"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	"testing"
)

func TestEasyJson(t *testing.T) {
	var rawMessage easyjson.RawMessage
	var out map[string]interface{}

	//goland:noinspection GoNilness
	if len(rawMessage) > 0 {
		err := json.Unmarshal(rawMessage, &out)
		if err != nil {
			t.Error(err)
		}
	}

	var rv ReturnValue
	err := json.Unmarshal(easyjson.RawMessage("{}"), &rv)
	if err != nil {
		t.Error(err)
	}
}

func TestReturnValue(t *testing.T) {
	rvs := []string{
		`{"waitForData": true,"next": "","data": ""}`,
		`{"waitForData": true,"next": "","data": [1, 2, 3]}`,
		`null`,
		`{"waitForData": true}`,
	}
	for _, s := range rvs {
		var rv ReturnValue
		err := json.Unmarshal([]byte(s), &rv)
		if err != nil {
			t.Errorf("failed to unmarshal return value: %v", err)
		}
	}
}

func TestRun(t *testing.T) {
	// unmarshalArgs takes a JSON object and unmarshals it to a map from string to empty interface
	unmarshalArgs := func(arguments easyjson.RawMessage) map[string]interface{} {
		var args map[string]interface{}
		err := json.Unmarshal(arguments, &args)
		if err != nil {
			t.Errorf("Failed to unmarshal arguments: %v", err)
		}
		return args
	}

	// An expectation specifies the expected state after a script run for a specific script
	type expectation struct {
		// callCount specifies the number of times a script is expected to be called
		callCount int
		// waitCount specifies the number of times the wait function is expected to be called
		waitCount int
		// arguments specifies the expected arguments a script received when executed
		arguments []map[string]interface{}
		// returnValues specifies the expected return values from execution(s) of a script
		returnValues []ReturnValue
	}

	type script struct {
		configObject *configV1.ConfigObject
		function     function
	}

	type test struct {
		// name of test describing it's purpose
		name string
		// context given to test run
		ctx context.Context
		// seed is the seed which annotations is used as arguments
		seed configV1.ConfigObject
		// scripts is a list of scripts given to a test run
		scripts []script
		// seq is the ordered sequence of id's in which scripts are expected to be executed
		seq []string
		// expectToFail specifies if a script run is expected to fail
		expectToFail bool
		// err (optional) is the expected error of a script run that is expected to fail
		err error
		// expectations is a map of script id's with expectations for the specific script
		expectations map[string]expectation
	}

	tests := []test{
		{
			name: "script recursion",
			ctx:  context.Background(),
			scripts: []script{
				{
					&configV1.ConfigObject{
						Id:   "1",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{
							Name:        "one.js",
							Description: "A script that will call itself once",
							Annotation: []*configV1.Annotation{
								{
									Key:   "next",
									Value: "self",
								},
							},
						},
					},
					func() function {
						count := 2
						return func(message easyjson.RawMessage) (easyjson.RawMessage, error) {
							count--
							args := unmarshalArgs(message)
							next := args["next"].(string)
							if count > 0 {
								return json.Marshal(ReturnValue{Next: next})
							}
							return json.Marshal(ReturnValue{})
						}
					}(),
				},
			},
			seq: []string{"1", "1"},
			expectations: map[string]expectation{
				"1": {
					callCount: 2,
					arguments: []map[string]interface{}{
						{
							"next": "self",
						},
						{
							"next": "self",
						},
					},
					returnValues: []ReturnValue{
						{
							Next: "self",
						},
						{},
					},
				},
			},
		},
		// test argument precedence: ReturnValue.Data > seed annotations > script annotations
		{
			name: "argument precedence",
			ctx:  context.Background(),
			seed: configV1.ConfigObject{
				Kind: configV1.Kind_seed,
				Meta: &configV1.Meta{
					Annotation: []*configV1.Annotation{
						{
							Key:   "username",
							Value: "medium",
						},
					},
				},
			},
			scripts: []script{
				{
					&configV1.ConfigObject{
						Id:   "1",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{
							Name:        "one.js",
							Description: "A script that will call itself once",
							Annotation: []*configV1.Annotation{
								{
									Key:   "username",
									Value: "small",
								},
								{
									Key: "v7n_seed-annotation-key",
									Value: "username",
								},
							},
						},
					},
					func() function {
						data, err := json.Marshal(struct {
							Username string `json:"username"`
						}{Username: "large"})
						if err != nil {
							t.Errorf("Failed to marshal data: %v", err)
						}
						count := 2
						return func(_ easyjson.RawMessage) (easyjson.RawMessage, error) {
							count--
							if count > 0 {
								return json.Marshal(ReturnValue{
									Next: "self",
									Data: data,
								})
							}
							return json.Marshal(ReturnValue{})
						}
					}(),
				},
			},
			seq: []string{"1", "1"},
			expectations: map[string]expectation{
				"1": {
					callCount: 2,
					arguments: []map[string]interface{}{
						{
							"username": "medium",
						},
						{
							"username": "large",
						},
					},
					returnValues: []ReturnValue{
						{
							Next: "self",
							Data: func() []byte {
								data, _ := json.Marshal(struct {
									Username string `json:"username"`
								}{Username: "large"})
								return data
							}(),
						},
						{},
					},
				},
			},
		},
		{
			name: "chained scripts",
			ctx:  context.Background(),
			scripts: []script{
				{
					&configV1.ConfigObject{
						Id:   "2",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{
							Name:        "two.js",
							Description: "A script that schedules script #3 to be executed",
							Annotation: []*configV1.Annotation{
								{
									Key:   "next",
									Value: "3",
								},
							},
						},
					},
					func(message easyjson.RawMessage) (easyjson.RawMessage, error) {
						args := unmarshalArgs(message)
						next := args["next"].(string)
						rv := ReturnValue{Next: next}
						return json.Marshal(rv)
					},
				},
				{
					&configV1.ConfigObject{
						Id:   "3",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{
							Name:        "three.js",
							Description: "A script to be scheduled by #2",
						},
					},
					func(_ easyjson.RawMessage) (easyjson.RawMessage, error) {
						return nil, nil
					},
				},
			},
			seq: []string{"2", "3"},
			expectations: map[string]expectation{
				"2": {
					callCount: 1,
					arguments: []map[string]interface{}{
						{
							"next": "3",
						},
					},
					returnValues: []ReturnValue{
						{
							Next: "3",
						},
					},
				},
				"3": {
					callCount:    1,
					arguments:    []map[string]interface{}{},
					returnValues: []ReturnValue{},
				},
			},
		},
		{
			name: "wait for data",
			ctx:  context.Background(),
			scripts: []script{
				{
					&configV1.ConfigObject{
						Id:   "4",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{
							Name: "four.js",
						},
					},
					func(_ easyjson.RawMessage) (easyjson.RawMessage, error) {
						return json.Marshal(ReturnValue{WaitForData: true})
					},
				},
			},
			seq: []string{"4"},
			expectations: map[string]expectation{
				"4": {
					callCount: 1,
					arguments: []map[string]interface{}{},
					returnValues: []ReturnValue{
						{
							WaitForData: true,
						},
					},
					waitCount: 1,
				},
			},
		},
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				return ctx
			}(),
			scripts: []script{
				{
					&configV1.ConfigObject{
						Id:   "1",
						Kind: configV1.Kind_browserScript,
						Meta: &configV1.Meta{Name: "redundant.js"},
					},
					nil,
				},
			},
			expectToFail: true,
			err:          context.Canceled,
		},
	}

	//goland:noinspection GoVetCopyLock
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			functions := make(map[string]function)
			scripts := make(map[string]*configV1.ConfigObject)
			waiters := make(map[string]waiter)

			//goland:noinspection GoVetCopyLock
			for _, script := range test.scripts {
				id := script.configObject.GetId()
				scripts[id] = script.configObject
				functions[id] = script.function
				waiters[id] = waiter{}
			}

			executor := newScriptExecutor(functions)
			// the first script in a test is considered the starting point
			id := test.scripts[0].configObject.GetId()
			name := test.scripts[0].configObject.GetMeta().GetName()
			waiter := waiters[id]

			err := Run(test.ctx, id, &test.seed, scripts, executor.execute, waiter.wait)
			if err != nil {
				if !test.expectToFail {
					t.Errorf("Script %s failed unexpectedly: %v", name, err)
				} else if test.err != nil && !errors.Is(err, test.err) {
					t.Errorf("Expected error was: %v, but got: %v", test.err, err)
				}
			}

			// for debugging test by logging actual input/output (arguments/results)
			//for id, metric := range executor.metrics {
			//	name := scripts[id].GetMeta().GetName()
			//	t.Logf("Script %s (%s) metrics:\n", name, id)
			//	for i, execution := range metric {
			//		t.Logf("%d. iteration: (%s) -> %s\n", i+1, execution.arguments, execution.result)
			//	}
			//}

			if len(test.seq) != len(executor.seq) {
				t.Errorf("Expected length of call sequence is %d, got %d", len(test.seq), len(executor.seq))
			}
			// t.Logf("Expected sequence: %v\n", test.seq)
			// t.Logf("Actual sequence: %v\n", executor.seq)
			for i, want := range test.seq {
				got := executor.seq[i]
				if want != got {
					t.Errorf("Expected sequence #%d was %s, got %s", i, want, got)
				}
			}

			for id, expect := range test.expectations {
				name = scripts[id].GetMeta().GetName()

				metrics, ok := executor.metrics[id]
				if !ok {
					t.Errorf("Expected script %s (%s) to have metrics (i.e. to have been called)", name, id)
				}

				gotCallCount := len(metrics)
				if expect.callCount != gotCallCount {
					t.Errorf("Expected script %s (%s) to be called %d times, got %d", name, id, expect.callCount, gotCallCount)
				}

				if expect.waitCount != waiter.count {
					t.Errorf("Expected script of %s (%s) to wait %d times, got %d", name, id, expect.callCount, waiter.count)
				}

				for i, expected := range expect.arguments {
					actual := unmarshalArgs(metrics[i].arguments)

					for key, want := range expected {
						if got, ok := actual[key]; !ok {
							t.Errorf("In iteration %d, %s (%s): missing expected argument %s", i+1, name, id, key)
						} else if want != got {
							t.Errorf("In iteration %d, %s (%s): expected argument %s to have value %v, got %v", i+1, name, id, key, want, got)
						}
					}
					for key, got := range actual {
						_, ok := expected[key]
						if !ok {
							t.Errorf("In iteration %d, %s (%s): unexpected argument %s = %v", i+1, name, id, key, got)
						}
					}
				}
			}

		})
	}
}

type waiter struct {
	count int
}

func (w *waiter) wait() {
	w.count++
}

func TestWaiter(t *testing.T) {
	waiter := waiter{}
	waiter.wait()

	if waiter.count != 1 {
		t.Error("Expected wait count to be 1")
	}
}

type metric struct {
	arguments easyjson.RawMessage
	result    easyjson.RawMessage
}

type function func(message easyjson.RawMessage) (easyjson.RawMessage, error)

// A scriptExecutor is a function executor that records some metrics about functions that are executed.
type scriptExecutor struct {
	metrics   map[string][]*metric
	functions map[string]function
	seq       []string
}

// newScriptExecutor returns a new mock script executor
func newScriptExecutor(functions map[string]function) *scriptExecutor {
	return &scriptExecutor{
		functions: functions,
		metrics:   make(map[string][]*metric),
	}
}

func (e *scriptExecutor) execute(_ context.Context, script *configV1.ConfigObject, arguments easyjson.RawMessage) (easyjson.RawMessage, error) {
	name := script.GetMeta().GetName()
	id := script.GetId()

	metric := metric{arguments: arguments}
	// append metric for this script execution
	e.metrics[script.Id] = append(e.metrics[script.Id], &metric)
	e.seq = append(e.seq, id)

	function, ok := e.functions[script.Id]
	if !ok {
		return nil, fmt.Errorf("failed to find script function for %s", script)
	}
	if function == nil {
		return nil, fmt.Errorf("nil function")
	}

	res, err := function(arguments)
	if err != nil {
		return nil, fmt.Errorf("script %s (%s) failed: %w", name, id, err)
	}
	metric.result = res

	return res, nil
}
